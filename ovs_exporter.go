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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/barnes-c/ovs-exporter/collector"
	"github.com/barnes-c/ovs-exporter/internal/datasource"
	"github.com/barnes-c/ovs-exporter/internal/otel"
	"github.com/barnes-c/ovs-exporter/internal/ovsdb"
	"github.com/barnes-c/ovs-exporter/internal/probes"
	"github.com/barnes-c/ovs-exporter/internal/scrape"
	"github.com/barnes-c/ovs-exporter/internal/unixctl"
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

	ovsDBSocket = kingpin.Flag(
		"ovs.db-socket",
		"libovsdb endpoint for the Open_vSwitch database.",
	).Default("unix:/var/run/openvswitch/db.sock").String()
	ovsRunDir = kingpin.Flag(
		"ovs.run-dir",
		"Directory containing the ovs-vswitchd unix control socket and pid file.",
	).Default("/var/run/openvswitch").String()
	cacheTTL = kingpin.Flag(
		"cache.ttl",
		"TTL between successive unixctl scrapes. ovsdb data is monitor-cached and ignores this.",
	).Default("15s").Duration()

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
	webPrometheus = kingpin.Flag(
		"web.prometheus",
		"Serve the Prometheus scrape endpoint at --web.telemetry-path. Disable for OTLP-push-only deployments.",
	).Default("true").Bool()

	toolkitFlags = kingpinflag.AddFlags(kingpin.CommandLine, ":10054")
)

// buildHandler wires the HTTP routes served by the exporter: the OTel
// Prometheus handler at metricsPath, healthz/readyz probes, and the
// exporter-toolkit landing page at "/" (unless metricsPath itself is "/").
func buildHandler(res *otel.Result, metricsPath string, readyChecks map[string]probes.Checker) (http.Handler, error) {
	mux := http.NewServeMux()
	if res.PromHandler != nil {
		mux.Handle(metricsPath, res.PromHandler)
	}
	mux.Handle("/healthz", probes.Health())
	mux.Handle("/readyz", probes.Ready(readyChecks))

	if metricsPath != "/" {
		links := []web.LandingLinks{}
		if res.PromHandler != nil {
			links = append(links, web.LandingLinks{Address: metricsPath, Text: "Metrics"})
		}
		links = append(links,
			web.LandingLinks{Address: "/healthz", Text: "Health"},
			web.LandingLinks{Address: "/readyz", Text: "Readiness"},
		)
		landing, err := web.NewLandingPage(web.LandingConfig{
			Name:        "OVS Exporter",
			Description: "OTel-native Prometheus exporter for Open vSwitch (OVS)",
			Version:     version.Info(),
			Links:       links,
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

	// Propagate the flag values into the env vars autoexport reads
	// directly. Without this, autoexport falls back to its own defaults
	// (e.g. http/protobuf for OTLP) when the user supplied a value only
	// via --otel.* flags.
	envFromFlag := map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": *otelOTLPEndpoint,
		"OTEL_EXPORTER_OTLP_PROTOCOL": *otelProtocol,
		"OTEL_METRIC_EXPORT_INTERVAL": fmt.Sprintf("%d", otelInterval.Milliseconds()),
	}
	for k, v := range envFromFlag {
		if v == "" {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			logger.Error("Failed to set env var", "key", k, "err", err)
			os.Exit(1)
		}
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	otelResult, err := otel.Setup(rootCtx, logger, otel.Config{
		ServiceName:       *otelServiceName,
		ServiceVersion:    version.Version,
		Protocol:          *otelProtocol,
		OTLPInterval:      *otelInterval,
		MetricsExporter:   *otelMetricsExporter,
		TracesExporter:    *otelTracesExporter,
		LogsExporter:      *otelLogsExporter,
		TraceSampleRate:   *otelTraceSampleRate,
		PrometheusEnabled: *webPrometheus,
	})
	if err != nil {
		logger.Error("Failed to set up OTel pipeline", "err", err)
		os.Exit(1)
	}
	if otelResult.Logger != nil {
		logger = otelResult.Logger
	}

	// Connect libovsdb (best-effort). A failed connect leaves OVS data
	// nil — collectors that read it will simply emit no points until the
	// next time we try (operator restart). The wrapper has its own
	// reconnect loop once Connect succeeds.
	connectCtx, connectCancel := context.WithTimeout(rootCtx, 10*time.Second)
	ovsClient, err := ovsdb.Connect(connectCtx, ovsdb.Config{
		Endpoint: *ovsDBSocket,
		Logger:   logger.With("component", "ovsdb"),
		Tracer:   otelResult.Tracer,
	})
	connectCancel()
	if err != nil {
		logger.Warn("ovsdb client connect failed; OVS-table metrics will be empty until restart",
			"endpoint", *ovsDBSocket, "err", err)
		ovsClient = nil
	}

	// unixctl client + ovs scraper. The client is lazy-connected; the
	// scraper's first refresh will dial the socket.
	unixClient, err := unixctl.New(unixctl.Config{
		RunDir: *ovsRunDir,
		Daemon: "ovs-vswitchd",
		Logger: logger.With("component", "unixctl-ovs"),
	})
	if err != nil {
		logger.Error("Failed to create unixctl client", "err", err)
		os.Exit(1)
	}

	ovsScraper, err := scrape.New(scrape.Config[unixctl.OVSSnapshot]{
		Name:     "ovs",
		Interval: *cacheTTL,
		Refresh:  datasource.NewOVSRefresh(unixClient, logger.With("component", "scrape-ovs")),
		Logger:   logger.With("component", "scrape-ovs"),
		Tracer:   otelResult.Tracer,
	})
	if err != nil {
		logger.Error("Failed to create OVS scraper", "err", err)
		os.Exit(1)
	}

	src := datasource.NewDataSource(ovsClient, ovsScraper)

	group, err := collector.NewGroup(logger)
	if err != nil {
		logger.Error("Failed to instantiate collectors", "err", err)
		os.Exit(1)
	}
	if err := group.RegisterAll(otelResult.Meter, src); err != nil {
		logger.Error("Failed to register collectors", "err", err)
		os.Exit(1)
	}
	logger.Info("Collectors registered", "names", group.Names())

	scrapeCtx, scrapeCancel := context.WithCancel(rootCtx)
	go ovsScraper.Run(scrapeCtx)

	readyChecks := buildReadyChecks(ovsClient, ovsScraper, *cacheTTL)

	mux, err := buildHandler(otelResult, *metricsPath, readyChecks)
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

	server := &http.Server{Handler: otelhttp.NewHandler(mux, "ovs-exporter")}
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

	scrapeCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "HTTP shutdown error: %v\n", err)
	}
	if err := group.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Collector close error: %v\n", err)
	}
	if err := unixClient.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "unixctl close error: %v\n", err)
	}
	if ovsClient != nil {
		if err := ovsClient.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "ovsdb close error: %v\n", err)
		}
	}
	if err := otelResult.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "OTel shutdown error: %v\n", err)
	}
	os.Exit(exitCode)
}

// buildReadyChecks wires the readyz dependency checks. Each subsystem
// owns its own health verdict (Client.Healthy, Scraper.Stale); this
// function just decides which checks to expose under what name and what
// staleness threshold counts as not-ready.
func buildReadyChecks(ovsClient *ovsdb.Client, ovsScraper *scrape.Scraper[unixctl.OVSSnapshot], ttl time.Duration) map[string]probes.Checker {
	return map[string]probes.Checker{
		"ovsdb": probes.CheckerFunc(func(context.Context) error {
			return ovsClient.Healthy()
		}),
		"unixctl-ovs": probes.CheckerFunc(func(context.Context) error {
			return ovsScraper.Stale(3 * ttl)
		}),
	}
}
