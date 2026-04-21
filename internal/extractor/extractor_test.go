package extractor

import (
	"go/token"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/rebaseandpanic/go-metricy-extract/internal/annotations"
	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
)

// ignoreSourceLocation is used by the bulk metric-shape tests below. Those
// fixtures only pin Name/Type/Help/Labels/Descriptions; after step 6,
// every emitted descriptor also carries a SourceLocation. The location is
// exercised explicitly by TestExtractSource_SourceLocation, so bulk tests
// elide it here to keep fixtures readable.
var ignoreSourceLocation = cmpopts.IgnoreFields(model.MetricDescriptor{}, "SourceLocation")

// strPtr is a small helper for building descriptor fixtures with pointer
// fields. Kept inside _test.go to avoid leaking into production code.
func strPtr(s string) *string { return &s }

func TestExtractSource(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		wantMetrics  []model.MetricDescriptor
		wantWarnings int // exact count (0 means none)
	}{
		{
			name: "simple counter",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "test_total", Help: "test metric"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "test_total", Type: "counter", Help: "test metric"},
			},
		},
		{
			name: "counter with bare CounterOpts type",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
		},
		{
			name: "counter with untyped composite literal",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
var Y = prometheus.NewCounter(CounterOpts{Name: "y_total", Help: "another"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
				{Name: "y_total", Type: "counter", Help: "another"},
			},
		},
		{
			name: "two counters preserve declaration order",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var First = prometheus.NewCounter(prometheus.CounterOpts{Name: "first_total", Help: "first"})
var Second = prometheus.NewCounter(prometheus.CounterOpts{Name: "second_total", Help: "second"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "first_total", Type: "counter", Help: "first"},
				{Name: "second_total", Type: "counter", Help: "second"},
			},
		},
		{
			name: "multiple vars in one var block",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var (
    A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a_total", Help: "a"})
    B = prometheus.NewCounter(prometheus.CounterOpts{Name: "b_total", Help: "b"})
)
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a_total", Type: "counter", Help: "a"},
				{Name: "b_total", Type: "counter", Help: "b"},
			},
		},
		{
			name: "non-prometheus receiver is skipped silently",
			src: `package p
var X = foo.NewCounter(foo.CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "promauto.NewCounter is recognized",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
		},
		{
			name: "NewGauge: simple case",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewGauge(prometheus.GaugeOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "gauge", Help: "y"},
			},
		},
		{
			name: "NewCounterVec with single label",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"lbl"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "lbl"},
				}},
			},
		},
		{
			name: "non-literal Name emits warning and skips metric",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var nameVar = "dynamic_total"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: nameVar, Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 1,
		},
		{
			name: "non-literal Help emits warning and skips metric",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var helpVar = "dynamic help"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: helpVar})
`,
			wantMetrics:  nil,
			wantWarnings: 1,
		},
		{
			name: "opts passed as variable emits warning",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var cfg = prometheus.CounterOpts{Name: "x", Help: "y"}
var X = prometheus.NewCounter(cfg)
`,
			wantMetrics:  nil,
			wantWarnings: 1,
		},
		{
			name: "NewCounter with nil argument does not panic",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(nil)
`,
			wantMetrics:  nil,
			wantWarnings: 1, // nil is not a CompositeLit → "non-literal options"
		},
		{
			name: "NewCounter with zero arguments does not panic",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter()
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "empty file with only package declaration",
			src: `package main
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "function-local var is not extracted (top-level only)",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
func init() {
    var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "local_total", Help: "local"})
    _ = X
}
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "opts missing Name field is skipped silently",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "wrong opts type (GaugeOpts passed to NewCounter) is ignored",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.GaugeOpts{Name: "x", Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		// ---- New MUST subtests (step 3 review) ----
		{
			name: "chained selector receiver",
			src: `package p
var X = a.b.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "bare NewCounter via dot-import",
			src: `package foo
import . "github.com/prometheus/client_golang/prometheus"
var X = NewCounter(CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "positional CounterOpts",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{"foo_total", "bar"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "only Name present, Help missing",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "empty CounterOpts",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "extra arguments after opts",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"}, "extra")
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
		},
		// ---- New SHOULD subtests (step 3 review) ----
		{
			name: "raw-string literal Name",
			src: "package p\n" +
				"import \"github.com/prometheus/client_golang/prometheus\"\n" +
				"var X = prometheus.NewCounter(prometheus.CounterOpts{Name: `foo_total`, Help: \"h\"})\n",
			wantMetrics: []model.MetricDescriptor{
				{Name: "foo_total", Type: "counter", Help: "h"},
			},
		},
		{
			name: "Help before Name",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Help: "h", Name: "n"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "n", Type: "counter", Help: "h"},
			},
		},
		{
			name: "additional opts fields ignored",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Namespace: "ns", Subsystem: "sub", Name: "x", Help: "y", ConstLabels: prometheus.Labels{"k": "v"}})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
		},
		{
			name: "nested NewCounter call",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"}))
`,
			wantMetrics:  nil,
			wantWarnings: -1, // sentinel: assert >= 1 below
		},
		{
			name: "NewHistogram: simple case",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "histogram", Help: "y"},
			},
		},
		{
			name: "NewSummary: simple case",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewSummary(prometheus.SummaryOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "summary", Help: "y"},
			},
		},
		{
			name: "NewCounterVec with two labels preserves order",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"method", "status"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "method"},
					{Name: "status"},
				}},
			},
		},
		{
			name: "NewGaugeVec with one label",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "gauge", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "l"},
				}},
			},
		},
		{
			name: "NewHistogramVec with one label",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "histogram", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "l"},
				}},
			},
		},
		{
			name: "NewSummaryVec with one label",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewSummaryVec(prometheus.SummaryOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "summary", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "l"},
				}},
			},
		},
		{
			name: "promauto.NewCounterVec with labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "l"},
				}},
			},
		},
		{
			name: "promauto.With(reg).NewCounter is skipped silently (deferred)",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var reg = newRegistry()
var X = promauto.With(reg).NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		{
			name: "Vec constructor missing labels argument warns and skips metric",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics:  nil,
			wantWarnings: 1,
		},
		{
			name: "Vec with partially non-literal labels emits subset + warning",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var someVar = "b"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"a", someVar, "c"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "a"},
					{Name: "c"},
				}},
			},
			wantWarnings: 1,
		},
		{
			name: "Vec with labels passed as variable emits metric without labels + warning",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var lbls = []string{"a", "b"}
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, lbls)
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
			wantWarnings: 1,
		},
		{
			name: "Vec with all non-literal labels emits metric without labels + warning",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var v1 = "a"
var v2 = "b"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{v1, v2})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
			wantWarnings: 1,
		},
		{
			name: "Vec with map literal instead of slice emits metric without labels + warning",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, map[string]string{})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
			wantWarnings: 1,
		},
		{
			name: "multi-name value spec",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var A, B = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"}), prometheus.NewCounter(prometheus.CounterOpts{Name: "b", Help: "hb"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter", Help: "ha"},
				{Name: "b", Type: "counter", Help: "hb"},
			},
		},
		// ---- Bonus ----
		{
			name: "top-level CounterOpts literal (not a call)",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.CounterOpts{Name: "x", Help: "y"}
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		// ---- MUST (step 4 review): W1 empty slice ----
		{
			name: "Vec with empty labels slice emits metric without labels + warning",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
			wantWarnings: 1,
		},
		// ---- MUST (step 4 review): W2 duplicate labels ----
		{
			name: "Vec with duplicate label names emits metric + warnings",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"a", "a", "b"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "a"},
					{Name: "a"},
					{Name: "b"},
				}},
			},
			wantWarnings: -1, // sentinel: assert >= 1 below
		},
		// ---- MUST: fixed-size array instead of slice ----
		{
			name: "Vec with fixed-size array instead of slice emits without labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, [2]string{"a", "b"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
			wantWarnings: 1,
		},
		// ---- MUST: promauto.With(reg).NewCounterVec silent skip ----
		{
			name: "promauto.With(reg).NewCounterVec is skipped silently (deferred)",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var reg = newRegistry()
var X = promauto.With(reg).NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics:  nil,
			wantWarnings: 0,
		},
		// ---- MUST: promauto × 6 types ----
		{
			name: "promauto.NewGauge is recognized",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewGauge(prometheus.GaugeOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "gauge", Help: "y"},
			},
		},
		{
			name: "promauto.NewHistogram is recognized",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewHistogram(prometheus.HistogramOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "histogram", Help: "y"},
			},
		},
		{
			name: "promauto.NewSummary is recognized",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewSummary(prometheus.SummaryOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "summary", Help: "y"},
			},
		},
		{
			name: "promauto.NewGaugeVec with labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewGaugeVec(prometheus.GaugeOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "gauge", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "l"},
				}},
			},
		},
		{
			name: "promauto.NewHistogramVec with labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewHistogramVec(prometheus.HistogramOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "histogram", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "l"},
				}},
			},
		},
		{
			name: "promauto.NewSummaryVec with labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus/promauto"
var X = promauto.NewSummaryVec(prometheus.SummaryOpts{Name: "x", Help: "y"}, []string{"l"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "summary", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "l"},
				}},
			},
		},
		// ---- SHOULD (step 4 review) ----
		{
			name: "Vec with empty-string label element",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{""})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Labels: []model.LabelDescriptor{
					{Name: ""},
				}},
			},
			wantWarnings: 0,
		},
		{
			name: "Vec with string(name) conversion in labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var name = "m"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{string(name)})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
			wantWarnings: -1, // sentinel: assert >= 1 below
		},
		{
			name: "Vec with constant in labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
const l = "m"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{l, "b"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Labels: []model.LabelDescriptor{
					{Name: "b"},
				}},
			},
			wantWarnings: 1,
		},
		{
			name: "mixed scalar types: Counter + Gauge",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a_total", Help: "a"})
var B = prometheus.NewGauge(prometheus.GaugeOpts{Name: "b", Help: "b"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a_total", Type: "counter", Help: "a"},
				{Name: "b", Type: "gauge", Help: "b"},
			},
		},
		{
			name: "mixed: Counter + CounterVec",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a_total", Help: "a"})
var B = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "b_total", Help: "b"}, []string{"method"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a_total", Type: "counter", Help: "a"},
				{Name: "b_total", Type: "counter", Help: "b", Labels: []model.LabelDescriptor{
					{Name: "method"},
				}},
			},
		},
		{
			name: "two Vecs with different labels do not bleed",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var A = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "a", Help: "ha"}, []string{"a", "b"})
var B = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "b", Help: "hb"}, []string{"x", "y", "z"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter", Help: "ha", Labels: []model.LabelDescriptor{
					{Name: "a"},
					{Name: "b"},
				}},
				{Name: "b", Type: "gauge", Help: "hb", Labels: []model.LabelDescriptor{
					{Name: "x"},
					{Name: "y"},
					{Name: "z"},
				}},
			},
		},
		{
			name: "multiple Vecs in var(...) block",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var (
    A = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "a", Help: "ha"}, []string{"a"})
    B = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "b", Help: "hb"}, []string{"b"})
)
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter", Help: "ha", Labels: []model.LabelDescriptor{
					{Name: "a"},
				}},
				{Name: "b", Type: "gauge", Help: "hb", Labels: []model.LabelDescriptor{
					{Name: "b"},
				}},
			},
		},
	}

	fset := token.NewFileSet()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExtractSource(fset, "test.go", tc.src)
			if err != nil {
				t.Fatalf("ExtractSource returned error: %v", err)
			}

			if diff := cmp.Diff(tc.wantMetrics, res.Metrics, ignoreSourceLocation); diff != "" {
				t.Errorf("metrics mismatch (-want +got):\n%s", diff)
			}

			switch {
			case tc.wantWarnings == -1:
				if len(res.Warnings) < 1 {
					t.Errorf("warning count: want >= 1, got 0")
				}
			default:
				if got := len(res.Warnings); got != tc.wantWarnings {
					t.Errorf("warning count: want %d, got %d (%v)", tc.wantWarnings, got, res.Warnings)
				}
			}
		})
	}
}

func TestExtractSource_SyntaxErrorReturnsError(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
this is not valid go
`
	_, err := ExtractSource(fset, "test.go", src)
	if err == nil {
		t.Fatalf("expected parser error, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected error to mention parse step, got: %v", err)
	}
}

// TestExtractSource_WarningSuffixesAreConsistent pins the canonical trailing
// phrasing of every warning branch in the extractor. Each case triggers a
// specific warning path and asserts the emitted message matches one of the
// documented suffixes. Breaks loudly if any branch's wording drifts.
func TestExtractSource_WarningSuffixesAreConsistent(t *testing.T) {
	// Canonical phrase markers. A warning is acceptable if it ends with any
	// of these suffixes, or (for the continue/emit branches) contains the
	// substring. Keep this list in sync with formatWarning call sites in
	// extractor.go.
	acceptable := []string{
		"; skipping metric",
		"emitting metric without labels",
		"continuing with extracted labels",
		"emitting metric with zero labels",
		"consumer will fail registration",
	}

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "empty labels slice",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{})
`,
		},
		{
			name: "duplicate label",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"a", "a"})
`,
		},
		{
			name: "non-literal Name",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var n = "x"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: n, Help: "y"})
`,
		},
		{
			name: "missing Vec labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"})
`,
		},
		{
			name: "labels passed as variable",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var lbls = []string{"a"}
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, lbls)
`,
		},
		{
			name: "labels wrong type",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, map[string]string{})
`,
		},
		{
			name: "all non-literal labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var v = "a"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{v})
`,
		},
		{
			name: "partial non-literal labels",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var v = "b"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"a", v})
`,
		},
		{
			name: "opts non-literal",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var cfg = prometheus.CounterOpts{Name: "x", Help: "y"}
var X = prometheus.NewCounter(cfg)
`,
		},
	}

	fset := token.NewFileSet()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExtractSource(fset, "test.go", tc.src)
			if err != nil {
				t.Fatalf("ExtractSource returned error: %v", err)
			}
			if len(res.Warnings) == 0 {
				t.Fatalf("expected at least one warning, got none")
			}
			for _, w := range res.Warnings {
				matched := false
				for _, suf := range acceptable {
					if strings.HasSuffix(w, suf) || strings.Contains(w, suf) {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("warning %q does not match any canonical suffix/marker %v", w, acceptable)
				}
			}
		})
	}
}

// TestExtractSource_Annotations verifies that doc-comment directives
// (@metric description / calculation, @label) flow through to the emitted
// MetricDescriptor and LabelDescriptor fields.
func TestExtractSource_Annotations(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		wantMetrics  []model.MetricDescriptor
		wantWarnings int // -1 = >= 1
	}{
		{
			name: "counter with @metric description and calculation",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// HttpRequests counts HTTP traffic.
//
// @metric description Total incoming HTTP requests across all endpoints.
// @metric calculation Incremented in the logging middleware per request.
var HttpRequests = prometheus.NewCounter(prometheus.CounterOpts{Name: "http_requests_total", Help: "total"})
`,
			wantMetrics: []model.MetricDescriptor{
				{
					Name:        "http_requests_total",
					Type:        "counter",
					Help:        "total",
					Description: strPtr("Total incoming HTTP requests across all endpoints."),
					Calculation: strPtr("Incremented in the logging middleware per request."),
				},
			},
			wantWarnings: 0,
		},
		{
			name: "counter-vec with @label annotations",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description HTTP requests by method and status.
// @label method HTTP verb: GET, POST, PUT, DELETE
// @label status HTTP response status class (2xx, 4xx, 5xx)
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"method", "status"})
`,
			wantMetrics: []model.MetricDescriptor{
				{
					Name:        "x",
					Type:        "counter",
					Help:        "y",
					Description: strPtr("HTTP requests by method and status."),
					Labels: []model.LabelDescriptor{
						{Name: "method", Description: strPtr("HTTP verb: GET, POST, PUT, DELETE")},
						{Name: "status", Description: strPtr("HTTP response status class (2xx, 4xx, 5xx)")},
					},
				},
			},
			wantWarnings: 0,
		},
		{
			name: "@label for non-existent label name emits warning, metric still emitted",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// @label method HTTP method
// @label nonexistent this label is not declared in the labels slice
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"method"})
`,
			wantMetrics: []model.MetricDescriptor{
				{
					Name: "x",
					Type: "counter",
					Help: "y",
					Labels: []model.LabelDescriptor{
						{Name: "method", Description: strPtr("HTTP method")},
					},
				},
			},
			wantWarnings: 1,
		},
		{
			name: "var block with individual doc on each ValueSpec",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var (
    // @metric description description for A
    A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"})
    // @metric description description for B
    B = prometheus.NewCounter(prometheus.CounterOpts{Name: "b", Help: "hb"})
)
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter", Help: "ha", Description: strPtr("description for A")},
				{Name: "b", Type: "counter", Help: "hb", Description: strPtr("description for B")},
			},
			wantWarnings: 0,
		},
		{
			name: "var block with doc on GenDecl only applies to sole ValueSpec",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description shared by the var block
var (
    A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"})
)
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter", Help: "ha", Description: strPtr("shared by the var block")},
			},
			wantWarnings: 0,
		},
		{
			name: "non-annotated doc (plain text) yields nil descriptions",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// HttpRequests is a counter.
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y"},
			},
			wantWarnings: 0,
		},
		{
			name: "duplicate @metric description emits warning prefixed with varName",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description first
// @metric description second
var MyMetric = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter", Help: "y", Description: strPtr("second")},
			},
			wantWarnings: 1,
		},
		{
			name: "ValueSpec.Doc wins over GenDecl.Doc when both present",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description from gen decl
var (
    // @metric description from value spec
    A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"})
)
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter", Help: "ha", Description: strPtr("from value spec")},
			},
			wantWarnings: 0,
		},
		{
			name: "GenDecl.Doc applies to every ValueSpec in block",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description shared text
var (
    A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"})
    B = prometheus.NewCounter(prometheus.CounterOpts{Name: "b", Help: "hb"})
)
`,
			wantMetrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter", Help: "ha", Description: strPtr("shared text")},
				{Name: "b", Type: "counter", Help: "hb", Description: strPtr("shared text")},
			},
			wantWarnings: 0,
		},
	}

	fset := token.NewFileSet()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ExtractSource(fset, "test.go", tc.src)
			if err != nil {
				t.Fatalf("ExtractSource returned error: %v", err)
			}

			if diff := cmp.Diff(tc.wantMetrics, res.Metrics, ignoreSourceLocation); diff != "" {
				t.Errorf("metrics mismatch (-want +got):\n%s", diff)
			}

			switch {
			case tc.wantWarnings == -1:
				if len(res.Warnings) < 1 {
					t.Errorf("warning count: want >= 1, got 0")
				}
			default:
				if got := len(res.Warnings); got != tc.wantWarnings {
					t.Errorf("warning count: want %d, got %d (%v)", tc.wantWarnings, got, res.Warnings)
				}
			}
		})
	}
}

// TestExtractSource_ScalarMetricOrphanLabelWording pins the scalar-specific
// orphan-@label wording. For a NewCounter (no labels slice) with a @label
// directive, the warning must point the user at the Vec constructor, not at
// the absent labels slice.
func TestExtractSource_ScalarMetricOrphanLabelWording(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description count
// @label method orphan-on-scalar
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
	res, err := ExtractSource(fset, "test.go", src)
	if err != nil {
		t.Fatalf("ExtractSource returned error: %v", err)
	}
	wantMetrics := []model.MetricDescriptor{
		{Name: "x", Type: "counter", Help: "y", Description: strPtr("count")},
	}
	if diff := cmp.Diff(wantMetrics, res.Metrics, ignoreSourceLocation); diff != "" {
		t.Errorf("metrics mismatch (-want +got):\n%s", diff)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("want 1 warning, got %d (%v)", len(res.Warnings), res.Warnings)
	}
	w := res.Warnings[0]
	if !strings.Contains(w, "X:") {
		t.Errorf("warning should carry varName prefix X:, got %q", w)
	}
	if !strings.Contains(w, `"method"`) {
		t.Errorf("warning should mention orphan label name \"method\", got %q", w)
	}
	if !strings.Contains(w, "scalar metric") {
		t.Errorf("warning should mention scalar metric wording, got %q", w)
	}
}

// TestExtractSource_VecOrphanLabelWording pins the Vec-orphan wording
// fragments: the label name must be quoted and the phrase
// "not declared in labels slice" must appear.
func TestExtractSource_VecOrphanLabelWording(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
import "github.com/prometheus/client_golang/prometheus"
// @label method HTTP method
// @label nonexistent this label is not declared in the labels slice
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x", Help: "y"}, []string{"method"})
`
	res, err := ExtractSource(fset, "test.go", src)
	if err != nil {
		t.Fatalf("ExtractSource returned error: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("want 1 warning, got %d (%v)", len(res.Warnings), res.Warnings)
	}
	w := res.Warnings[0]
	if !strings.Contains(w, `"nonexistent"`) {
		t.Errorf("warning should quote orphan label name, got %q", w)
	}
	if !strings.Contains(w, "not declared in labels slice") {
		t.Errorf("warning should contain the canonical Vec-orphan phrase, got %q", w)
	}
}

// TestExtractSource_MultiNameSpec_ParserWarningsPrefixedWithFirstName pins
// the "first name wins" prefix rule for annotation warnings on a multi-name
// pairwise var spec. Both metrics end up with the same (overwritten)
// Description; the single duplicate-overwrite warning carries the first
// spec name as prefix.
func TestExtractSource_MultiNameSpec_ParserWarningsPrefixedWithFirstName(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description first desc
// @metric description second desc
var First, Second = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"}), prometheus.NewCounter(prometheus.CounterOpts{Name: "b", Help: "hb"})
`
	res, err := ExtractSource(fset, "test.go", src)
	if err != nil {
		t.Fatalf("ExtractSource returned error: %v", err)
	}
	wantMetrics := []model.MetricDescriptor{
		{Name: "a", Type: "counter", Help: "ha", Description: strPtr("second desc")},
		{Name: "b", Type: "counter", Help: "hb", Description: strPtr("second desc")},
	}
	if diff := cmp.Diff(wantMetrics, res.Metrics, ignoreSourceLocation); diff != "" {
		t.Errorf("metrics mismatch (-want +got):\n%s", diff)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("want 1 warning, got %d (%v)", len(res.Warnings), res.Warnings)
	}
	if !strings.HasPrefix(res.Warnings[0], "First:") {
		t.Errorf("warning should be prefixed with first spec name First:, got %q", res.Warnings[0])
	}
}

// TestExtractSourceWithParser_CustomParser verifies the injected-parser hook
// is honoured — the fake parser's output appears on descriptors and its
// warnings reach res.Warnings (prefixed with the first varName of the spec).
func TestExtractSourceWithParser_CustomParser(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
import "github.com/prometheus/client_golang/prometheus"
// any doc at all — custom parser ignores content
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
	fake := fakeParser{
		ann: annotations.Annotations{
			Description: strPtr("injected description"),
			Calculation: strPtr("injected calc"),
		},
		warnings: []string{"synthetic warning"},
	}
	res, err := ExtractSourceWithParser(fset, "test.go", src, fake)
	if err != nil {
		t.Fatalf("ExtractSourceWithParser returned error: %v", err)
	}
	want := []model.MetricDescriptor{{
		Name:        "x",
		Type:        "counter",
		Help:        "y",
		Description: strPtr("injected description"),
		Calculation: strPtr("injected calc"),
	}}
	if diff := cmp.Diff(want, res.Metrics, ignoreSourceLocation); diff != "" {
		t.Errorf("metrics mismatch (-want +got):\n%s", diff)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("want 1 warning, got %d (%v)", len(res.Warnings), res.Warnings)
	}
	// Exact-match pin for the `<var>: <issue>` formatWarning contract — a
	// substring check would silently accept drift to ` — ` or other separators.
	if want := "X: synthetic warning"; res.Warnings[0] != want {
		t.Errorf("warning mismatch: want %q, got %q", want, res.Warnings[0])
	}
}

// fakeParser is a tiny AnnotationParser that returns a fixed Annotations
// value and warning list regardless of input. Used to verify parser
// injection is honoured.
type fakeParser struct {
	ann      annotations.Annotations
	warnings []string
}

func (f fakeParser) Parse(_ string) (annotations.Annotations, []string) {
	return f.ann, f.warnings
}

// TestExtractSourceWithParser_NilParserFallsBackToDefault guards the nil
// contract so callers can pass a zero-valued options struct without panics.
func TestExtractSourceWithParser_NilParserFallsBackToDefault(t *testing.T) {
	fset := token.NewFileSet()
	src := `package p
import "github.com/prometheus/client_golang/prometheus"
// @metric description from default parser
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
	res, err := ExtractSourceWithParser(fset, "test.go", src, nil)
	if err != nil {
		t.Fatalf("ExtractSourceWithParser returned error: %v", err)
	}
	if len(res.Metrics) != 1 {
		t.Fatalf("want 1 metric, got %d", len(res.Metrics))
	}
	if res.Metrics[0].Description == nil || *res.Metrics[0].Description != "from default parser" {
		t.Errorf("description not populated via default parser; got %v", res.Metrics[0].Description)
	}
}

// TestExtractSource_WarningMessageMentionsVariable verifies that warnings
// emitted for malformed metric declarations start with the offending variable
// name. Consumers (CLI, CI tooling, golden tests) rely on this for grepping.
func TestExtractSource_WarningMessageMentionsVariable(t *testing.T) {
	fset := token.NewFileSet()
	const varName = "HttpRequests"
	src := `package p
import "github.com/prometheus/client_golang/prometheus"
var nameVar = "x"
var ` + varName + ` = prometheus.NewCounter(prometheus.CounterOpts{Name: nameVar, Help: "y"})
`
	res, err := ExtractSource(fset, "test.go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("want 1 warning, got %d", len(res.Warnings))
	}
	got := res.Warnings[0]
	if !strings.Contains(got, varName) {
		t.Errorf("warning should mention the offending variable %q, got: %q", varName, got)
	}
	if !strings.Contains(got, "Name") {
		t.Errorf("warning should mention the field name, got: %q", got)
	}
	if !strings.HasSuffix(got, "; skipping metric") {
		t.Errorf("warning should end with canonical suffix; got: %q", got)
	}
}

// intPtr returns a pointer to the given int, used in SourceLocation.Line
// assertions where the struct literal demands a *int.
func intPtr(i int) *int { return &i }

// TestExtractSource_SourceLocation verifies that emitted metrics carry a
// populated SourceLocation pointing at the declaring identifier. This
// covers the contract documented on [model.MetricDescriptor.SourceLocation]
// — File/Line/Member populated, Class always nil (Go has no classes).
func TestExtractSource_SourceLocation(t *testing.T) {
	t.Run("SourceLocationIsPopulated", func(t *testing.T) {
		fset := token.NewFileSet()
		src := `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
		const filename = "/abs/path/to/file.go"
		res, err := ExtractSource(fset, filename, src)
		if err != nil {
			t.Fatalf("ExtractSource: %v", err)
		}
		if len(res.Metrics) != 1 {
			t.Fatalf("want 1 metric, got %d", len(res.Metrics))
		}
		sl := res.Metrics[0].SourceLocation
		if sl == nil {
			t.Fatalf("want SourceLocation populated, got nil")
		}
		if sl.File != filename {
			t.Errorf("File: got %q, want %q", sl.File, filename)
		}
		if sl.Line == nil || *sl.Line <= 0 {
			t.Errorf("Line: want positive, got %v", sl.Line)
		}
		if sl.Class != nil {
			t.Errorf("Class: want nil (Go has no classes), got %v", sl.Class)
		}
		if sl.Member == nil || *sl.Member != "X" {
			t.Errorf("Member: want \"X\", got %v", sl.Member)
		}
	})

	t.Run("SourceLocationWithRepoRoot", func(t *testing.T) {
		fset := token.NewFileSet()
		src := `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
		res, err := ExtractSourceWithOptions(fset, "/abs/path/to/file.go", src, ExtractOptions{RepoRoot: "/abs/path"})
		if err != nil {
			t.Fatalf("ExtractSourceWithOptions: %v", err)
		}
		if len(res.Metrics) != 1 {
			t.Fatalf("want 1 metric, got %d", len(res.Metrics))
		}
		sl := res.Metrics[0].SourceLocation
		if sl == nil {
			t.Fatalf("want SourceLocation populated, got nil")
		}
		if sl.File != "to/file.go" {
			t.Errorf("File: got %q, want %q", sl.File, "to/file.go")
		}
	})

	t.Run("SourceLocationLineNumberCorrect", func(t *testing.T) {
		fset := token.NewFileSet()
		// Line 1: package p
		// Line 2: import ...
		// Line 3: blank
		// Line 4: blank
		// Line 5: var X = ...
		src := "package p\nimport \"github.com/prometheus/client_golang/prometheus\"\n\n\nvar X = prometheus.NewCounter(prometheus.CounterOpts{Name: \"x\", Help: \"y\"})\n"
		res, err := ExtractSource(fset, "f.go", src)
		if err != nil {
			t.Fatalf("ExtractSource: %v", err)
		}
		if len(res.Metrics) != 1 {
			t.Fatalf("want 1 metric, got %d", len(res.Metrics))
		}
		sl := res.Metrics[0].SourceLocation
		if sl == nil || sl.Line == nil {
			t.Fatalf("want SourceLocation with Line, got %+v", sl)
		}
		if *sl.Line != 5 {
			t.Errorf("Line: got %d, want 5", *sl.Line)
		}
	})

	t.Run("SourceLocationForMultiNameSpec", func(t *testing.T) {
		fset := token.NewFileSet()
		src := `package p
import "github.com/prometheus/client_golang/prometheus"
var A, B = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"}), prometheus.NewCounter(prometheus.CounterOpts{Name: "b", Help: "hb"})
`
		res, err := ExtractSource(fset, "f.go", src)
		if err != nil {
			t.Fatalf("ExtractSource: %v", err)
		}
		if len(res.Metrics) != 2 {
			t.Fatalf("want 2 metrics, got %d", len(res.Metrics))
		}
		slA := res.Metrics[0].SourceLocation
		slB := res.Metrics[1].SourceLocation
		if slA == nil || slA.Member == nil || *slA.Member != "A" {
			t.Errorf("metric A Member: got %+v, want A", slA)
		}
		if slB == nil || slB.Member == nil || *slB.Member != "B" {
			t.Errorf("metric B Member: got %+v, want B", slB)
		}
		// A and B share one var spec, so line numbers match.
		if slA.Line == nil || slB.Line == nil || *slA.Line != *slB.Line {
			t.Errorf("A/B should share the same line; got A.Line=%v B.Line=%v", slA.Line, slB.Line)
		}
	})

	t.Run("SourceLocationInVarBlock", func(t *testing.T) {
		fset := token.NewFileSet()
		// Line 1: package p
		// Line 2: import ...
		// Line 3: var (
		// Line 4:     A = ...
		// Line 5:     B = ...
		// Line 6: )
		src := `package p
import "github.com/prometheus/client_golang/prometheus"
var (
	A = prometheus.NewCounter(prometheus.CounterOpts{Name: "a", Help: "ha"})
	B = prometheus.NewCounter(prometheus.CounterOpts{Name: "b", Help: "hb"})
)
`
		res, err := ExtractSource(fset, "f.go", src)
		if err != nil {
			t.Fatalf("ExtractSource: %v", err)
		}
		if len(res.Metrics) != 2 {
			t.Fatalf("want 2 metrics, got %d", len(res.Metrics))
		}
		if got, want := *res.Metrics[0].SourceLocation.Line, 4; got != want {
			t.Errorf("A line: got %d, want %d", got, want)
		}
		if got, want := *res.Metrics[1].SourceLocation.Line, 5; got != want {
			t.Errorf("B line: got %d, want %d", got, want)
		}
		// Sanity: Members are A and B respectively.
		if m := res.Metrics[0].SourceLocation.Member; m == nil || *m != "A" {
			t.Errorf("A member: got %v", m)
		}
		if m := res.Metrics[1].SourceLocation.Member; m == nil || *m != "B" {
			t.Errorf("B member: got %v", m)
		}
	})

	t.Run("SourceLocationPathIsForwardSlashOnAllOS", func(t *testing.T) {
		fset := token.NewFileSet()
		src := `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
		// Filenames supplied to ParseFile are opaque strings — token.FileSet
		// keeps them verbatim. Exercise with forward-slashed paths (portable)
		// plus an explicit repoRoot so MakeRelative applies filepath.ToSlash.
		res, err := ExtractSourceWithOptions(fset, "/abs/root/sub/dir/f.go", src, ExtractOptions{RepoRoot: "/abs/root"})
		if err != nil {
			t.Fatalf("ExtractSourceWithOptions: %v", err)
		}
		if len(res.Metrics) != 1 {
			t.Fatalf("want 1 metric, got %d", len(res.Metrics))
		}
		got := res.Metrics[0].SourceLocation.File
		if strings.Contains(got, `\`) {
			t.Errorf("File contains backslash (want forward-slash only): %q", got)
		}
		if got != "sub/dir/f.go" {
			t.Errorf("File: got %q, want %q", got, "sub/dir/f.go")
		}
	})

	t.Run("SourceLocationOutsideRepoRootIsAbsolute", func(t *testing.T) {
		fset := token.NewFileSet()
		src := `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
		// The filename lives *outside* the configured repo root — MakeRelative
		// should detect the escape (rel begins with "..") and pass the original
		// absolute path through unchanged.
		const filename = "/other/absolute/path.go"
		res, err := ExtractSourceWithOptions(fset, filename, src, ExtractOptions{RepoRoot: "/repo/root"})
		if err != nil {
			t.Fatalf("ExtractSourceWithOptions: %v", err)
		}
		if len(res.Metrics) != 1 {
			t.Fatalf("want 1 metric, got %d", len(res.Metrics))
		}
		sl := res.Metrics[0].SourceLocation
		if sl == nil {
			t.Fatalf("want SourceLocation populated, got nil")
		}
		if sl.File != filename {
			t.Errorf("File: got %q, want %q (outside-repoRoot path must pass through)", sl.File, filename)
		}
	})

	t.Run("SourceLocationNilWhenFilenameEmpty", func(t *testing.T) {
		fset := token.NewFileSet()
		src := `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`
		// An empty filename hits the defensive `pos.Filename == ""` branch
		// in buildSourceLocation — the metric should emit without any
		// SourceLocation, rather than with an empty File string.
		res, err := ExtractSource(fset, "", src)
		if err != nil {
			t.Fatalf("ExtractSource: %v", err)
		}
		if len(res.Metrics) != 1 {
			t.Fatalf("want 1 metric, got %d", len(res.Metrics))
		}
		if res.Metrics[0].SourceLocation != nil {
			t.Errorf("want nil SourceLocation for empty filename, got %+v", res.Metrics[0].SourceLocation)
		}
	})

	// Additional guard: the intPtr helper is unused until we assert on *int
	// values elsewhere. Reference it here so unused-helper analysers stay
	// quiet and future sub-tests can use it without re-plumbing.
	_ = intPtr
}
