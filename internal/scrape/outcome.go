package scrape

import "time"

// Outcome is the result of a single TTL refresh. It is replaced atomically
// on every tick
type Outcome struct {
	Time     time.Time
	Duration time.Duration
	Success  bool
	Err      error
}
