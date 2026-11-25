package hlc

import (
	"sync"
	"time"
)

const logicalBits = 16

type Clock struct {
	mu   sync.Mutex
	last int64
	now  func() int64 // wall clock in ms; swappable for tests
}

func New() *Clock {
	return &Clock{now: func() int64 { return time.Now().UnixMilli() }}
}

// NewWithSource lets tests drive the wall clock.
func NewWithSource(now func() int64) *Clock { return &Clock{now: now} }

// Now returns a timestamp for a local event (a "send" in HLC terms).
func (c *Clock) Now() int64 { return c.Update(0) }

// Update merges a timestamp observed from elsewhere (e.g. the version currently stored
// in the row, which may have been written by another node) and returns a new timestamp
// strictly greater than: the local wall clock, every timestamp this clock has issued,
// and `seen`. This is the HLC "receive" rule written as one max():
//
//	physical time moved on      -> wall<<16 wins, counter resets to 0
//	same ms / clock went back   -> last+1 wins, counter increments
//	someone else is ahead       -> seen+1 wins, we catch up to them
func (c *Clock) Update(seen int64) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	next := max(c.now()<<logicalBits, c.last+1, seen+1)
	c.last = next
	return next
}

// WallMillis extracts the physical part (useful in logs and tests).
func WallMillis(ts int64) int64 { return ts >> logicalBits }

// Logical extracts the counter part.
func Logical(ts int64) int64 { return ts & (1<<logicalBits - 1) }
