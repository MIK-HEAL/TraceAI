package collector

import (
	"context"
	"errors"
	"testing"
	"time"

	"toollens/internal/events"
	"toollens/internal/storage"
)

type retryStorage struct {
	failures int
	calls    int
}

func (s *retryStorage) Init(ctx context.Context) error { return nil }
func (s *retryStorage) Close() error                   { return nil }
func (s *retryStorage) Ping(ctx context.Context) error { return nil }
func (s *retryStorage) ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error) {
	return nil, nil
}
func (s *retryStorage) TopTools(ctx context.Context, since time.Time, limit int) ([]storage.ToolCount, error) {
	return nil, nil
}
func (s *retryStorage) TopFunctions(ctx context.Context, since time.Time, limit int) ([]storage.FunctionCount, error) {
	return nil, nil
}
func (s *retryStorage) TopAgents(ctx context.Context, since time.Time, limit int) ([]storage.AgentCount, error) {
	return nil, nil
}
func (s *retryStorage) ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]storage.ToolFailureRate, error) {
	return nil, nil
}
func (s *retryStorage) Stats(ctx context.Context, since time.Time) (storage.Stats, error) {
	return storage.Stats{}, nil
}
func (s *retryStorage) DailyStats(ctx context.Context, since time.Time) ([]storage.DailyStat, error) {
	return nil, nil
}
func (s *retryStorage) MonthlyStats(ctx context.Context, since time.Time) ([]storage.MonthlyStat, error) {
	return nil, nil
}
func (s *retryStorage) WeeklyStats(ctx context.Context, since time.Time) ([]storage.WeeklyStat, error) {
	return nil, nil
}
func (s *retryStorage) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]storage.ErrorBreakdown, error) {
	return nil, nil
}
func (s *retryStorage) InsertEvent(ctx context.Context, event events.ToolEvent) error {
	s.calls++
	if s.calls <= s.failures {
		return errors.New("temporary failure")
	}
	return nil
}

func TestBusRetriesAndSucceeds(t *testing.T) {
	store := &retryStorage{failures: 2}
	bus := NewBus(store, 1, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bus.Start(ctx); err != nil {
		t.Fatal(err)
	}

	event := events.NewToolEvent()
	event.AdapterName = "mcp"
	event.ToolType = "mcp"
	event.ToolName = "search"
	event.FunctionName = "tool_call"
	bus.Publish(event)

	time.Sleep(100 * time.Millisecond)
	bus.Close()

	if store.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", store.calls)
	}
	if bus.LastError() != nil {
		t.Fatalf("expected no last error, got %v", bus.LastError())
	}
}

func TestBusReportsPermanentFailure(t *testing.T) {
	store := &retryStorage{failures: 10}
	bus := NewBus(store, 1, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bus.Start(ctx); err != nil {
		t.Fatal(err)
	}

	event := events.NewToolEvent()
	event.AdapterName = "mcp"
	event.ToolType = "mcp"
	event.ToolName = "search"
	event.FunctionName = "tool_call"
	bus.Publish(event)

	time.Sleep(100 * time.Millisecond)
	bus.Close()

	if bus.LastError() == nil {
		t.Fatal("expected last error to be set")
	}
	select {
	case err := <-bus.Errors():
		if err == nil {
			t.Fatal("expected error from channel")
		}
	default:
		t.Fatal("expected error on error channel")
	}
}

func TestBusPublishDoesNotBlockWhenQueueFull(t *testing.T) {
	store := &retryStorage{failures: 0}
	bus := NewBus(store, 1, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bus.Start(ctx); err != nil {
		t.Fatal(err)
	}

	event := events.NewToolEvent()
	event.AdapterName = "mcp"
	event.ToolType = "mcp"
	event.ToolName = "search"
	event.FunctionName = "tool_call"
	bus.Publish(event)
	bus.Publish(event)

	bus.Close()
	if store.calls == 0 {
		t.Fatal("expected at least one persisted event")
	}
}

func TestBusCloseIsIdempotent(t *testing.T) {
	store := &retryStorage{failures: 0}
	bus := NewBus(store, 1, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bus.Start(ctx); err != nil {
		t.Fatal(err)
	}

	bus.Close()
	bus.Close()
}

type blockingStorage struct{}

func (s *blockingStorage) Init(ctx context.Context) error { return nil }
func (s *blockingStorage) Close() error                   { return nil }
func (s *blockingStorage) Ping(ctx context.Context) error { return nil }
func (s *blockingStorage) ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error) {
	return nil, nil
}
func (s *blockingStorage) TopTools(ctx context.Context, since time.Time, limit int) ([]storage.ToolCount, error) {
	return nil, nil
}
func (s *blockingStorage) TopFunctions(ctx context.Context, since time.Time, limit int) ([]storage.FunctionCount, error) {
	return nil, nil
}
func (s *blockingStorage) TopAgents(ctx context.Context, since time.Time, limit int) ([]storage.AgentCount, error) {
	return nil, nil
}
func (s *blockingStorage) ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]storage.ToolFailureRate, error) {
	return nil, nil
}
func (s *blockingStorage) Stats(ctx context.Context, since time.Time) (storage.Stats, error) {
	return storage.Stats{}, nil
}
func (s *blockingStorage) DailyStats(ctx context.Context, since time.Time) ([]storage.DailyStat, error) {
	return nil, nil
}
func (s *blockingStorage) MonthlyStats(ctx context.Context, since time.Time) ([]storage.MonthlyStat, error) {
	return nil, nil
}
func (s *blockingStorage) WeeklyStats(ctx context.Context, since time.Time) ([]storage.WeeklyStat, error) {
	return nil, nil
}
func (s *blockingStorage) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]storage.ErrorBreakdown, error) {
	return nil, nil
}
func (s *blockingStorage) InsertEvent(ctx context.Context, event events.ToolEvent) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestBusCloseWithTimeout(t *testing.T) {
	store := &blockingStorage{}
	bus := NewBus(store, 1, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := bus.Start(ctx); err != nil {
		t.Fatal(err)
	}

	event := events.NewToolEvent()
	event.AdapterName = "mcp"
	event.ToolType = "mcp"
	event.ToolName = "search"
	event.FunctionName = "tool_call"
	bus.Publish(event)

	err := bus.CloseWithTimeout(20 * time.Millisecond)
	if err == nil {
		t.Fatal("expected close timeout error")
	}
}
