package traceai

import (
	"context"
	"errors"
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/models"
	"github.com/MIK-HEAL/TraceAI/pkg/state"
)

type Client struct {
	Store  Store
	Export Exporter
	Opened bool
}

func New(store Store) *Client {
	return &Client{
		Store:  store,
		Export: NewLocalExporter(),
	}
}

func (c *Client) Start(ctx context.Context) error {
	if err := c.Store.Init(ctx); err != nil {
		return err
	}
	c.Opened = true
	return nil
}

func (c *Client) Publish(event models.ToolEvent) error {
	normalized := event.Normalize()
	if err := c.Store.InsertEvent(context.Background(), normalized); err != nil {
		return err
	}
	if c.Export != nil {
		if err := c.Export.Export(normalized); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Close(timeout time.Duration) error {
	c.Opened = false
	var errs []error
	if c.Export != nil {
		if err := c.Export.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := c.Store.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (c *Client) Status(ctx context.Context) (state.Status, error) {
	status := state.Status{
		CheckedAt: time.Now().UTC(),
	}
	if err := c.Store.Ping(ctx); err != nil {
		status.LastError = err.Error()
		return status, err
	}
	status.StorageOK = true
	status.QueueClosed = !c.Opened
	return status, nil
}

func (c *Client) Metrics(ctx context.Context) ([]state.Metric, error) {
	stats, err := c.Store.Stats(ctx, time.Time{})
	if err != nil {
		return nil, err
	}
	return []state.Metric{
		{Name: "calls", Value: float64(stats.Calls)},
		{Name: "success_rate", Value: stats.SuccessRate},
		{Name: "avg_latency_ms", Value: stats.AvgLatency},
		{Name: "input_size", Value: float64(stats.InputSize)},
		{Name: "output_size", Value: float64(stats.OutputSize)},
	}, nil
}

func (c *Client) TopTools(ctx context.Context, since time.Time, limit int) ([]models.ToolCount, error) {
	return c.Store.TopTools(ctx, since, limit)
}

func (c *Client) TopFunctions(ctx context.Context, since time.Time, limit int) ([]models.FunctionCount, error) {
	return c.Store.TopFunctions(ctx, since, limit)
}

func (c *Client) TopAgents(ctx context.Context, since time.Time, limit int) ([]models.AgentCount, error) {
	return c.Store.TopAgents(ctx, since, limit)
}

func (c *Client) Stats(ctx context.Context, since time.Time) (models.Stats, error) {
	return c.Store.Stats(ctx, since)
}

func (c *Client) DailyStats(ctx context.Context, since time.Time) ([]models.DailyStat, error) {
	return c.Store.DailyStats(ctx, since)
}

func (c *Client) MonthlyStats(ctx context.Context, since time.Time) ([]models.MonthlyStat, error) {
	return c.Store.MonthlyStats(ctx, since)
}

func (c *Client) WeeklyStats(ctx context.Context, since time.Time) ([]models.WeeklyStat, error) {
	return c.Store.WeeklyStats(ctx, since)
}

func (c *Client) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]models.ErrorBreakdown, error) {
	return c.Store.ErrorBreakdowns(ctx, since, limit)
}
