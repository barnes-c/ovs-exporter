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

// scopeName is the instrumentation scope reported on Meter and Tracer.
// Use the module path so the OTel `instrumentation.scope.name` attribute
// reflects this codebase.
const scopeName = "github.com/barnes-c/ovs-exporter"

// Config configures the OTel pipeline.
//
// The exporter selectors follow the OpenTelemetry environment-variable
// convention (OTEL_METRICS_EXPORTER, OTEL_TRACES_EXPORTER, OTEL_LOGS_EXPORTER)
// but default to "none" instead of the OTel-spec "otlp". Rationale: this is
// a Prometheus exporter, not a general-purpose OTel app — most deployments
// scrape /metrics and have no collector reachable; defaulting to "otlp"
// would spam connection errors at localhost:4317 on every install.
//
// The Prometheus reader is ALWAYS attached to the
// MeterProvider regardless of MetricsExporter — /metrics is part of the
// exporter's contract, not a configurable signal. MetricsExporter therefore
// controls only the *push* exporters layered on top of Prom.
//
// MetricsExporter accepts a comma-separated list, e.g. "otlp", "otlp,console",
// or "none". The literal "prometheus" is accepted as a no-op (already on).
// TracesExporter and LogsExporter accept a single value ("otlp", "console",
// or "none").
type Config struct {
	ServiceName     string
	ServiceVersion  string
	Protocol        string // OTLP transport: "grpc" | "http/protobuf"
	OTLPInterval    time.Duration
	MetricsExporter string  // push exporters; default "none"
	TracesExporter  string  // default "none"
	LogsExporter    string  // default "none"
	TraceSampleRate float64 // 0 < rate <= 1
	PromMaxRequests int     // promhttp MaxRequestsInFlight; 0 → 40
}

// Result is what Setup returns. PromHandler is always non-nil — it serves
// /metrics. Logger is the original logger by default; when LogsExporter is
// not "none" it is tee'd to also forward records through the OTel log
// pipeline — callers should replace their logger with this one.
type Result struct {
	Meter       metric.Meter
	Tracer      trace.Tracer
	PromHandler http.Handler
	Logger      *slog.Logger
	Shutdown    func(ctx context.Context) error
}

// Setup constructs the configured OTel pipeline. The Prometheus reader is
// always installed. Push exporters whose selector is "none" are skipped.
// The returned Shutdown must be called at process exit.
func Setup(ctx context.Context, logger *slog.Logger, cfg Config) (*Result, error) {
	cfg.MetricsExporter = cmp.Or(cfg.MetricsExporter, "none")
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
	otel.SetMeterProvider(mp)
	shutdowns = append(shutdowns, mp.Shutdown)
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

// buildMeterProvider assembles the MeterProvider. The Prometheus reader is
// always present; additional push readers come from the parsed
// MetricsExporter list. Histograms are aggregated as native (base-2)
// exponential histograms — Prom 3.0+ ingests these directly.
func buildMeterProvider(ctx context.Context, res *resource.Resource, cfg Config) (*sdkmetric.MeterProvider, http.Handler, error) {
	promReader, promHandler, err := newPromReader(cfg.PromMaxRequests)
	if err != nil {
		return nil, nil, err
	}
	readers := []sdkmetric.Reader{promReader}

	pushKinds, err := parsePushExporters(cfg.MetricsExporter)
	if err != nil {
		return nil, nil, err
	}
	for _, kind := range pushKinds {
		exp, err := newMetricExporter(ctx, kind, cfg.Protocol)
		if err != nil {
			return nil, nil, err
		}
		readers = append(readers, sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(cfg.OTLPInterval)))
	}

	opts := []sdkmetric.Option{
		sdkmetric.WithResource(res),
		sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Kind: sdkmetric.InstrumentKindHistogram},
			sdkmetric.Stream{
				Aggregation: sdkmetric.AggregationBase2ExponentialHistogram{
					MaxSize:  160,
					MaxScale: 20,
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

// parsePushExporters splits the comma-separated MetricsExporter value into
// the non-Prom kinds that need a PeriodicReader. "none" or empty yields nil.
// "prometheus" is accepted but ignored — the Prom reader is always-on. Other
// entries are normalized (trimmed, deduped).
func parsePushExporters(s string) ([]string, error) {
	if strings.TrimSpace(s) == "none" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range strings.Split(s, ",") {
		k := strings.TrimSpace(raw)
		switch k {
		case "", "prometheus":
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
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
