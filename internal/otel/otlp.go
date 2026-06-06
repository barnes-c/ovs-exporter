package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func newMetricExporter(ctx context.Context, kind, protocol string) (sdkmetric.Exporter, error) {
	switch kind {
	case "otlp":
		switch protocol {
		case "grpc":
			return otlpmetricgrpc.New(ctx)
		case "http/protobuf", "http":
			return otlpmetrichttp.New(ctx)
		default:
			return nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", protocol)
		}
	case "console", "otlp/stdout":
		return stdoutmetric.New()
	default:
		return nil, fmt.Errorf("unsupported metrics exporter %q", kind)
	}
}

func newTraceExporter(ctx context.Context, kind, protocol string) (sdktrace.SpanExporter, error) {
	switch kind {
	case "otlp":
		switch protocol {
		case "grpc":
			return otlptracegrpc.New(ctx)
		case "http/protobuf", "http":
			return otlptracehttp.New(ctx)
		default:
			return nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", protocol)
		}
	case "console", "otlp/stdout":
		return stdouttrace.New()
	default:
		return nil, fmt.Errorf("unsupported traces exporter %q", kind)
	}
}

func newLogExporter(ctx context.Context, kind, protocol string) (sdklog.Exporter, error) {
	switch kind {
	case "otlp":
		switch protocol {
		case "grpc":
			return otlploggrpc.New(ctx)
		case "http/protobuf", "http":
			return otlploghttp.New(ctx)
		default:
			return nil, fmt.Errorf("unsupported OTLP protocol %q, must be \"grpc\" or \"http/protobuf\"", protocol)
		}
	case "console", "otlp/stdout":
		return stdoutlog.New()
	default:
		return nil, fmt.Errorf("unsupported logs exporter %q", kind)
	}
}
