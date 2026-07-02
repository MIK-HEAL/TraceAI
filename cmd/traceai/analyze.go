package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/analytics"
)

// ---------------------------------------------------------------------------
// Call sequence analysis — M203
// ---------------------------------------------------------------------------

func runCallSeq(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("call-seq", flag.ExitOnError)
	limit := fs.Int("limit", 10, "number of top sequences to show")
	depth := fs.Int("depth", 2, "chain length (2 = pairs, 3 = triples)")
	_ = fs.Parse(args)

	seqs, err := engine.CallSequences(context.Background(), time.Time{}, *depth, *limit)
	if err != nil {
		return wrapError("call sequences", err)
	}

	if len(seqs) == 0 {
		_, _ = fmt.Fprintln(out, "No call sequences found. Collect more events first.")
		return nil
	}

	_, _ = fmt.Fprintf(out, "%-50s %s\n", "Call Sequence", "Count")
	_, _ = fmt.Fprintln(out, "--------------------------------------------------------------")
	for _, seq := range seqs {
		_, _ = fmt.Fprintf(out, "%-50s %d\n", seq.Sequence, seq.Count)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Retry pattern analysis — M203
// ---------------------------------------------------------------------------

func runRetryPatterns(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("retry-patterns", flag.ExitOnError)
	limit := fs.Int("limit", 10, "number of tools to show")
	_ = fs.Parse(args)

	patterns, err := engine.RetryPatterns(context.Background(), time.Time{}, *limit)
	if err != nil {
		return wrapError("retry patterns", err)
	}

	if len(patterns) == 0 {
		_, _ = fmt.Fprintln(out, "No retry patterns found. Collect more events first.")
		return nil
	}

	// Header
	_, _ = fmt.Fprintf(out, "%-18s %6s %6s %5s %5s %5s %5s %5s\n",
		"Tool", "Calls", "Sess", "OK", "FAIL", "RECOV", "DEGR", "MIXED")
	_, _ = fmt.Fprintln(out, "-------------------------------------------------------------------------")
	for _, p := range patterns {
		_, _ = fmt.Fprintf(out, "%-18s %6d %6d %5d %5d %5d %5d %5d\n",
			p.ToolName, p.TotalCalls, p.Sessions,
			p.NeverFails, p.AlwaysFails, p.Recovers, p.Degrades, p.Intermittent)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Zero-call tools analysis — M203
// ---------------------------------------------------------------------------

func runZeroCalls(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("zero-calls", flag.ExitOnError)
	catalog := fs.String("catalog", "", "expected tools catalog (file path or comma-separated list)")
	_ = fs.Parse(args)

	entries, err := loadCatalogEntries(*catalog)
	if err != nil {
		return wrapError("load catalog", err)
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "Provide --catalog with expected tool names (file or comma-separated list).")
		return nil
	}

	tools, err := engine.TopTools(context.Background(), time.Time{}, 0)
	if err != nil {
		return wrapError("top tools", err)
	}

	present := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		present[t.ToolName] = struct{}{}
	}

	var missing []string
	for _, expected := range entries {
		if _, ok := present[expected]; !ok {
			missing = append(missing, expected)
		}
	}

	if len(missing) == 0 {
		_, _ = fmt.Fprintln(out, "All expected tools have been called at least once.")
		return nil
	}

	_, _ = fmt.Fprintf(out, "Zero-call tools (%d/%d):\n", len(missing), len(entries))
	for _, name := range missing {
		_, _ = fmt.Fprintln(out, "  "+name)
	}
	_, _ = fmt.Fprintf(out, "\nTip: These tools might have poor descriptions, be hard to discover,\nor may not be needed. Consider improving descriptions or removing them.\n")
	return nil
}

// ---------------------------------------------------------------------------
// High-failure tools analysis — M203
// ---------------------------------------------------------------------------

func runHighFailures(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("high-failures", flag.ExitOnError)
	threshold := fs.Float64("threshold", 0.3, "failure rate threshold (0.0–1.0)")
	limit := fs.Int("limit", 10, "number of tools to show")
	_ = fs.Parse(args)

	rates, err := engine.ToolFailureRates(context.Background(), time.Time{}, *limit)
	if err != nil {
		return wrapError("tool failure rates", err)
	}

	var filtered []struct {
		name string
		rate float64
		calls int64
		fails int64
	}
	for _, r := range rates {
		if r.FailureRate >= *threshold {
			filtered = append(filtered, struct {
				name  string
				rate  float64
				calls int64
				fails int64
			}{r.ToolName, r.FailureRate, r.Calls, r.Failures})
		}
	}

	if len(filtered) == 0 {
		_, _ = fmt.Fprintf(out, "No tools exceed the failure threshold of %.0f%%.\n", *threshold*100)
		return nil
	}

	_, _ = fmt.Fprintf(out, "High-failure tools (threshold: %.0f%%):\n", *threshold*100)
	_, _ = fmt.Fprintf(out, "%-20s %8s %8s %8s\n", "Tool", "Calls", "Failures", "Rate")
	_, _ = fmt.Fprintln(out, "------------------------------------------------")
	for _, f := range filtered {
		_, _ = fmt.Fprintf(out, "%-20s %8d %8d %7.1f%%\n", f.name, f.calls, f.fails, f.rate*100)
	}

	_, _ = fmt.Fprintf(out, "\nTip: High failure tools may have confusing parameter schemas,\nmissing error handling, or need better descriptions. Check the error\nbreakdowns (`traceai report`) for specific failure reasons.\n")
	return nil
}
