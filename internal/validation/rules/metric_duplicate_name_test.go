package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricDuplicateNameRule(t *testing.T) {
	cases := []struct {
		name        string
		snapshot    *model.MetricSnapshot
		wantCount   int
		wantCountIn string // substring that must appear in the first violation message (e.g. "3 times")
	}{
		{
			name: "two metrics same name -- 1 violation",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter"}, {Name: "a", Type: "counter"},
			}},
			wantCount:   1,
			wantCountIn: "2 times",
		},
		{
			name: "three metrics same name -- 1 violation with count 3",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "b"}, {Name: "b"}, {Name: "b"},
			}},
			wantCount:   1,
			wantCountIn: "3 times",
		},
		{
			name: "all unique names -- no violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "a"}, {Name: "b"}, {Name: "c"},
			}},
			wantCount: 0,
		},
		{
			name: "two duplicate groups -- 2 violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "a"}, {Name: "a"},
				{Name: "b"}, {Name: "b"}, {Name: "b"},
				{Name: "c"},
			}},
			wantCount: 2,
		},
		{
			name: "two metrics with empty names also duplicate",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: ""}, {Name: ""},
			}},
			wantCount: 1,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
	}
	rule := &MetricDuplicateNameRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.duplicate-name" {
					t.Errorf("RuleID: got %q, want metric.duplicate-name", v.RuleID)
				}
			}
			if tc.wantCountIn != "" && !strings.Contains(vios[0].Message, tc.wantCountIn) {
				t.Errorf("message missing %q: %q", tc.wantCountIn, vios[0].Message)
			}
		})
	}
}

// TestMetricDuplicateNameRule_LocationAndMessage pins Location.MetricName
// and message content for a duplicate-name violation. The rule must emit
// the duplicated name in both fields so engine enrichment and the stderr
// summary agree on which metric is at fault.
func TestMetricDuplicateNameRule_LocationAndMessage(t *testing.T) {
	rule := &MetricDuplicateNameRule{}
	snap := &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
		{Name: "foo", Type: "counter"},
		{Name: "foo", Type: "counter"},
	}}
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
