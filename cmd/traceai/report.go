package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/analytics"
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

type behaviorProfile struct {
	Label string
	Read  int64
	Write int64
	Other int64
	Total int64
}

func makeReport(ctx context.Context, engine *analytics.Engine, limit int, catalogSpec string, trendDays int) (string, error) {
	tools, err := engine.TopTools(ctx, time.Time{}, limit)
	if err != nil {
		return "", err
	}
	failures, err := engine.ToolFailureRates(ctx, time.Time{}, limit)
	if err != nil {
		return "", err
	}
	agents, err := engine.TopAgents(ctx, time.Time{}, limit)
	if err != nil {
		return "", err
	}

	catalog, err := loadCatalogEntries(catalogSpec)
	if err != nil {
		return "", err
	}

	eventsRows, err := engine.Store.ListEvents(ctx, maxInt())
	if err != nil {
		return "", err
	}

	zeroCalls := zeroCallTools(tools, catalog)
	behavior := summarizeBehavior(eventsRows)
	breakdowns, err := engine.ErrorBreakdowns(ctx, time.Time{}, limit)
	if err != nil {
		return "", err
	}

	if trendDays <= 0 {
		trendDays = 7
	}
	since := time.Now().UTC().AddDate(0, 0, -(trendDays - 1))
	trendRows, err := engine.DailyStats(ctx, since)
	if err != nil {
		return "", err
	}
	weeklyRows, err := engine.WeeklyStats(ctx, time.Time{})
	if err != nil {
		return "", err
	}

	var b strings.Builder
	appendSection(&b, "Tool Heatmap")
	if len(tools) == 0 {
		appendLine(&b, "none")
	} else {
		for _, row := range tools {
			appendLine(&b, fmt.Sprintf("%-18s calls=%-4d success=%-4d", row.ToolName, row.Calls, row.Success))
		}
	}

	if len(catalog) > 0 {
		appendSection(&b, "Zero Call Tools")
		if len(zeroCalls) == 0 {
			appendLine(&b, "none")
		} else {
			for _, name := range zeroCalls {
				appendLine(&b, name)
			}
		}
	}

	appendSection(&b, "Error Rate Ranking")
	if len(failures) == 0 {
		appendLine(&b, "none")
	} else {
		for _, row := range failures {
			appendLine(&b, fmt.Sprintf("%-18s calls=%-4d failures=%-4d rate=%.1f%%", row.ToolName, row.Calls, row.Failures, row.FailureRate*100))
		}
	}

	appendSection(&b, "Behavior Profile")
	appendLine(&b, fmt.Sprintf("%s read=%d write=%d other=%d total=%d", behavior.Label, behavior.Read, behavior.Write, behavior.Other, behavior.Total))

	appendSection(&b, "Failure Reasons")
	if len(breakdowns) == 0 {
		appendLine(&b, "none")
	} else {
		for _, row := range breakdowns {
			appendLine(&b, fmt.Sprintf("%-12s %-16s %-12s calls=%-4d failures=%-4d", row.Category, blankOrDash(row.ErrorType), blankOrDash(row.ErrorCode), row.Calls, row.Failures))
		}
	}

	appendSection(&b, "Agent Usage")
	if len(agents) == 0 {
		appendLine(&b, "none")
	} else {
		for _, row := range agents {
			appendLine(&b, fmt.Sprintf("%-18s calls=%-4d success=%-4d", row.AgentName, row.Calls, row.Success))
		}
	}

	appendSection(&b, "Trend")
	if len(trendRows) == 0 {
		appendLine(&b, "none")
	} else {
		for _, row := range trendRows {
			appendLine(&b, fmt.Sprintf("%s calls=%d success=%d avg_latency_ms=%.2f", row.StatDay, row.Calls, row.Success, avgLatencyForDay(row)))
		}
		if len(trendRows) > 1 {
			first := trendRows[0]
			last := trendRows[len(trendRows)-1]
			appendLine(&b, fmt.Sprintf("delta calls=%d success=%d input=%d output=%d", last.Calls-first.Calls, last.Success-first.Success, last.InputSize-first.InputSize, last.OutputSize-first.OutputSize))
		}
	}

	appendSection(&b, "Weekly Snapshot")
	if len(weeklyRows) == 0 {
		appendLine(&b, "none")
	} else {
		for _, row := range weeklyRows {
			appendLine(&b, fmt.Sprintf("%s calls=%d success=%d", row.StatWeek, row.Calls, row.Success))
		}
	}

	return b.String(), nil
}

func avgLatencyForDay(row storage.DailyStat) float64 {
	if row.Calls == 0 {
		return 0
	}
	return float64(row.TotalDurationMS) / float64(row.Calls)
}

func appendSection(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString(title)
	b.WriteString("\n")
}

func appendLine(b *strings.Builder, line string) {
	b.WriteString(line)
	b.WriteString("\n")
}

func blankOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func zeroCallTools(actual []storage.ToolCount, catalog []string) []string {
	present := make(map[string]struct{}, len(actual))
	for _, row := range actual {
		present[row.ToolName] = struct{}{}
	}
	out := make([]string, 0)
	for _, expected := range catalog {
		if _, ok := present[expected]; ok {
			continue
		}
		out = append(out, expected)
	}
	sort.Strings(out)
	return out
}

func loadCatalogEntries(spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	if info, err := os.Stat(spec); err == nil && !info.IsDir() {
		data, err := os.ReadFile(spec)
		if err != nil {
			return nil, err
		}
		return splitCatalogEntries(string(data)), nil
	}
	return splitCatalogEntries(spec), nil
}

func splitCatalogEntries(raw string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for _, part := range strings.Split(line, ",") {
			part = strings.TrimSpace(part)
			if part == "" || strings.HasPrefix(part, "#") {
				continue
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			out = append(out, part)
		}
	}
	sort.Strings(out)
	return out
}

func summarizeBehavior(rows []events.ToolEvent) behaviorProfile {
	var profile behaviorProfile
	for _, row := range rows {
		switch classifyFunction(row.FunctionName) {
		case "read":
			profile.Read++
		case "write":
			profile.Write++
		default:
			profile.Other++
		}
		profile.Total++
	}

	switch {
	case profile.Read > profile.Write*2 && profile.Read >= profile.Other:
		profile.Label = "read-heavy"
	case profile.Write > profile.Read*2 && profile.Write >= profile.Other:
		profile.Label = "write-heavy"
	case profile.Total == 0:
		profile.Label = "no-data"
	default:
		profile.Label = "balanced"
	}
	return profile
}

func classifyFunction(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case hasAnyPrefix(normalized, []string{"read", "get", "list", "search", "fetch", "find", "query"}):
		return "read"
	case hasAnyPrefix(normalized, []string{"write", "create", "update", "delete", "add", "remove", "patch", "merge", "commit", "insert", "edit"}):
		return "write"
	default:
		return "other"
	}
}

func hasAnyPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func maxInt() int {
	return int(^uint(0) >> 1)
}
