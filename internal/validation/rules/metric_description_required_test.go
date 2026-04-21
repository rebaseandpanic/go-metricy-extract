package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func strPtr(s string) *string { return &s }

func TestMetricDescriptionRequiredRule(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		wantCount int
	}{
		{
			name: "description set -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Description: strPtr("explained")}},
			},
			wantCount: 0,
		},
		{
			name: "description nil -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Description: nil}},
			},
			wantCount: 1,
		},
		{
			name: "description empty string -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Description: strPtr("")}},
			},
			wantCount: 1,
		},
		{
			// Whitespace-only description is NOT trimmed: pins the
			// no-trim contract. If three spaces are offensive to the
			// reader that's a separate rule's concern.
			name: "whitespace-only description is NOT a violation (no trim)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Description: strPtr("   ")}},
			},
			wantCount: 0,
		},
		{
			name: "mix of set and missing -- 2 violations for 2 missing",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "a", Description: strPtr("ok")},
					{Name: "b", Description: nil},
					{Name: "c", Description: strPtr("")},
					{Name: "d", Description: strPtr("also ok")},
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
	rule := &MetricDescriptionRequiredRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.description-required" {
					t.Errorf("RuleID: got %q, want metric.description-required", v.RuleID)
				}
				if v.Severity != validation.SeverityError {
					t.Errorf("Severity: got %v, want SeverityError", v.Severity)
				}
			}
		})
	}
}

// TestMetricDescriptionRequiredRule_LocationAndMessage pins that the rule
// propagates the metric name into both Location.MetricName (so engine
// enrichment can attach file/line) and the violation message (so stderr
// summaries identify the offending metric).
func TestMetricDescriptionRequiredRule_LocationAndMessage(t *testing.T) {
	rule := &MetricDescriptionRequiredRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{{Name: "foo", Description: nil}},
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
