package provider

import (
	"bytes"
	"io"
	"net/http"
)

const DefaultMaxBodyBytes int64 = 1 << 20

// ReadRequestBody reads a small replayable copy without changing what the
// downstream transport receives. The bool is false when the body is too large.
func ReadRequestBody(request *http.Request, maxBytes int64) ([]byte, bool, error) {
	if request == nil || request.Body == nil {
		return nil, true, nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyBytes
	}
	if request.GetBody != nil {
		body, err := request.GetBody()
		if err != nil {
			return nil, false, err
		}
		defer body.Close()
		return readLimited(body, maxBytes)
	}
	data, complete, err := readLimited(request.Body, maxBytes)
	if err != nil {
		return nil, false, err
	}
	if complete {
		request.Body = io.NopCloser(bytes.NewReader(data))
		return data, true, nil
	}
	// The original body has only consumed the prefix, so replay it before the
	// remaining stream to keep the user request byte-for-byte intact.
	request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(data), request.Body))
	return nil, false, nil
}

// ReadResponseBody reads a bounded response prefix and restores the response
// stream for the caller. Oversized and streaming bodies are left unparsed.
func ReadResponseBody(response *http.Response, maxBytes int64) ([]byte, bool, error) {
	if response == nil || response.Body == nil {
		return nil, true, nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyBytes
	}
	data, complete, err := readLimited(response.Body, maxBytes)
	if err != nil {
		return nil, false, err
	}
	if complete {
		response.Body = io.NopCloser(bytes.NewReader(data))
		return data, true, nil
	}
	response.Body = io.NopCloser(io.MultiReader(bytes.NewReader(data), response.Body))
	return nil, false, nil
}

func readLimited(body io.Reader, maxBytes int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > maxBytes {
		return data, false, nil
	}
	return data, true, nil
}
