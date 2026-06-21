package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"toollens/internal/events"
	"toollens/internal/storage"
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

func (b *Bus) Publish(event events.ToolEvent) {
	b.publishMu.Lock()
	defer b.publishMu.Unlock()
	if b.closed {
		slog.Default().With("component", "collector").Warn("event dropped", "reason", "bus_closed", "event_id", event.EventID)
		b.reportError(errors.New("bus is closed"))
		return
	}
	select {
	case b.input <- event:
	default:
		select {
		case b.input <- event:
		default:
			slog.Default().With("component", "collector").Warn("event dropped", "reason", "queue_full", "event_id", event.EventID, "queue_len", len(b.input), "queue_cap", cap(b.input))
			b.reportError(errors.New("event dropped: bus queue full"))
		}
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
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return fmt.Errorf("failed to persist event %s after %d attempts: %w", event.EventID, b.maxRetries, err)
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
