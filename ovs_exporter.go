package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"

	_ "github.com/barnes-c/ovs-exporter/collector"
	"github.com/barnes-c/ovs-exporter/internal/otel"
	"github.com/barnes-c/ovs-exporter/internal/probes"
)

var (
	metricsPath = kingpin.Flag(
		"web.telemetry-path",
		"Path under which to expose metrics.",
	).Default("/metrics").String()

	maxProcs = kingpin.Flag(
		"runtime.gomaxprocs",
		"The target number of CPUs Go will run on (GOMAXPROCS).",
	).Envar("GOMAXPROCS").Default("1").Int()

	// OVS / OVN data-source flags and --cache.ttl will be added by their
	// consuming tasks: T5 / T8 (ovs.*), T9 (cache.ttl), T16 (ovn.*).

	otelMetricsExporter = kingpin.Flag(
		"otel.metrics-exporter",
		"Comma-separated push exporters; /metrics is always served. Values: otlp, console, none.",
	).Envar("OTEL_METRICS_EXPORTER").Default("").String()
	otelTracesExporter = kingpin.Flag(
		"otel.traces-exporter",
		"Traces exporter. Values: otlp, console, none.",
	).Envar("OTEL_TRACES_EXPORTER").Default("").String()
	otelLogsExporter = kingpin.Flag(
		"otel.logs-exporter",
		"Logs exporter. Values: otlp, console, none.",
	).Envar("OTEL_LOGS_EXPORTER").Default("").String()
	otelOTLPEndpoint = kingpin.Flag(
		"otel.otlp.endpoint",
		"OTLP collector endpoint (e.g. localhost:4317). Sets OTEL_EXPORTER_OTLP_ENDPOINT when provided.",
	).Envar("OTEL_EXPORTER_OTLP_ENDPOINT").Default("").String()
	otelProtocol = kingpin.Flag(
		"otel.otlp.protocol",
		"OTLP transport protocol. Values: grpc, http/protobuf.",
	).Envar("OTEL_EXPORTER_OTLP_PROTOCOL").Default("grpc").String()
	otelInterval = kingpin.Flag(
		"otel.otlp.interval",
		"OTLP push interval.",
	).Default("15s").Duration()
	otelTraceSampleRate = kingpin.Flag(
		"otel.trace-sample-rate",
		"Trace sample rate (0 < rate <= 1).",
	).Default("1.0").Float64()
	otelServiceName = kingpin.Flag(
		"otel.service-name",
		"OTel service.name resource attribute.",
	).Envar("OTEL_SERVICE_NAME").Default("ovs-exporter").String()

	toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":10054")
)

// buildHandler wires the HTTP routes served by the exporter: the OTel
// Prometheus handler at metricsPath, healthz/readyz probes, and the
// exporter-toolkit landing page at "/" (unless metricsPath itself is "/").
// readyz is currently registered with no checks (always 200) — actual
// checks for libovsdb connectivity and unixctl scrape freshness will be
// wired when those data sources land in main.
func buildHandler(res *otel.Result, metricsPath string) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.Handle(metricsPath, res.PromHandler)
	mux.Handle("/healthz", probes.Health())
	mux.Handle("/readyz", probes.Ready(nil))

	if metricsPath != "/" {
		landing, err := web.NewLandingPage(web.LandingConfig{
			Name:        "OVS Exporter",
			Description: "OTel-native Prometheus exporter for Open vSwitch and OVN",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{Address: metricsPath, Text: "Metrics"},
				{Address: "/healthz", Text: "Health"},
				{Address: "/readyz", Text: "Readiness"},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("creating landing page: %w", err)
		}
		mux.Handle("/", landing)
	}
	return mux, nil
}

func main() {
	promslogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.Version(version.Print("ovs-exporter"))
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()

	logger := promslog.New(promslogConfig)

	runtime.GOMAXPROCS(*maxProcs)

	if *otelOTLPEndpoint != "" {
		if err := os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", *otelOTLPEndpoint); err != nil {
			logger.Error("Failed to set OTEL_EXPORTER_OTLP_ENDPOINT", "err", err)
			os.Exit(1)
		}
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	otelResult, err := otel.Setup(rootCtx, logger, otel.Config{
		ServiceName:     *otelServiceName,
		ServiceVersion:  version.Version,
		Protocol:        *otelProtocol,
		OTLPInterval:    *otelInterval,
		MetricsExporter: *otelMetricsExporter,
		TracesExporter:  *otelTracesExporter,
		LogsExporter:    *otelLogsExporter,
		TraceSampleRate: *otelTraceSampleRate,
	})
	if err != nil {
		logger.Error("Failed to set up OTel pipeline", "err", err)
		os.Exit(1)
	}
	if otelResult.Logger != nil {
		logger = otelResult.Logger
	}

	// TODO(T5/T8/T9/T11): discover unixctl sockets, connect libovsdb clients,
	// start the TTL scraper, instantiate probes that gate readyz on those
	// states.

	mux, err := buildHandler(otelResult, *metricsPath)
	if err != nil {
		logger.Error("Failed to build HTTP handler", "err", err)
		os.Exit(1)
	}

	logger.Info("Starting ovs-exporter", "version", version.Info())
	logger.Info("Build context", "build_context", version.BuildContext())
	if u, err := user.Current(); err == nil && u.Uid == "0" {
		logger.Warn("ovs-exporter is running as root. Run as a dedicated user in the openvswitch group instead.")
	}
	logger.Debug("Go MAXPROCS", "procs", runtime.GOMAXPROCS(0))

	server := &http.Server{Handler: mux}
	serveErrCh := make(chan error, 1)
	go func() {
		err := web.ListenAndServe(server, toolkitFlags, logger)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
			return
		}
		close(serveErrCh)
	}()

	exitCode := 0
	select {
	case err := <-serveErrCh:
		if err != nil {
			logger.Error("ListenAndServe failed", "err", err)
			exitCode = 1
		}
	case <-rootCtx.Done():
		logger.Info("Shutdown signal received")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "HTTP shutdown error: %v\n", err)
	}
	if err := otelResult.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "OTel shutdown error: %v\n", err)
	}
	os.Exit(exitCode)
}
