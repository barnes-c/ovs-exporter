package collector

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"

	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// fakeCollector is a no-op Collector used by registry-level tests so they
// don't depend on libovsdb / unixctl.
type fakeCollector struct {
	name         string
	registerErr  error
	closeErr     error
	registerHits int
	closeHits    int
}

func (f *fakeCollector) Name() string { return f.name }
func (f *fakeCollector) Register(metric.Meter, DataSource) error {
	f.registerHits++
	return f.registerErr
}
func (f *fakeCollector) Close() error { f.closeHits++; return f.closeErr }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGroup_RegisterAll_HitsEveryCollector(t *testing.T) {
	a := &fakeCollector{name: "a"}
	b := &fakeCollector{name: "b"}
	g := &Group{log: discardLogger(), collectors: map[string]Collector{"a": a, "b": b}}

	if err := g.RegisterAll(noopmetric.NewMeterProvider().Meter("test"), nil); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	if a.registerHits != 1 || b.registerHits != 1 {
		t.Errorf("each collector should be registered once, got a=%d b=%d", a.registerHits, b.registerHits)
	}
}

func TestGroup_RegisterAll_EmitsUpGauge(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	g := &Group{
		log: discardLogger(),
		collectors: map[string]Collector{
			"healthy":   &fakeCollector{name: "healthy"},
			"unhealthy": &fakeCollector{name: "unhealthy"},
		},
		deps: map[string]DepCheck{
			"healthy":   func(DataSource) bool { return true },
			"unhealthy": func(DataSource) bool { return false },
		},
	}

	if err := g.RegisterAll(mp.Meter("test"), nil); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	t.Cleanup(func() { _ = g.Close() })

	got := collectInt64Gauges(t, reader)
	cases := map[string]int64{
		"ovs.collector.up{collector=healthy}":   1,
		"ovs.collector.up{collector=unhealthy}": 0,
	}
	for k, want := range cases {
		if g, ok := got[k]; !ok {
			t.Errorf("missing metric %q (got: %v)", k, got)
		} else if g != want {
			t.Errorf("metric %q = %d, want %d", k, g, want)
		}
	}
}

func TestGroup_RegisterAll_PropagatesError(t *testing.T) {
	want := errors.New("boom")
	a := &fakeCollector{name: "a", registerErr: want}
	g := &Group{log: discardLogger(), collectors: map[string]Collector{"a": a}}

	err := g.RegisterAll(noopmetric.NewMeterProvider().Meter("test"), nil)
	if err == nil || !errors.Is(err, want) {
		t.Errorf("err = %v, want wraps %v", err, want)
	}
}

func TestGroup_Close_JoinsErrors(t *testing.T) {
	errA := errors.New("a-close")
	errB := errors.New("b-close")
	a := &fakeCollector{name: "a", closeErr: errA}
	b := &fakeCollector{name: "b", closeErr: errB}
	g := &Group{log: discardLogger(), collectors: map[string]Collector{"a": a, "b": b}}

	err := g.Close()
	if err == nil || !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Errorf("Close should wrap both errors, got %v", err)
	}
	if a.closeHits != 1 || b.closeHits != 1 {
		t.Errorf("each collector should be closed once, got a=%d b=%d", a.closeHits, b.closeHits)
	}
}

func TestGroup_Names_Sorted(t *testing.T) {
	g := &Group{log: discardLogger(), collectors: map[string]Collector{
		"zebra":  &fakeCollector{name: "zebra"},
		"alpha":  &fakeCollector{name: "alpha"},
		"mango":  &fakeCollector{name: "mango"},
		"banana": &fakeCollector{name: "banana"},
	}}
	got := g.Names()
	want := []string{"alpha", "banana", "mango", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
}

// TestDisableDefaultCollectors_RespectsForced verifies the operator-override
// semantics: collectors explicitly toggled on the CLI must not be flipped by
// --collector.disable-defaults.
func TestDisableDefaultCollectors_RespectsForced(t *testing.T) {
	resetRegistryForTest(t)

	on1, on2 := true, true
	collectorState["default-on-1"] = &on1
	collectorState["default-on-2"] = &on2
	forcedCollectors["default-on-2"] = true // operator-set on CLI

	DisableDefaultCollectors()

	if on1 {
		t.Error("default-on-1 should be disabled by DisableDefaultCollectors")
	}
	if !on2 {
		t.Error("default-on-2 was operator-set; DisableDefaultCollectors must not flip it")
	}
}

// TestRegisterCollector_EndToEnd exercises the registration path through
// NewGroup so registerCollector + collectorFlagAction are covered.
func TestRegisterCollector_EndToEnd(t *testing.T) {
	resetRegistryForTest(t)

	const name = "__t4_smoke"
	registerCollector(name, DefaultEnabled, func(*slog.Logger) (Collector, error) {
		return &fakeCollector{name: name}, nil
	}, func(DataSource) bool { return true })

	if _, ok := factories[name]; !ok {
		t.Error("registerCollector did not populate factories")
	}
	state, ok := collectorState[name]
	if !ok {
		t.Fatal("registerCollector did not populate collectorState")
	}
	// kingpin populates the *bool only when Parse() runs. Simulate the
	// default being applied so NewGroup sees the collector as enabled.
	*state = true

	g, err := NewGroup(discardLogger())
	if err != nil {
		t.Fatalf("NewGroup: %v", err)
	}
	got := g.Names()
	if len(got) != 1 || got[0] != name {
		t.Errorf("Names = %v, want [%q]", got, name)
	}
}

// resetRegistryForTest snapshots the package-level registry maps, clears
// them for the test, and restores them in cleanup. Without this the global
// state from collector init() functions would leak across tests.
//
// Note: kingpin flag registrations from registerCollector still leak into
// the global CommandLine; tests that exercise registerCollector must use
// unique flag names.
func resetRegistryForTest(t *testing.T) {
	t.Helper()
	factoriesMu.Lock()
	defer factoriesMu.Unlock()

	savedFactories := factories
	savedDeps := collectorDeps
	savedState := collectorState
	savedForced := forcedCollectors
	factories = make(map[string]func(logger *slog.Logger) (Collector, error))
	collectorDeps = make(map[string]DepCheck)
	collectorState = make(map[string]*bool)
	forcedCollectors = make(map[string]bool)

	t.Cleanup(func() {
		factoriesMu.Lock()
		defer factoriesMu.Unlock()
		factories = savedFactories
		collectorDeps = savedDeps
		collectorState = savedState
		forcedCollectors = savedForced
	})
}
