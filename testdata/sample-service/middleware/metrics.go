// Package middleware provides request instrumentation.
package middleware

import "github.com/prometheus/client_golang/prometheus"

// BytesProcessed counts bytes read/written per connection.
//
// @metric description Total bytes processed across all connections.
// @metric calculation Incremented in Read/Write methods with payload length.
// @label direction Either "in" or "out"
var BytesProcessed = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "bytes_processed_total",
		Help: "Total bytes processed",
	},
	[]string{"direction"},
)
