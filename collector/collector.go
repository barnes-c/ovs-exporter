package collector

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/alecthomas/kingpin/v2"
	"go.opentelemetry.io/otel/metric"

	"github.com/barnes-c/ovs-exporter/internal/ovsdb/ovsmodel"
)

// Collector is the interface every sub-collector implements.
type Collector interface {
	// Name returns the collector's short name, e.g. "ovs-bridges". It is
	// the same string used in the --collector.<name> flag.
	Name() string
	// Register creates the OTel instruments owned by this collector on the
	// given Meter and wires their callbacks. It is called exactly once at
	// startup, before /metrics is served.
	Register(meter metric.Meter, src DataSource) error
	// Close releases any per-collector resources. The shared DataSource is
	// closed by the orchestrator, not by collectors.
	Close() error
}

// DataSource is the read API collectors use to observe state. Each accessor
// returns an interface that the corresponding internal package implements
// Accessors return nil when the underlying transport is not configured
// e.g. OVNNB() is nil on hosts where --ovn.nb-addresses is empty.
//
// The view interfaces below are empty placeholders. Concrete methods are
// added by the collectors that need them (T6 adds Bridges to OVSView,
// T7 adds Interfaces, etc.). The libovsdb-backed implementations live in
// internal/ovsdb (T5), the unixctl-backed implementations in internal/scrape
// (T9).
type DataSource interface {
	OVS() OVSView
	OVNNB() OVNNBView
	OVNSB() OVNSBView
	UnixCtlOVS() UnixCtlSnapshot
	UnixCtlNorthd() UnixCtlSnapshot
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

// OVN view placeholders, populated by T17 when the OVN libovsdb clients land.
type (
	OVNNBView       interface{}
	OVNSBView       interface{}
	UnixCtlSnapshot interface{}
)

// Default-state shorthand used by registerCollector callers.
const (
	DefaultEnabled  = true
	DefaultDisabled = false
)

var (
	factoriesMu      sync.Mutex
	factories        = make(map[string]func(logger *slog.Logger) (Collector, error))
	collectorState   = make(map[string]*bool)
	forcedCollectors = make(map[string]bool)
)

// registerCollector adds a sub-collector to the registry and declares its
// --collector.<name> flag. Called from init() in each collector file. The
// flag's Action records the collector as "forced" so DisableDefaultCollectors
// knows to leave operator-set values alone.
func registerCollector(name string, isDefaultEnabled bool, factory func(logger *slog.Logger) (Collector, error)) {
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
	}
	return &Group{log: logger, collectors: out}, nil
}

// RegisterAll calls Register on every collector in the group, passing the
// supplied Meter and DataSource. Stops and returns on the first error.
func (g *Group) RegisterAll(meter metric.Meter, src DataSource) error {
	for name, c := range g.collectors {
		if err := c.Register(meter, src); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}
	return nil
}

// Close calls Close on every collector and joins their errors.
func (g *Group) Close() error {
	var errs []error
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
