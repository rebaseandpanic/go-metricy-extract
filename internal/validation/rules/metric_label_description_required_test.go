package rules

import (
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricLabelDescriptionRequiredRule(t *testing.T) {
	cases := []struct {
		name       string
		snapshot   *model.MetricSnapshot
		wantCount  int
		wantLabels []string // sorted label names we expect to see in violations
	}{
		{
			name: "all labels described -- no violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{
					{Name: "method", Description: strPtr("HTTP method")},
					{Name: "code", Description: strPtr("Status code")},
				},
			}}},
			wantCount: 0,
		},
		{
			name: "one label missing description -- 1 violation",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{
					{Name: "method", Description: strPtr("HTTP method")},
					{Name: "code", Description: nil},
				},
			}}},
			wantCount:  1,
			wantLabels: []string{"code"},
		},
		{
			name: "empty-string description counts as missing",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{
					{Name: "method", Description: strPtr("")},
				},
			}}},
			wantCount:  1,
			wantLabels: []string{"method"},
		},
		{
			name: "two labels both missing -- 2 violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{
					{Name: "a", Description: nil},
					{Name: "b", Description: strPtr("")},
				},
			}}},
			wantCount:  2,
			wantLabels: []string{"a", "b"},
		},
		{
			name: "scalar metric (no labels) -- no violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "scalar", Labels: nil,
			}}},
			wantCount: 0,
		},
		{
			// Empty (non-nil) Labels slice behaves the same as nil: no
			// labels to describe, no violations. Pins that the rule's
			// label-loop guard works for both nil and len==0.
			name: "empty labels slice yields no violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "scalar", Labels: []model.LabelDescriptor{},
			}}},
			wantCount: 0,
		},
		{
			// Whitespace-only description is NOT trimmed — pins the
			// no-trim contract across every required-style rule in the
			// registry.
			name: "whitespace-only label description is NOT a violation (no trim)",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "m", Labels: []model.LabelDescriptor{{Name: "m", Description: strPtr("   ")}},
			}}},
			wantCount: 0,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
	}
	rule := &MetricLabelDescriptionRequiredRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for i, v := range vios {
				if v.RuleID != "metric.label-description-required" {
					t.Errorf("RuleID[%d]: got %q, want metric.label-description-required", i, v.RuleID)
				}
				if v.Location == nil || v.Location.LabelName == "" {
					t.Errorf("Location.LabelName must be set for label-scoped violation: %+v", v)
				}
			}
			if len(tc.wantLabels) > 0 {
				got := map[string]bool{}
				for _, v := range vios {
					got[v.Location.LabelName] = true
				}
				for _, want := range tc.wantLabels {
					if !got[want] {
						t.Errorf("missing expected label %q in violations (got labels: %v)", want, got)
					}
				}
			}
		})
	}
}
