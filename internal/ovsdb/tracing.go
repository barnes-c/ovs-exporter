package ovsdb

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// dbSystem matches the OpenTelemetry semantic convention value for the
	// OVSDB protocol. Set on every span this package emits.
	attrDBSystemValue = "ovsdb"
	// dbName is the OVSDB database name. Open_vSwitch is the only DB this
	// client talks to; OVN NB/SB clients (T16) will set their own values.
	attrDBNameValue = "Open_vSwitch"
)

// startSpan opens an ovsdb.* span if a Tracer is configured. Returns the
// derived context and the span (which may be a no-op).
func (c *Client) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if c.tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	baseAttrs := []attribute.KeyValue{
		attribute.String("db.system", attrDBSystemValue),
		attribute.String("db.name", attrDBNameValue),
		attribute.String("ovsdb.endpoint", c.cfg.Endpoint),
	}
	return c.tracer.Start(ctx, name, trace.WithAttributes(append(baseAttrs, attrs...)...))
}

// endSpan records err on span (if non-nil) and ends it.
func endSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// attrOp returns the ovsdb.op attribute for a span.
func attrOp(op string) attribute.KeyValue {
	return attribute.String("ovsdb.op", op)
}
