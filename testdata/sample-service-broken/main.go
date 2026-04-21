// Package broken is a test fixture that deliberately triggers validation errors.
// Used by the golden suite to pin the wire format of violation JSON.
package broken

import "github.com/prometheus/client_golang/prometheus"

// BareCounterNoAnnotations lacks both description and calculation annotations.
// Expected violations: metric.description-required, metric.calculation-required.
var BareCounterNoAnnotations = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "bare_counter_total",
	Help: "Counter with no business annotations",
})

// DuplicateA and DuplicateB share the same metric name — triggers metric.duplicate-name.
//
// @metric description First of two metrics deliberately sharing a name.
// @metric calculation Incremented somewhere.
var DuplicateA = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "duplicated_metric",
	Help: "First",
})

// @metric description Second of two metrics deliberately sharing a name with a DIFFERENT type.
// @metric calculation Observed somewhere.
var DuplicateB = prometheus.NewGauge(prometheus.GaugeOpts{
	Name: "duplicated_metric",
	Help: "Second",
})
