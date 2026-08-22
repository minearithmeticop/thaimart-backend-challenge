// Package report runs background reporting jobs. The user count reporter
// satisfies the requirement to log the total number of users every 10
// seconds (configurable via REPORT_EVERY).
package report

import (
	"context"
	"log/slog"
	"time"

	"github.com/minearithmeticop/thaimart-backend-challenge/internal/app"
)

// UserCountReporter periodically logs the total number of users.
type UserCountReporter struct {
	repo     app.UserRepository
	interval time.Duration
	log      *slog.Logger
}

func NewUserCountReporter(repo app.UserRepository, interval time.Duration, log *slog.Logger) *UserCountReporter {
	return &UserCountReporter{repo: repo, interval: interval, log: log}
}

// Run blocks until ctx is cancelled, reporting once per interval. It
// reports immediately on start so operators see a heartbeat without
// waiting a full interval. A flaky database never stops the loop:
// failures are logged and retried on the next tick.
func (r *UserCountReporter) Run(ctx context.Context) {
	r.reportOnce(ctx)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("user count reporter stopped")
			return
		case <-ticker.C:
			r.reportOnce(ctx)
		}
	}
}

// reportOnce is split out so tests can exercise a single report directly.
func (r *UserCountReporter) reportOnce(ctx context.Context) {
	count, err := r.repo.Count(ctx)
	if err != nil {
		r.log.Warn("user count report failed", "err", err)
		return
	}
	r.log.Info("user count report", "total_users", count)
}
