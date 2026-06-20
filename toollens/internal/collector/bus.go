package collector

import (
	"context"
	"sync"
	"time"

	"toollens/internal/events"
	"toollens/internal/storage"
)

type Bus struct {
	storage   storage.Storage
	input     chan events.ToolEvent
	batchSize int
	flushEvery time.Duration
	done      chan struct{}
	wg        sync.WaitGroup
	once      sync.Once
}

func NewBus(storage storage.Storage, batchSize int, flushEvery time.Duration) *Bus {
	if batchSize <= 0 {
		batchSize = 32
	}
	if flushEvery <= 0 {
		flushEvery = 500 * time.Millisecond
	}
	return &Bus{
		storage:   storage,
		input:     make(chan events.ToolEvent, batchSize*4),
		batchSize: batchSize,
		flushEvery: flushEvery,
		done:      make(chan struct{}),
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
				_ = b.storage.InsertEvent(ctx, event)
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

func (b *Bus) Close() {
	b.once.Do(func() { close(b.done) })
	close(b.input)
	b.wg.Wait()
}
