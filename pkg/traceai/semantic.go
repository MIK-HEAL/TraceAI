package traceai

import "github.com/MIK-HEAL/TraceAI/pkg/semantic"

func SemanticFields() []string {
	out := make([]string, len(semantic.All))
	copy(out, semantic.All)
	return out
}
