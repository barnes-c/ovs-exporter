package collector

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/alecthomas/kingpin/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
	"github.com/barnes-c/ovs-exporter/internal/unixctl"
)

type Collector interface {
	Name() string
	Register(meter metric.Meter, src DataSource) error
	Close() error
}

// DepCheck reports whether a collector's data dependency is currently
// available. Declared once at registerCollector time and consulted by
// Group when emitting ovs.collector.up; lets alerts distinguish "no
// data" from "actually zero" without each collector implementing the
// same predicate.
type DepCheck func(DataSource) bool

// OVSViewAvailable is the dep check shared by every collector that reads
// from the libovsdb cache.
var OVSViewAvailable DepCheck = func(s DataSource) bool {
	return s != nil && s.OVS() != nil
}

// UnixctlHas builds a DepCheck that requires a UnixCtlOVS snapshot plus a
// specific sub-field within it. Saves repeating the nil dance in every
// unixctl-backed collector's init().
func UnixctlHas(present func(*unixctl.OVSSnapshot) bool) DepCheck {
	return func(s DataSource) bool {
		if s == nil {
			return false
		}
		snap := s.UnixCtlOVS()
		return snap != nil && present(snap)
	}
}

// DataSource is the read API collectors use to observe state. Each
// accessor returns nil when the underlying transport is not configured
// (e.g. UnixCtlOVS returns nil before the first successful scrape, OVS
// returns nil when the libovsdb client failed to connect at startup).
type DataSource interface {
	OVS() OVSView
	// UnixCtlOVS returns the most recently scraped ovs-vswitchd appctl
	// snapshot. Returns nil before the first successful scrape;
	// individual snapshot fields may also be nil if a particular parser
	// has not yet succeeded.
	UnixCtlOVS() *unixctl.OVSSnapshot
}

// OVSView is the read API over the Open_vSwitch DB cache. Methods correspond
// to tables the collectors iterate; each takes a callback invoked once per
// row. Iteration order is unspecified. Calling on a nil receiver, or when
// the underlying cache is not populated, is safe and yields no callbacks.
type OVSView interface {
	Bridges(fn func(*ovsmodel.Bridge))
	Ports(fn func(*ovsmodel.Port))
	Interfaces(fn func(*ovsmodel.Interface))
	OpenvSwitch() *ovsmodel.OpenvSwitch
}

const (
	DefaultEnabled  = true
	DefaultDisabled = false
)

var (
	factoriesMu      sync.Mutex
	factories        = make(map[string]func(logger *slog.Logger) (Collector, error))
	collectorDeps    = make(map[string]DepCheck)
	collectorState   = make(map[string]*bool)
	forcedCollectors = make(map[string]bool)
)

// registrar carries the OTel callback handle that every collector needs to
// unregister on shutdown. Collectors embed it so they don't each have to
// reimplement the same Close().
type registrar struct {
	registration metric.Registration
}

// Close unregisters the embedded callback. Safe to call before Register.
func (r *registrar) Close() error {
	if r.registration == nil {
		return nil
	}
	return r.registration.Unregister()
}

// registerCollector adds a sub-collector to the registry and declares its
// --collector.<name> flag. Called from init() in each collector file. The
// flag's Action records the collector as "forced" so DisableDefaultCollectors
// knows to leave operator-set values alone. dep is the data-availability
// predicate consulted by the ovs.collector.up gauge.
func registerCollector(
	name string,
	isDefaultEnabled bool,
	factory func(logger *slog.Logger) (Collector, error),
	dep DepCheck,
) {
	helpDefaultState := "disabled"
	if isDefaultEnabled {
		helpDefaultState = "enabled"
	}
	flagName := fmt.Sprintf("collector.%s", name)
	flagHelp := fmt.Sprintf("Enable the %s collector (default: %s).", name, helpDefaultState)
	defaultValue := fmt.Sprintf("%v", isDefaultEnabled)

	flag := kingpin.Flag(flagName, flagHelp).
		Default(defaultValue).
		Action(collectorFlagAction(name)).
		Bool()

	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	collectorState[name] = flag
	factories[name] = factory
	collectorDeps[name] = dep
}

// collectorFlagAction tags a collector as explicitly set by the operator so
// DisableDefaultCollectors does not override it.
func collectorFlagAction(name string) func(*kingpin.ParseContext) error {
	return func(*kingpin.ParseContext) error {
		factoriesMu.Lock()
		forcedCollectors[name] = true
		factoriesMu.Unlock()
		return nil
	}
}

// DisableDefaultCollectors flips every non-explicitly-set collector to
// disabled. Used by --collector.disable-defaults to switch into opt-in mode.
func DisableDefaultCollectors() {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	for name, state := range collectorState {
		if !forcedCollectors[name] {
			*state = false
		}
	}
}

// Registered returns the names of every collector known to the registry,
// regardless of enable state. Sorted alphabetically; safe for concurrent use.
func Registered() []string {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Group is the live set of enabled sub-collectors. It owns each instance and
// is the surface main.go uses to register everything against a Meter and to
// close cleanly at shutdown.
type Group struct {
	log        *slog.Logger
	collectors map[string]Collector
	deps       map[string]DepCheck
	src        DataSource
	upGauge    metric.Int64ObservableGauge
	upCallback metric.Registration
}

// NewGroup instantiates every enabled collector. If filters is non-empty,
// the result is restricted to that subset; filtering an unknown or disabled
// collector is an error.
func NewGroup(logger *slog.Logger, filters ...string) (*Group, error) {
	factoriesMu.Lock()
	defer factoriesMu.Unlock()

	filterSet := make(map[string]bool, len(filters))
	for _, f := range filters {
		state, ok := collectorState[f]
		if !ok {
			return nil, fmt.Errorf("unknown collector: %s", f)
		}
		if !*state {
			return nil, fmt.Errorf("disabled collector: %s", f)
		}
		filterSet[f] = true
	}

	out := make(map[string]Collector)
	deps := make(map[string]DepCheck)
	for name, state := range collectorState {
		if !*state {
			continue
		}
		if len(filterSet) > 0 && !filterSet[name] {
			continue
		}
		c, err := factories[name](logger.With("collector", name))
		if err != nil {
			return nil, fmt.Errorf("instantiate %s: %w", name, err)
		}
		out[name] = c
		deps[name] = collectorDeps[name]
	}
	return &Group{log: logger, collectors: out, deps: deps}, nil
}

// RegisterAll calls Register on every collector in the group, then registers
// a shared ovs.collector.up gauge whose value is driven by each collector's
// registry-declared DepCheck.
func (g *Group) RegisterAll(meter metric.Meter, src DataSource) error {
	g.src = src
	for name, c := range g.collectors {
		if err := c.Register(meter, src); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}

	if len(g.collectors) == 0 {
		return nil
	}

	var err error
	g.upGauge, err = meter.Int64ObservableGauge(
		"ovs.collector.up",
		metric.WithDescription("1 if the collector's data dependency is currently available; 0 otherwise."),
	)
	if err != nil {
		return fmt.Errorf("create ovs.collector.up: %w", err)
	}
	g.upCallback, err = meter.RegisterCallback(g.observeUp, g.upGauge)
	if err != nil {
		return fmt.Errorf("register ovs.collector.up callback: %w", err)
	}
	return nil
}

func (g *Group) observeUp(_ context.Context, o metric.Observer) error {
	for name := range g.collectors {
		v := int64(0)
		if dep := g.deps[name]; dep != nil && dep(g.src) {
			v = 1
		}
		o.ObserveInt64(g.upGauge, v, metric.WithAttributes(attribute.String("collector", name)))
	}
	return nil
}

// Close unregisters the shared up callback and closes every collector.
func (g *Group) Close() error {
	var errs []error
	if g.upCallback != nil {
		if err := g.upCallback.Unregister(); err != nil {
			errs = append(errs, fmt.Errorf("unregister ovs.collector.up: %w", err))
		}
	}
	for name, c := range g.collectors {
		if err := c.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Names returns the enabled collector names in sorted order. Used for filter
// validation and landing-page logging.
func (g *Group) Names() []string {
	out := make([]string, 0, len(g.collectors))
	for n := range g.collectors {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
