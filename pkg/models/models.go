// Package models exposes the stable public data model used by TraceAI clients.
// The aliases keep the SDK and the internal pipeline on one canonical schema.
package models

import (
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

const SchemaVersion = events.SchemaVersion

type ToolEvent = events.ToolEvent
type ToolCount = storage.ToolCount
type FunctionCount = storage.FunctionCount
type AgentCount = storage.AgentCount
type ToolFailureRate = storage.ToolFailureRate
type Stats = storage.Stats
type DailyStat = storage.DailyStat
type MonthlyStat = storage.MonthlyStat
type WeeklyStat = storage.WeeklyStat
type ErrorBreakdown = storage.ErrorBreakdown
