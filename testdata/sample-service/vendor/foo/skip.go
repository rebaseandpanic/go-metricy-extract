// Package foo is a vendored package — MUST NOT be scanned by go-metricy-extract.
//
// Its sole purpose is to prove the walker's vendor/ skip rule still fires when
// the extractor is pointed at a real repository tree. If this metric ever
// shows up in the golden snapshot, the skip rule has regressed.
package foo

import "github.com/prometheus/client_golang/prometheus"

// ShouldNotAppearInSnapshot is the canary metric: it exists, it is fully
// annotated, but it lives under vendor/ so the walker must skip the entire
// subtree before the extractor ever sees it.
var ShouldNotAppearInSnapshot = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "vendor_metric_bug",
	Help: "This should not appear",
})
