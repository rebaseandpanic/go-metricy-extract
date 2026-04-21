package rules

import (
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricNameRequiredRule(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		wantCount int
	}{
		{
			name: "metric with name -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Help: "h"}},
			},
			wantCount: 0,
		},
		{
			name: "metric with empty name -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "", Help: "h"}},
			},
			wantCount: 1,
		},
		{
			// Whitespace-only name is NOT trimmed by the rule — this pins
			// the "no-trim" contract. A whitespace-only name technically
			// has Name != "", so this rule considers it acceptable (other
			// validators can flag it).
			name: "whitespace-only name is NOT a violation (no trim)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "   ", Help: "h"}},
			},
			wantCount: 0,
		},
		{
			name: "multiple metrics with mixed empty names -- one violation per empty",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "a"}, {Name: ""}, {Name: ""}, {Name: "b"},
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
	rule := &MetricNameRequiredRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.name-required" {
					t.Errorf("RuleID: got %q, want metric.name-required", v.RuleID)
				}
				if v.Severity != validation.SeverityError {
					t.Errorf("Severity: got %v, want SeverityError", v.Severity)
				}
			}
		})
	}
}

// TestMetricNameRequiredRule_LocationMetricNameEmpty pins the code
// contract: when a metric has an empty Name, the violation's
// Location.MetricName is set to the empty string (not omitted, not a
// placeholder). Engine enrichment won't find a file/line for it — that's
// by design, since the name itself is the missing signal.
func TestMetricNameRequiredRule_LocationMetricNameEmpty(t *testing.T) {
	rule := &MetricNameRequiredRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{{Name: "", Help: "h"}},
	}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 1 {
		t.Fatalf("violations: got %d, want 1", len(vios))
	}
	v := vios[0]
	if v.Location == nil {
		t.Fatalf("Location must be non-nil: %+v", v)
	}
	if v.Location.MetricName != "" {
		t.Errorf("Location.MetricName: got %q, want empty", v.Location.MetricName)
	}
}
