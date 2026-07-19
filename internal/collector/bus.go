package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

var (
	ErrBusClosed = errors.New("collector bus is closed")
	ErrQueueFull = errors.New("collector bus queue is full")
)

type Bus struct {
	storage    storage.Storage
	input      chan events.ToolEvent
	batchSize  int
	flushEvery time.Duration
	done       chan struct{}
	errs       chan error
	wg         sync.WaitGroup
	once       sync.Once
	publishMu  sync.Mutex
	closed     bool
	mu         sync.Mutex
	lastErr    error
	maxRetries int
	runCtx     context.Context
	cancel     context.CancelFunc
}

func NewBus(storage storage.Storage, batchSize int, flushEvery time.Duration) *Bus {
	if batchSize <= 0 {
		batchSize = 32
	}
	if flushEvery <= 0 {
		flushEvery = 500 * time.Millisecond
	}
	return &Bus{
		storage:    storage,
		input:      make(chan events.ToolEvent, batchSize*4),
		batchSize:  batchSize,
		flushEvery: flushEvery,
		done:       make(chan struct{}),
		errs:       make(chan error, 1),
		maxRetries: 3,
	}
}

func (b *Bus) Start(ctx context.Context) error {
	b.publishMu.Lock()
	if b.runCtx != nil {
		b.publishMu.Unlock()
		return errors.New("bus already started")
	}
	if b.closed {
		b.publishMu.Unlock()
		return ErrBusClosed
	}
	runCtx, cancel := context.WithCancel(ctx)
	b.runCtx = runCtx
	b.cancel = cancel
	b.publishMu.Unlock()

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		ticker := time.NewTicker(b.flushEvery)
		defer ticker.Stop()
		var batch []events.ToolEvent
		flush := func() {
			if len(batch) == 0 {
				return
			}
			for _, event := range batch {
				if err := b.insertWithRetry(runCtx, event); err != nil {
					b.reportError(err)
				}
			}
			batch = batch[:0]
		}
		for {
			select {
			case <-runCtx.Done():
				flush()
				b.once.Do(func() { close(b.done) })
				slog.Default().With("component", "collector").Info("collector stopped", "reason", "context_done")
				return
			case event, ok := <-b.input:
				if !ok {
					flush()
					b.once.Do(func() { close(b.done) })
					return
				}
				batch = append(batch, event)
				if len(batch) >= b.batchSize {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()
	return nil
}

// Publish queues an event for asynchronous persistence. It never blocks the
// caller; a full or closed queue is returned as an error so callers can choose
// whether to retry, surface, or record the loss.
func (b *Bus) Publish(event events.ToolEvent) error {
	if err := event.Validate(); err != nil {
		err = fmt.Errorf("invalid event: %w", err)
		b.reportError(err)
		return err
	}

	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	if b.closed {
		slog.Default().With("component", "collector").Warn("event dropped", "reason", "bus_closed", "event_id", event.EventID)
		b.reportError(ErrBusClosed)
		return ErrBusClosed
	}
	event = event.Clone()
	select {
	case b.input <- event:
		return nil
	default:
		err := fmt.Errorf("%w (capacity %d)", ErrQueueFull, cap(b.input))
		slog.Default().With("component", "collector").Warn("event dropped", "reason", "queue_full", "event_id", event.EventID, "queue_len", len(b.input), "queue_cap", cap(b.input))
		b.reportError(err)
		return err
	}
}

func (b *Bus) Errors() <-chan error {
	return b.errs
}

func (b *Bus) Input() <-chan events.ToolEvent {
	return b.input
}

func (b *Bus) IsClosed() bool {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	return b.closed
}

func (b *Bus) LastError() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastErr
}

func (b *Bus) Close() {
	_ = b.CloseWithTimeout(5 * time.Second)
}

func (b *Bus) CloseWithTimeout(timeout time.Duration) error {
	b.publishMu.Lock()
	if b.runCtx == nil {
		b.closed = true
		b.publishMu.Unlock()
		return nil
	}
	if !b.closed {
		b.closed = true
		close(b.input)
	}
	b.publishMu.Unlock()
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-b.done:
		slog.Default().With("component", "collector").Info("collector closed")
		return nil
	case <-timer.C:
		if b.cancel != nil {
			b.cancel()
		}
		err := fmt.Errorf("collector close timed out after %s", timeout)
		b.reportError(err)
		slog.Default().With("component", "collector").Error("collector close timeout", "timeout", timeout, "error", err)
		return err
	}
}

func (b *Bus) insertWithRetry(ctx context.Context, event events.ToolEvent) error {
	var err error
	for attempt := 0; attempt < b.maxRetries; attempt++ {
		err = b.storage.InsertEvent(ctx, event)
		if err == nil {
			return nil
		}
		if !isRetryableStorageError(err) {
			return fmt.Errorf("persist event %s: %w", event.EventID, err)
		}
		delay := time.Duration(attempt+1) * 10 * time.Millisecond
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return fmt.Errorf("failed to persist event %s after %d attempts: %w", event.EventID, b.maxRetries, err)
}

func isRetryableStorageError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, token := range []string{"validation", "required", "constraint", "foreign key", "syntax", "malformed", "storage closed"} {
		if strings.Contains(message, token) {
			return false
		}
	}
	return true
}

func (b *Bus) reportError(err error) {
	if err == nil {
		return
	}
	b.mu.Lock()
	b.lastErr = err
	b.mu.Unlock()
	slog.Default().With("component", "collector").Error("collector error", "error", err)
	select {
	case b.errs <- err:
	default:
	}
}
