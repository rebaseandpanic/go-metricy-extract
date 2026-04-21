package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricCalculationRequiredRule(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		wantCount int
	}{
		{
			name: "calculation set -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Calculation: strPtr("count()")}},
			},
			wantCount: 0,
		},
		{
			name: "calculation nil -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Calculation: nil}},
			},
			wantCount: 1,
		},
		{
			name: "calculation empty string -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Calculation: strPtr("")}},
			},
			wantCount: 1,
		},
		{
			// Whitespace-only calculation is NOT trimmed: pins the
			// no-trim contract. Empty-string and whitespace-only are
			// intentionally distinct, as elsewhere.
			name: "whitespace-only calculation is NOT a violation (no trim)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Calculation: strPtr("  ")}},
			},
			wantCount: 0,
		},
		{
			name: "mix of set and missing -- counts only the missing ones",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "a", Calculation: strPtr("sum()")},
					{Name: "b", Calculation: nil},
					{Name: "c", Calculation: strPtr("")},
				},
			},
			wantCount: 2,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
	}
	rule := &MetricCalculationRequiredRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.calculation-required" {
					t.Errorf("RuleID: got %q, want metric.calculation-required", v.RuleID)
				}
			}
		})
	}
}

// TestMetricCalculationRequiredRule_LocationAndMessage pins that the
// rule mirrors the metric name into Location.MetricName and includes it
// in the message. Engine enrichment relies on the Location field; the
// stderr reader relies on the message.
func TestMetricCalculationRequiredRule_LocationAndMessage(t *testing.T) {
	rule := &MetricCalculationRequiredRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{{Name: "foo", Calculation: nil}},
	}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 1 {
		t.Fatalf("violations: got %d, want 1", len(vios))
	}
	v := vios[0]
	if v.Location == nil || v.Location.MetricName != "foo" {
		t.Errorf("Location.MetricName: got %+v, want \"foo\"", v.Location)
	}
	if !strings.Contains(v.Message, "foo") {
		t.Errorf("message %q should contain metric name", v.Message)
	}
}
