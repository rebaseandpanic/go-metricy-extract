package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricCounterTotalSuffixRule(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		wantCount int
	}{
		{
			name: "counter with _total suffix -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "http_requests_total", Type: "counter"},
				},
			},
			wantCount: 0,
		},
		{
			name: "counter without _total suffix -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "http_requests", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "gauge without _total suffix -- no violation (only counters are checked)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "active_connections", Type: "gauge"},
				},
			},
			wantCount: 0,
		},
		{
			name: "histogram without _total suffix -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "request_duration_seconds", Type: "histogram"},
				},
			},
			wantCount: 0,
		},
		{
			name: "summary without _total suffix -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "rpc_duration", Type: "summary"},
				},
			},
			wantCount: 0,
		},
		{
			// Name with bare word "total" but no underscore prefix — this
			// does not satisfy _total and must be flagged. Pinning this
			// avoids a lenient strings.Contains drift.
			name: "counter ending with 'total' without underscore -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "requeststotal", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			// Trailing underscore after _total means the name does not
			// actually end with _total. Flag it.
			name: "counter ending with '_total_' trailing underscore -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo_total_", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			// Literal name "_total" technically ends with _total -- the
			// naming-convention check passes. metric.name-snake-case will
			// flag the leading underscore separately.
			name: "counter named exactly '_total' -- no violation (suffix matches)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "_total", Type: "counter"},
				},
			},
			wantCount: 0,
		},
		{
			// Empty name is deferred to metric.name-required.
			name: "counter with empty name -- skipped (delegated to name-required)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "", Type: "counter"},
				},
			},
			wantCount: 0,
		},
		{
			name: "multiple metrics mixed -- one violation per offending counter",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "good_total", Type: "counter"},
					{Name: "bad_one", Type: "counter"},
					{Name: "bad_two", Type: "counter"},
					{Name: "some_gauge", Type: "gauge"},
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
	rule := &MetricCounterTotalSuffixRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.counter-total-suffix" {
					t.Errorf("RuleID: got %q, want metric.counter-total-suffix", v.RuleID)
				}
				if v.Severity != validation.SeverityWarning {
					t.Errorf("Severity: got %v, want SeverityWarning", v.Severity)
				}
			}
		})
	}
}

func TestMetricCounterTotalSuffixRule_LocationAndMessage(t *testing.T) {
	rule := &MetricCounterTotalSuffixRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{
			{Name: "foo_bar", Type: "counter"},
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
	if v.Location.MetricName != "foo_bar" {
		t.Errorf("Location.MetricName: got %q, want foo_bar", v.Location.MetricName)
	}
	if !strings.Contains(v.Message, "foo_bar") {
		t.Errorf("Message must contain metric name: got %q", v.Message)
	}
	// S2: message must mention the "_total" convention so autofixers and
	// readers can key on a characteristic token rather than only the metric name.
	if !strings.Contains(v.Message, "_total") {
		t.Errorf("message %q should mention _total", v.Message)
	}
}
