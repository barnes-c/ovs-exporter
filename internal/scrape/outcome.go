package scrape

import "time"

// Outcome is the result of a single TTL refresh. It is replaced atomically
// on every tick so the meta-collector that exports scrape.{duration,success}
// always reads a consistent pair.
type Outcome struct {
	Time     time.Time
	Duration time.Duration
	Success  bool
	Err      error
}
