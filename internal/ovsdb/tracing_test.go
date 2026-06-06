package ovsdb

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// recordedTracer returns a tracer that captures every span into the supplied
// SpanRecorder so tests can inspect attributes and status.
func recordedTracer(t *testing.T) (*Client, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	tracer := tp.Tracer("test")
	return &Client{
		cfg:    Config{Endpoint: "unix:/var/run/openvswitch/db.sock"},
		tracer: tracer,
	}, rec
}

func TestStartSpan_AddsAttributes(t *testing.T) {
	c, rec := recordedTracer(t)

	_, span := c.startSpan(context.Background(), "ovsdb.connect", attrOp("connect"))
	endSpan(span, nil)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	got := attrs(spans[0].Attributes())
	want := map[string]string{
		"db.system":      "ovsdb",
		"db.name":        "Open_vSwitch",
		"ovsdb.endpoint": "unix:/var/run/openvswitch/db.sock",
		"ovsdb.op":       "connect",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attr %s = %q, want %q", k, got[k], v)
		}
	}
}

func TestEndSpan_RecordsError(t *testing.T) {
	c, rec := recordedTracer(t)

	_, span := c.startSpan(context.Background(), "ovsdb.monitor")
	endSpan(span, errors.New("connection refused"))

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("status code = %v, want %v", spans[0].Status().Code, codes.Error)
	}
	if len(spans[0].Events()) == 0 {
		t.Error("expected RecordError to add an event")
	}
}

// A Client with a nil Tracer must not panic and must return a no-op span.
func TestStartSpan_NoTracer(t *testing.T) {
	c := &Client{cfg: Config{Endpoint: "unix:/var/run/openvswitch/db.sock"}}
	ctx := context.Background()

	ctx2, span := c.startSpan(ctx, "ovsdb.connect")
	if ctx2 != ctx {
		t.Error("ctx should pass through when no tracer is set")
	}
	endSpan(span, errors.New("safe to call"))
}

func attrs(in []attribute.KeyValue) map[string]string {
	out := make(map[string]string, len(in))
	for _, kv := range in {
		out[string(kv.Key)] = kv.Value.AsString()
	}
	return out
}
