// Package sample is a test fixture exercising go-metricy-extract.
//
// The file is intentionally shaped like a small real service: a handful of
// package-level Prometheus metric declarations (counter / gauge / histogram /
// summary, scalar and vec variants, both prometheus.New* and promauto.New*
// forms) each carrying @metric and @label annotations in their doc comments.
// Golden-file tests pin the extractor's JSON output against the shape produced
// from this fixture — any drift in sort order, field names, or metadata
// extraction surfaces as a test diff.
package sample

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HttpRequestsTotal counts incoming HTTP requests.
//
// @metric description Total incoming HTTP requests across all endpoints.
// @metric calculation Incremented in LoggingMiddleware on each completed request.
// @label method HTTP method: GET, POST, PUT, DELETE
// @label status_code HTTP response status code
var HttpRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests processed",
	},
	[]string{"method", "status_code"},
)

// ActiveConnections tracks current active connections.
//
// @metric description Number of currently active client connections.
// @metric calculation Incremented on connect, decremented on disconnect.
var ActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "active_connections",
	Help: "Number of active connections",
})

// RequestDuration measures HTTP request latency.
//
// @metric description HTTP request duration in seconds.
// @metric calculation Observed in LoggingMiddleware after downstream returns.
// @label endpoint Route template of the matched endpoint
var RequestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name: "http_request_duration_seconds",
		Help: "HTTP request duration",
	},
	[]string{"endpoint"},
)

// TotalErrors tracks unrecoverable errors across the service.
//
// @metric description Total unrecoverable errors emitted by any handler.
// @metric calculation Incremented whenever a handler returns an error result.
var TotalErrors = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "errors_total",
	Help: "Total errors across all handlers",
})

// RequestSize observes payload size distribution.
//
// @metric description Distribution of inbound request payload sizes in bytes.
// @metric calculation Observed in middleware after request body is read.
var RequestSize = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name: "http_request_size_bytes",
	Help: "Inbound request body size",
})

// QueueDepth reports the current depth of the per-worker queue.
//
// @metric description Current number of items pending in a worker's queue.
// @metric calculation Updated by workers on enqueue/dequeue.
// @label worker_id Identifier of the worker whose queue is being measured
var QueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name: "worker_queue_depth",
	Help: "Current queue depth per worker",
}, []string{"worker_id"})

// CacheHitsRatio approximates cache hit ratio via summary.
//
// @metric description Running summary of cache hit ratio observed over the last 1m window.
// @metric calculation Observed in Cache.Get/Set hooks with hit=1 or miss=0.
var CacheHitsRatio = prometheus.NewSummary(prometheus.SummaryOpts{
	Name: "cache_hit_ratio",
	Help: "Cache hit ratio (running summary)",
})

// OperationLatency observes per-operation latency with labels.
//
// @metric description Per-operation latency summary across all RPC methods.
// @metric calculation Observed in RPC middleware at response time.
// @label operation Name of the RPC method being observed
var OperationLatency = prometheus.NewSummaryVec(prometheus.SummaryOpts{
	Name: "rpc_operation_latency_seconds",
	Help: "Per-operation RPC latency",
}, []string{"operation"})

// TasksProcessed counts completed tasks (promauto counter).
//
// @metric description Total number of tasks completed by the task runner.
// @metric calculation Incremented by TaskRunner after successful task completion.
var TasksProcessed = promauto.NewCounter(prometheus.CounterOpts{
	Name: "tasks_processed_total",
	Help: "Total tasks processed",
})

// JobAttempts tracks task attempts grouped by outcome (promauto Vec).
//
// @metric description Total task attempts labelled by outcome.
// @metric calculation Incremented in TaskRunner after each attempt resolves.
// @label outcome Either "success", "retry", or "failed"
var JobAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "job_attempts_total",
	Help: "Total job attempts per outcome",
}, []string{"outcome"})

// customRegistry would normally hold a per-handler collector namespace.
// In the fixture it's a placeholder so we can declare a promauto.With(reg)
// metric below and prove the extractor silently skips the chained form.
var customRegistry = prometheus.NewRegistry()

// SkippedBecauseWithReg is declared via the promauto.With(registry).NewX
// chained form, which the extractor does NOT support (deferred to v0.2).
// TestGolden_PromautoWithRegSkipped asserts it stays absent from snapshots.
//
// @metric description Canary metric — if this appears in the snapshot, the extractor has regressed.
// @metric calculation Canary — not expected to be counted.
var SkippedBecauseWithReg = promauto.With(customRegistry).NewCounter(prometheus.CounterOpts{
	Name: "should_not_appear_in_snapshot_with_reg",
	Help: "Canary",
})
