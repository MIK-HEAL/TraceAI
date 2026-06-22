package traceai

import (
	"context"
	"encoding/json"
	"net/http"

	grpc "google.golang.org/grpc"
)

func CaptureRPC(ctx context.Context, client *Client, info CallInfo, fn func(context.Context) (int64, int64, error)) error {
	ctx = RecordStart(ctx, client, info)
	inputSize, outputSize, err := fn(ctx)
	return RecordFinish(ctx, err == nil, inputSize, outputSize, err)
}

func HTTPMiddleware(client *Client, info CallInfo) func(http.Handler) http.Handler {
	info = info.withDefaults()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			current := info
			if current.ToolName == "" {
				current.ToolName = r.URL.Path
			}
			if current.FunctionName == "" {
				current.FunctionName = r.Method + " " + r.URL.Path
			}
			ctx := RecordStart(r.Context(), client, current)
			rr := &recordingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			reqSize := requestSize(r)
			next.ServeHTTP(rr, r.WithContext(ctx))
			success := rr.statusCode < http.StatusInternalServerError && rr.lastWriteErr == nil
			err := rr.lastWriteErr
			if !success && err == nil {
				err = HTTPStatusError{Status: rr.statusCode}
			}
			_ = RecordFinish(ctx, success, int64(reqSize), int64(rr.bytesWritten), err)
		})
	}
}

func UnaryServerInterceptor(client *Client, info CallInfo) grpc.UnaryServerInterceptor {
	info = info.withDefaults()
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		current := info
		if current.FunctionName == "" {
			current.FunctionName = "unary"
		}
		ctx = RecordStart(ctx, client, current)
		var resp any
		var err error
		defer func() {
			_ = RecordFinish(ctx, err == nil, messageSize(req), messageSize(resp), err)
		}()
		resp, err = handler(ctx, req)
		return resp, err
	}
}

func StreamServerInterceptor(client *Client, info CallInfo) grpc.StreamServerInterceptor {
	info = info.withDefaults()
	return func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		current := info
		if current.FunctionName == "" {
			current.FunctionName = "stream"
		}
		ctx := RecordStart(stream.Context(), client, current)
		var inputSize, outputSize int64
		wrapped := &recordingServerStream{
			ServerStream: stream,
			onRecv: func(msg any) {
				inputSize += messageSize(msg)
			},
			onSend: func(msg any) {
				outputSize += messageSize(msg)
			},
		}
		var err error
		defer func() {
			_ = RecordFinish(ctx, err == nil, inputSize, outputSize, err)
		}()
		err = handler(srv, wrapped)
		return err
	}
}

func WrapMCP(client *Client, info CallInfo, next func(context.Context) error) func(context.Context) error {
	info = info.withDefaults()
	return func(ctx context.Context) error {
		if info.ToolType == "" {
			info.ToolType = "mcp"
		}
		ctx = RecordStart(ctx, client, info)
		err := next(ctx)
		_ = RecordFinish(ctx, err == nil, 0, 0, err)
		return err
	}
}

func messageSize(v any) int64 {
	if v == nil {
		return 0
	}
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return int64(len(data))
}

type recordingServerStream struct {
	grpc.ServerStream
	onRecv func(any)
	onSend func(any)
}

func (s *recordingServerStream) RecvMsg(m any) error {
	err := s.ServerStream.RecvMsg(m)
	if err == nil && s.onRecv != nil {
		s.onRecv(m)
	}
	return err
}

func (s *recordingServerStream) SendMsg(m any) error {
	err := s.ServerStream.SendMsg(m)
	if err == nil && s.onSend != nil {
		s.onSend(m)
	}
	return err
}
