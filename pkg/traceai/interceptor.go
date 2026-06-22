package traceai

import (
	"context"
	"net/http"
)

type Interceptor struct {
	Client *Client
	Info   CallInfo
}

func (i Interceptor) Record(success bool, inputSize, outputSize int64, err error) {
	if i.Client == nil {
		return
	}
	ctx := RecordStart(context.Background(), i.Client, i.Info)
	_ = RecordFinish(ctx, success, inputSize, outputSize, err)
}

func (i Interceptor) WrapHTTP(next http.Handler, toolType, toolName, functionName string) http.Handler {
	return HTTPMiddleware(i.Client, CallInfo{
		AdapterName:    i.Info.AdapterName,
		AdapterVersion: i.Info.AdapterVersion,
		AgentName:      i.Info.AgentName,
		AgentVersion:   i.Info.AgentVersion,
		ToolType:       toolType,
		ToolName:       toolName,
		FunctionName:   functionName,
		Metadata:       i.Info.Metadata,
	})(next)
}

func (i Interceptor) CaptureRPC(ctx context.Context, toolType, toolName, functionName string, fn func(context.Context) (int64, int64, error)) error {
	return CaptureRPC(ctx, i.Client, CallInfo{
		AdapterName:    i.Info.AdapterName,
		AdapterVersion: i.Info.AdapterVersion,
		AgentName:      i.Info.AgentName,
		AgentVersion:   i.Info.AgentVersion,
		ToolType:       toolType,
		ToolName:       toolName,
		FunctionName:   functionName,
		Metadata:       i.Info.Metadata,
	}, fn)
}

func (i Interceptor) WrapMCP(agentName, toolName, functionName string) func(func() error) func() error {
	return func(next func() error) func() error {
		return func() error {
			return WrapMCP(i.Client, CallInfo{
				AdapterName:    i.Info.AdapterName,
				AdapterVersion: i.Info.AdapterVersion,
				AgentName:      agentName,
				AgentVersion:   i.Info.AgentVersion,
				ToolType:       "mcp",
				ToolName:       toolName,
				FunctionName:   functionName,
				Metadata:       i.Info.Metadata,
			}, func(context.Context) error {
				return next()
			})(context.Background())
		}
	}
}

type recordingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	lastWriteErr error
}

func (r *recordingResponseWriter) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recordingResponseWriter) Write(p []byte) (int, error) {
	n, err := r.ResponseWriter.Write(p)
	r.bytesWritten += n
	if err != nil {
		r.lastWriteErr = err
	}
	return n, err
}

func (r *recordingResponseWriter) Err() error {
	return r.lastWriteErr
}

func requestSize(r *http.Request) int {
	if r == nil || r.Body == nil {
		return 0
	}
	if r.ContentLength >= 0 {
		return int(r.ContentLength)
	}
	return 0
}
