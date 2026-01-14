package daemon

import (
	"context"
	"fmt"
	"time"

	"github.com/afterdarksys/apiproxyd/pkg/cache"
)

// Scheduler handles background tasks like cache cleanup
type Scheduler struct {
	cache    cache.Cache
	interval time.Duration
	ticker   *time.Ticker
	done     chan struct{}
}

// NewScheduler creates a new background scheduler
func NewScheduler(c cache.Cache, interval time.Duration) *Scheduler {
	if interval <= 0 {
		interval = 1 * time.Hour // default to hourly cleanup
	}

	return &Scheduler{
		cache:    c,
		interval: interval,
		done:     make(chan struct{}),
	}
}

// Start begins the background scheduler
func (s *Scheduler) Start(ctx context.Context) {
	s.ticker = time.NewTicker(s.interval)

	go func() {
		// Run initial cleanup
		s.runCleanup()

		for {
			select {
			case <-s.ticker.C:
				s.runCleanup()
			case <-s.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the background scheduler
func (s *Scheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.done)
}

// runCleanup performs cache cleanup
func (s *Scheduler) runCleanup() {
	start := time.Now()

	// Clean up expired entries in database cache
	if cleaner, ok := s.cache.(interface{ CleanupExpired() error }); ok {
		if err := cleaner.CleanupExpired(); err != nil {
			fmt.Printf("Cache cleanup error: %v\n", err)
			return
		}
	}

	// Clean up expired entries in memory cache (if layered)
	if layered, ok := s.cache.(*cache.LayeredCache); ok {
		layered.CleanupExpired()
	}

	duration := time.Since(start)
	fmt.Printf("Cache cleanup completed in %v\n", duration)
}

// RunNow triggers an immediate cleanup
func (s *Scheduler) RunNow() {
	s.runCleanup()
}
