package otel

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	otelslog "go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Using the module path so the OTel `instrumentation.scope.name` mirrors code base
const scopeName = "github.com/barnes-c/ovs-exporter"

// histogramBoundaries is the explicit-bucket set applied to every Histogram
// instrument via a View. Cumulative temporality (the SDK default) keeps the
// Prometheus exporter output compatible with PromQL `histogram_quantile`.
// Exponential histograms are intentionally NOT enabled — most Prometheus
// deployments still cannot parse them.
var histogramBoundaries = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// Config configures the OTel pipeline.
//
// The exporter selectors follow the OpenTelemetry environment-variable
// convention (OTEL_METRICS_EXPORTER, OTEL_TRACES_EXPORTER, OTEL_LOGS_EXPORTER)
// but with two project-specific deviations:
//
//   - MetricsExporter defaults to "prometheus" (not "otlp") so /metrics is
//     served out of the box. Comma-separated values are allowed to enable
//     multiple readers, e.g. "prometheus,otlp" or "prometheus,console".
//   - TracesExporter and LogsExporter default to "none" — most operators
//     only adopt traces/logs when they wire up an OTel collector.
//
// Values "prometheus", "otlp", "console", and "none" are supported. The
// canonical alias "otlp/stdout" is accepted for "console".
type Config struct {
	ServiceName     string
	ServiceVersion  string
	Protocol        string // OTLP transport: "grpc" | "http/protobuf"
	OTLPInterval    time.Duration
	MetricsExporter string  // comma-separated; default "prometheus"
	TracesExporter  string  // default "none"
	LogsExporter    string  // default "none"
	TraceSampleRate float64 // 0 < rate <= 1
	PromMaxRequests int     // promhttp MaxRequestsInFlight; 0 → 40
}

// Result is what Setup returns. PromHandler is nil when "prometheus" is not
// in MetricsExporter. Logger is the original logger by default; when
// LogsExporter != "none" it is tee'd to also forward records through the
// OTel log pipeline — callers should replace their logger with this one.
type Result struct {
	Meter       metric.Meter
	Tracer      trace.Tracer
	PromHandler http.Handler
	Logger      *slog.Logger
	Shutdown    func(ctx context.Context) error
}

func Setup(ctx context.Context, logger *slog.Logger, cfg Config) (*Result, error) {
	cfg.MetricsExporter = cmp.Or(cfg.MetricsExporter, "prometheus")
	cfg.TracesExporter = cmp.Or(cfg.TracesExporter, "none")
	cfg.LogsExporter = cmp.Or(cfg.LogsExporter, "none")
	cfg.Protocol = cmp.Or(cfg.Protocol, "grpc")
	cfg.OTLPInterval = cmp.Or(cfg.OTLPInterval, 15*time.Second)
	cfg.TraceSampleRate = cmp.Or(cfg.TraceSampleRate, 1.0)
	cfg.PromMaxRequests = cmp.Or(cfg.PromMaxRequests, 40)

	if cfg.ServiceName == "" {
		return nil, fmt.Errorf("ServiceName is required")
	}

	res, err := buildResource(ctx, cfg.ServiceName, cfg.ServiceVersion)
	if err != nil {
		return nil, err
	}

	var shutdowns []func(context.Context) error

	mp, promHandler, err := buildMeterProvider(ctx, res, cfg)
	if err != nil {
		return nil, err
	}
	if mp != nil {
		otel.SetMeterProvider(mp)
		shutdowns = append(shutdowns, mp.Shutdown)
	}
	meter := otel.Meter(scopeName)

	tracer := otel.Tracer(scopeName)
	if cfg.TracesExporter != "none" {
		tp, err := buildTracerProvider(ctx, res, cfg)
		if err != nil {
			return nil, err
		}
		otel.SetTracerProvider(tp)
		shutdowns = append(shutdowns, tp.Shutdown)
		tracer = tp.Tracer(scopeName)
	}

	finalLogger := logger
	if cfg.LogsExporter != "none" {
		lp, teedLogger, err := buildLoggerProvider(ctx, res, logger, cfg)
		if err != nil {
			return nil, err
		}
		shutdowns = append(shutdowns, lp.Shutdown)
		finalLogger = teedLogger
	}

	logger.Info("OTel pipeline configured",
		"service_name", cfg.ServiceName,
		"service_version", cfg.ServiceVersion,
		"metrics_exporter", cfg.MetricsExporter,
		"traces_exporter", cfg.TracesExporter,
		"logs_exporter", cfg.LogsExporter,
		"otlp_protocol", cfg.Protocol,
		"otlp_interval", cfg.OTLPInterval,
		"trace_sample_rate", cfg.TraceSampleRate,
		"prom_endpoint_enabled", promHandler != nil,
	)

	shutdown := func(ctx context.Context) error {
		var errs []error
		for _, s := range shutdowns {
			if err := s(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}

	return &Result{
		Meter:       meter,
		Tracer:      tracer,
		PromHandler: promHandler,
		Logger:      finalLogger,
		Shutdown:    shutdown,
	}, nil
}

// buildMeterProvider assembles the MeterProvider from the comma-separated
// MetricsExporter list. Returns (nil, nil, nil) when MetricsExporter is
// "none". PromHandler is non-nil iff "prometheus" appears in the list.
func buildMeterProvider(ctx context.Context, res *resource.Resource, cfg Config) (*sdkmetric.MeterProvider, http.Handler, error) {
	kinds, err := parseMetricsExporters(cfg.MetricsExporter)
	if err != nil {
		return nil, nil, err
	}
	if len(kinds) == 0 {
		return nil, nil, nil
	}

	var (
		readers     []sdkmetric.Reader
		promHandler http.Handler
	)

	for _, kind := range kinds {
		switch kind {
		case "prometheus":
			reader, handler, err := newPromReader(cfg.PromMaxRequests)
			if err != nil {
				return nil, nil, err
			}
			readers = append(readers, reader)
			promHandler = handler
		case "otlp":
			exp, err := newMetricExporter(ctx, "otlp", cfg.Protocol)
			if err != nil {
				return nil, nil, err
			}
			readers = append(readers, sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(cfg.OTLPInterval)))
		case "console", "otlp/stdout":
			exp, err := newMetricExporter(ctx, "console", "")
			if err != nil {
				return nil, nil, err
			}
			readers = append(readers, sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(cfg.OTLPInterval)))
		default:
			return nil, nil, fmt.Errorf("unsupported metrics exporter %q", kind)
		}
	}

	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
					Boundaries: histogramBoundaries,
				},
			},
		)),
	}
	for _, r := range readers {
		opts = append(opts, sdkmetric.WithReader(r))
	}

	return sdkmetric.NewMeterProvider(opts...), promHandler, nil
}

func buildTracerProvider(ctx context.Context, res *resource.Resource, cfg Config) (*sdktrace.TracerProvider, error) {
	exp, err := newTraceExporter(ctx, cfg.TracesExporter, cfg.Protocol)
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRate))),
	), nil
}

func buildLoggerProvider(ctx context.Context, res *resource.Resource, logger *slog.Logger, cfg Config) (*sdklog.LoggerProvider, *slog.Logger, error) {
	exp, err := newLogExporter(ctx, cfg.LogsExporter, cfg.Protocol)
	if err != nil {
		return nil, nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
	)
	otelHandler := otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp))
	return lp, slog.New(multiHandler{logger.Handler(), otelHandler}), nil
}

// parseMetricsExporters splits a comma-separated list and normalizes aliases.
// Whitespace and duplicates are tolerated.
func parseMetricsExporters(s string) ([]string, error) {
	if strings.TrimSpace(s) == "none" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range strings.Split(s, ",") {
		k := strings.TrimSpace(raw)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("metrics exporter list is empty")
	}
	return out, nil
}

// multiHandler fans out slog records to multiple handlers.
type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range m {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithAttrs(attrs)
	}
	return handlers
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	handlers := make(multiHandler, len(m))
	for i, h := range m {
		handlers[i] = h.WithGroup(name)
	}
	return handlers
}
