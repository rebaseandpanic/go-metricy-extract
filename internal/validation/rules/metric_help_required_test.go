package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricHelpRequiredRule(t *testing.T) {
	cases := []struct {
		name          string
		snapshot      *model.MetricSnapshot
		wantCount     int
		wantNameInMsg string // if non-empty, assert the first violation quotes this name
	}{
		{
			name: "metric with help text -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Help: "describes foo"}},
			},
			wantCount: 0,
		},
		{
			name: "metric with empty help -- 1 violation, message quotes name",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Help: ""}},
			},
			wantCount:     1,
			wantNameInMsg: "foo",
		},
		{
			// Whitespace-only Help is NOT trimmed — pins the "no-trim"
			// contract. A single space is technically non-empty Help
			// text per this rule, so it passes. Stricter checks belong
			// to a separate rule.
			name: "whitespace-only help is NOT a violation (no trim)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Help: "   "}},
			},
			wantCount: 0,
		},
		{
			name: "metric with empty name and empty help -- message falls back to generic form",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "", Help: ""}},
			},
			wantCount: 1,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
	}
	rule := &MetricHelpRequiredRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.help-required" {
					t.Errorf("RuleID: got %q, want metric.help-required", v.RuleID)
				}
			}
			if tc.wantNameInMsg != "" && !strings.Contains(vios[0].Message, tc.wantNameInMsg) {
				t.Errorf("message missing name %q: %q", tc.wantNameInMsg, vios[0].Message)
			}
		})
	}
}

// TestMetricHelpRequiredRule_LocationAndMessage pins two contracts:
//   - Violation.Location.MetricName mirrors the metric name that triggered
//     the rule so the engine can enrich file/line.
//   - The message carries the metric name so stderr summaries are
//     self-documenting.
func TestMetricHelpRequiredRule_LocationAndMessage(t *testing.T) {
	rule := &MetricHelpRequiredRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{{Name: "foo", Help: ""}},
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
