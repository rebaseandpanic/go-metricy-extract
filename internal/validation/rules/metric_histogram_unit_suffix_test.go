package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricHistogramUnitSuffixRule(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		wantCount int
	}{
		{
			name: "histogram with _seconds -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "request_duration_seconds", Type: "histogram"},
				},
			},
			wantCount: 0,
		},
		{
			name: "histogram with _bytes -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "response_size_bytes", Type: "histogram"},
				},
			},
			wantCount: 0,
		},
		{
			name: "histogram with _ratio -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "cache_hit_ratio", Type: "histogram"},
				},
			},
			wantCount: 0,
		},
		{
			name: "histogram without a unit suffix -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "request_duration", Type: "histogram"},
				},
			},
			wantCount: 1,
		},
		{
			name: "histogram with non-listed unit _weight -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "payload_weight", Type: "histogram"},
				},
			},
			wantCount: 1,
		},
		{
			// Uppercase suffix is treated as a different string — case-
			// sensitivity is a deliberate contract choice (Prometheus
			// names are conventionally lowercase).
			name: "histogram with uppercase _SECONDS -- 1 violation (case-sensitive)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "request_duration_SECONDS", Type: "histogram"},
				},
			},
			wantCount: 1,
		},
		{
			// Ends with _total, not any recognised unit -- violation.
			// (Prometheus convention reserves _total for counters; the
			// extra _seconds would be the tail we want to see.)
			name: "histogram with _seconds_total -- 1 violation (suffix is _total)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "request_duration_seconds_total", Type: "histogram"},
				},
			},
			wantCount: 1,
		},
		{
			name: "counter without unit suffix -- no violation (only histograms checked)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "http_requests_total", Type: "counter"},
				},
			},
			wantCount: 0,
		},
		{
			name: "gauge without unit suffix -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "active_connections", Type: "gauge"},
				},
			},
			wantCount: 0,
		},
		{
			name: "summary without unit suffix -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "rpc_duration", Type: "summary"},
				},
			},
			wantCount: 0,
		},
		{
			name: "histogram with empty name -- skipped",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "", Type: "histogram"},
				},
			},
			wantCount: 0,
		},
		{
			name: "multiple histograms mixed -- one violation per offender",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "good_seconds", Type: "histogram"},
					{Name: "bad_one", Type: "histogram"},
					{Name: "ok_bytes", Type: "histogram"},
					{Name: "bad_two", Type: "histogram"},
					{Name: "some_counter_total", Type: "counter"},
				},
			},
			wantCount: 2,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
		{
			name:      "nil snapshot -- no violations",
			snapshot:  nil,
			wantCount: 0,
		},
	}
	rule := &MetricHistogramUnitSuffixRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.histogram-unit-suffix" {
					t.Errorf("RuleID: got %q, want metric.histogram-unit-suffix", v.RuleID)
				}
				if v.Severity != validation.SeverityWarning {
					t.Errorf("Severity: got %v, want SeverityWarning", v.Severity)
				}
			}
		})
	}
}

// TestMetricHistogramUnitSuffixRule_AllAllowedSuffixes pins every entry in
// histogramUnitSuffixes as positive-path accepted. If someone removes a
// suffix from the allow-list, this table-driven test points at exactly
// which one regressed.
func TestMetricHistogramUnitSuffixRule_AllAllowedSuffixes(t *testing.T) {
	suffixes := []string{
		"_seconds", "_milliseconds", "_microseconds", "_nanoseconds",
		"_bytes", "_kilobytes", "_megabytes",
		"_ratio", "_percent", "_fraction",
		"_bits", "_celsius", "_meters",
	}
	rule := &MetricHistogramUnitSuffixRule{}
	for _, suf := range suffixes {
		t.Run("suffix"+suf, func(t *testing.T) {
			snap := &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo" + suf, Type: "histogram"},
				},
			}
			vios := rule.Validate(snap, validation.Context{})
			if len(vios) != 0 {
				t.Errorf("suffix %q should be accepted; got %d violations", suf, len(vios))
			}
		})
	}
}

func TestMetricHistogramUnitSuffixRule_LocationAndMessage(t *testing.T) {
	rule := &MetricHistogramUnitSuffixRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{
			{Name: "latency_millis", Type: "histogram"},
		},
	}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 1 {
		t.Fatalf("violations: got %d, want 1", len(vios))
	}
	v := vios[0]
	if v.Location == nil {
		t.Fatalf("Location must be non-nil: %+v", v)
	}
	if v.Location.MetricName != "latency_millis" {
		t.Errorf("Location.MetricName: got %q, want latency_millis", v.Location.MetricName)
	}
	if !strings.Contains(v.Message, "latency_millis") {
		t.Errorf("Message must contain metric name: got %q", v.Message)
	}
	// S3: the message must mention the "unit suffix" convention or a concrete
	// example such as "_seconds" — a characteristic token downstream tooling
	// and autofixers can key on.
	if !strings.Contains(v.Message, "unit suffix") && !strings.Contains(v.Message, "_seconds") {
		t.Errorf("message %q should mention 'unit suffix' or a concrete suffix like '_seconds'", v.Message)
	}
}
