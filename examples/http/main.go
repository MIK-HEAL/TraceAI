package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/traceai"
)

func main() {
	ctx := context.Background()
	client := traceai.New(traceai.NewMemoryStore())
	if err := client.Start(ctx); err != nil {
		panic(err)
	}
	defer func() { _ = client.Close(5 * time.Second) }()

	handler := traceai.HTTPMiddleware(client, traceai.CallInfo{
		AdapterName:  "http",
		AgentName:    "demo-agent",
		ToolType:     "http",
		ToolName:     "profile",
		FunctionName: "GET /health",
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req, _ := http.NewRequest(http.MethodGet, "http://localhost/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	rows, err := client.Store.ListEvents(ctx, 10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("events=%d status=%d semantic=%v\n", len(rows), rr.Code, traceai.SemanticFields())
}
