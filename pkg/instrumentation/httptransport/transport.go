// Package httptransport provides a fail-open provider-aware HTTP transport.
package httptransport

import (
	"net/http"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
)

type Transport struct {
	Base     http.RoundTripper
	Sink     provider.DecisionSink
	Adapters []provider.Adapter
	OnError  func(error)
}

func NewTransport(base http.RoundTripper, sink provider.DecisionSink, adapters ...provider.Adapter) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{Base: base, Sink: sink, Adapters: adapters}
}

func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	adapter := t.match(request)
	if adapter == nil {
		return base.RoundTrip(request)
	}

	requestContext, err := adapter.ObserveRequest(request)
	if err != nil {
		t.report(err)
		return base.RoundTrip(request)
	}
	response, err := base.RoundTrip(request)
	if err != nil || response == nil {
		return response, err
	}
	decisions, observeErr := adapter.ObserveResponse(requestContext, response)
	if observeErr != nil {
		t.report(observeErr)
		return response, nil
	}
	if len(decisions) > 0 && t.Sink != nil {
		t.Sink.Record(request.Context(), decisions)
	}
	return response, nil
}

func (t *Transport) match(request *http.Request) provider.Adapter {
	for _, adapter := range t.Adapters {
		if adapter != nil && adapter.Match(request) {
			return adapter
		}
	}
	return nil
}

func (t *Transport) report(err error) {
	if err != nil && t.OnError != nil {
		t.OnError(err)
	}
}
