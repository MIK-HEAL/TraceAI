package collector

import (
	"context"
	"fmt"
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
	mu         sync.Mutex
	lastErr    error
	maxRetries int
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
				if err := b.insertWithRetry(ctx, event); err != nil {
					b.reportError(err)
				}
			}
			batch = batch[:0]
		}
		for {
			select {
			case <-ctx.Done():
				flush()
				b.once.Do(func() { close(b.done) })
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
	select {
	case b.input <- event:
	default:
		b.input <- event
	}
}

func (b *Bus) Errors() <-chan error {
	return b.errs
}

func (b *Bus) LastError() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastErr
}

func (b *Bus) Close() {
	b.once.Do(func() { close(b.done) })
	close(b.input)
	b.wg.Wait()
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
	select {
	case b.errs <- err:
	default:
	}
}
