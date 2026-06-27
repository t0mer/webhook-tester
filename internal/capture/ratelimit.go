package capture

import (
	"sync"
	"time"
)

// rateLimiter enforces a per-token sliding-window request cap. The limit is
// derived from the token's timeout as 100/timeout requests per minute; a
// timeout of 0 means unlimited.
type rateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	hits   map[string][]time.Time
	now    func() time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		window: time.Minute,
		hits:   make(map[string][]time.Time),
		now:    time.Now,
	}
}

// allow reports whether a request for tokenID is permitted given its timeout.
// limitPerMinute = 100/timeout (timeout <= 0 => unlimited).
func (rl *rateLimiter) allow(tokenID string, timeout int) bool {
	if timeout <= 0 {
		return true
	}
	limit := 100 / timeout
	if limit < 1 {
		limit = 1
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	cutoff := now.Add(-rl.window)

	kept := rl.hits[tokenID][:0]
	for _, t := range rl.hits[tokenID] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	if len(kept) >= limit {
		rl.hits[tokenID] = kept
		return false
	}
	rl.hits[tokenID] = append(kept, now)
	return true
}
