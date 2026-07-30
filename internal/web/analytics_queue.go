package web

import (
	"context"
	"log/slog"
	"time"
)

// analyticsWriteWorkers/analyticsQueueSize bound how many analytics DB
// writes (page views, conversion events) can be in flight or queued at
// once, so a traffic spike can't spawn unbounded goroutines that each hold
// a DB connection for up to analyticsWriteTimeout — starving the pool that
// real request handlers need.
const (
	analyticsWriteWorkers = 8
	analyticsQueueSize    = 512
	analyticsWriteTimeout = 5 * time.Second
)

// analyticsJob is a queued analytics DB write, run with a fresh
// context.WithTimeout(analyticsWriteTimeout) since it outlives the request
// that queued it.
type analyticsJob func(ctx context.Context)

// analyticsQueue is a small fixed-size worker pool for analytics writes.
// submit never blocks the caller: if every worker is busy and the queue is
// full, the write is dropped rather than piling up more goroutines.
type analyticsQueue struct {
	jobs chan analyticsJob
}

func newAnalyticsQueue() *analyticsQueue {
	q := &analyticsQueue{jobs: make(chan analyticsJob, analyticsQueueSize)}
	for i := 0; i < analyticsWriteWorkers; i++ {
		go q.run()
	}
	return q
}

func (q *analyticsQueue) run() {
	for job := range q.jobs {
		ctx, cancel := context.WithTimeout(context.Background(), analyticsWriteTimeout)
		job(ctx)
		cancel()
	}
}

func (q *analyticsQueue) submit(job analyticsJob) {
	select {
	case q.jobs <- job:
	default:
		slog.Warn("analytics queue full, dropping write")
	}
}
