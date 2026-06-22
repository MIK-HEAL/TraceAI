package traceai

import (
	"context"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
	istorage "github.com/MIK-HEAL/TraceAI/internal/storage"
	"github.com/MIK-HEAL/TraceAI/pkg/models"
)

type Store interface {
	Init(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
	InsertEvent(ctx context.Context, event models.ToolEvent) error
	ListEvents(ctx context.Context, limit int) ([]models.ToolEvent, error)
	TopTools(ctx context.Context, since time.Time, limit int) ([]models.ToolCount, error)
	TopFunctions(ctx context.Context, since time.Time, limit int) ([]models.FunctionCount, error)
	TopAgents(ctx context.Context, since time.Time, limit int) ([]models.AgentCount, error)
	ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]models.ToolFailureRate, error)
	Stats(ctx context.Context, since time.Time) (models.Stats, error)
	DailyStats(ctx context.Context, since time.Time) ([]models.DailyStat, error)
	MonthlyStats(ctx context.Context, since time.Time) ([]models.MonthlyStat, error)
	WeeklyStats(ctx context.Context, since time.Time) ([]models.WeeklyStat, error)
	ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]models.ErrorBreakdown, error)
}

type wrappedStore struct {
	inner istorage.Storage
}

func NewMemoryStore() Store {
	return &wrappedStore{inner: istorage.NewMemoryStorage()}
}

func OpenSQLite(path string) Store {
	return &wrappedStore{inner: istorage.NewSQLiteStorage(path)}
}

func wrapStorage(inner istorage.Storage) Store {
	return &wrappedStore{inner: inner}
}

func (s *wrappedStore) Init(ctx context.Context) error {
	return s.inner.Init(ctx)
}

func (s *wrappedStore) Close() error {
	return s.inner.Close()
}

func (s *wrappedStore) Ping(ctx context.Context) error {
	return s.inner.Ping(ctx)
}

func (s *wrappedStore) InsertEvent(ctx context.Context, event models.ToolEvent) error {
	return s.inner.InsertEvent(ctx, toInternalEvent(event))
}

func (s *wrappedStore) ListEvents(ctx context.Context, limit int) ([]models.ToolEvent, error) {
	rows, err := s.inner.ListEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.ToolEvent, len(rows))
	for i, row := range rows {
		out[i] = models.ToolEvent(row)
	}
	return out, nil
}

func (s *wrappedStore) TopTools(ctx context.Context, since time.Time, limit int) ([]models.ToolCount, error) {
	rows, err := s.inner.TopTools(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.ToolCount, len(rows))
	for i, row := range rows {
		out[i] = models.ToolCount(row)
	}
	return out, nil
}

func (s *wrappedStore) TopFunctions(ctx context.Context, since time.Time, limit int) ([]models.FunctionCount, error) {
	rows, err := s.inner.TopFunctions(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.FunctionCount, len(rows))
	for i, row := range rows {
		out[i] = models.FunctionCount(row)
	}
	return out, nil
}

func (s *wrappedStore) TopAgents(ctx context.Context, since time.Time, limit int) ([]models.AgentCount, error) {
	rows, err := s.inner.TopAgents(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.AgentCount, len(rows))
	for i, row := range rows {
		out[i] = models.AgentCount(row)
	}
	return out, nil
}

func (s *wrappedStore) ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]models.ToolFailureRate, error) {
	rows, err := s.inner.ToolFailureRates(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.ToolFailureRate, len(rows))
	for i, row := range rows {
		out[i] = models.ToolFailureRate(row)
	}
	return out, nil
}

func (s *wrappedStore) Stats(ctx context.Context, since time.Time) (models.Stats, error) {
	stats, err := s.inner.Stats(ctx, since)
	if err != nil {
		return models.Stats{}, err
	}
	return models.Stats(stats), nil
}

func (s *wrappedStore) DailyStats(ctx context.Context, since time.Time) ([]models.DailyStat, error) {
	rows, err := s.inner.DailyStats(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]models.DailyStat, len(rows))
	for i, row := range rows {
		out[i] = models.DailyStat(row)
	}
	return out, nil
}

func (s *wrappedStore) MonthlyStats(ctx context.Context, since time.Time) ([]models.MonthlyStat, error) {
	rows, err := s.inner.MonthlyStats(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]models.MonthlyStat, len(rows))
	for i, row := range rows {
		out[i] = models.MonthlyStat(row)
	}
	return out, nil
}

func (s *wrappedStore) WeeklyStats(ctx context.Context, since time.Time) ([]models.WeeklyStat, error) {
	rows, err := s.inner.WeeklyStats(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]models.WeeklyStat, len(rows))
	for i, row := range rows {
		out[i] = models.WeeklyStat(row)
	}
	return out, nil
}

func (s *wrappedStore) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]models.ErrorBreakdown, error) {
	rows, err := s.inner.ErrorBreakdowns(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.ErrorBreakdown, len(rows))
	for i, row := range rows {
		out[i] = models.ErrorBreakdown(row)
	}
	return out, nil
}

type storageBridge struct {
	store Store
}

func (b storageBridge) Init(ctx context.Context) error {
	return b.store.Init(ctx)
}

func (b storageBridge) Close() error {
	return b.store.Close()
}

func (b storageBridge) Ping(ctx context.Context) error {
	return b.store.Ping(ctx)
}

func (b storageBridge) InsertEvent(ctx context.Context, event events.ToolEvent) error {
	return b.store.InsertEvent(ctx, models.ToolEvent(event))
}

func (b storageBridge) ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error) {
	rows, err := b.store.ListEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]events.ToolEvent, len(rows))
	for i, row := range rows {
		out[i] = events.ToolEvent(row)
	}
	return out, nil
}

func (b storageBridge) TopTools(ctx context.Context, since time.Time, limit int) ([]istorage.ToolCount, error) {
	rows, err := b.store.TopTools(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.ToolCount, len(rows))
	for i, row := range rows {
		out[i] = istorage.ToolCount(row)
	}
	return out, nil
}

func (b storageBridge) TopFunctions(ctx context.Context, since time.Time, limit int) ([]istorage.FunctionCount, error) {
	rows, err := b.store.TopFunctions(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.FunctionCount, len(rows))
	for i, row := range rows {
		out[i] = istorage.FunctionCount(row)
	}
	return out, nil
}

func (b storageBridge) TopAgents(ctx context.Context, since time.Time, limit int) ([]istorage.AgentCount, error) {
	rows, err := b.store.TopAgents(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.AgentCount, len(rows))
	for i, row := range rows {
		out[i] = istorage.AgentCount(row)
	}
	return out, nil
}

func (b storageBridge) ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]istorage.ToolFailureRate, error) {
	rows, err := b.store.ToolFailureRates(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.ToolFailureRate, len(rows))
	for i, row := range rows {
		out[i] = istorage.ToolFailureRate(row)
	}
	return out, nil
}

func (b storageBridge) Stats(ctx context.Context, since time.Time) (istorage.Stats, error) {
	stats, err := b.store.Stats(ctx, since)
	if err != nil {
		return istorage.Stats{}, err
	}
	return istorage.Stats(stats), nil
}

func (b storageBridge) DailyStats(ctx context.Context, since time.Time) ([]istorage.DailyStat, error) {
	rows, err := b.store.DailyStats(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.DailyStat, len(rows))
	for i, row := range rows {
		out[i] = istorage.DailyStat(row)
	}
	return out, nil
}

func (b storageBridge) MonthlyStats(ctx context.Context, since time.Time) ([]istorage.MonthlyStat, error) {
	rows, err := b.store.MonthlyStats(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.MonthlyStat, len(rows))
	for i, row := range rows {
		out[i] = istorage.MonthlyStat(row)
	}
	return out, nil
}

func (b storageBridge) WeeklyStats(ctx context.Context, since time.Time) ([]istorage.WeeklyStat, error) {
	rows, err := b.store.WeeklyStats(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.WeeklyStat, len(rows))
	for i, row := range rows {
		out[i] = istorage.WeeklyStat(row)
	}
	return out, nil
}

func (b storageBridge) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]istorage.ErrorBreakdown, error) {
	rows, err := b.store.ErrorBreakdowns(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]istorage.ErrorBreakdown, len(rows))
	for i, row := range rows {
		out[i] = istorage.ErrorBreakdown(row)
	}
	return out, nil
}

func toInternalEvent(event models.ToolEvent) events.ToolEvent {
	return events.ToolEvent(event)
}
