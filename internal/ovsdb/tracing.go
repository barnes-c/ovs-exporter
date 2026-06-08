package ovsdb

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	attrDBSystemValue = "ovsdb"
	attrDBNameValue   = "Open_vSwitch"
)

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
