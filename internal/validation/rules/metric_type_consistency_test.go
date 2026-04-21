package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricTypeConsistencyRule(t *testing.T) {
	cases := []struct {
		name         string
		snapshot     *model.MetricSnapshot
		wantCount    int
		wantTypesIn  string // substring required in first violation message
	}{
		{
			name: "counter and gauge share a name -- 1 violation",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter"}, {Name: "x", Type: "gauge"},
			}},
			wantCount:   1,
			wantTypesIn: "counter, gauge",
		},
		{
			name: "counter and counter same name -- consistent, no violation",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "x", Type: "counter"}, {Name: "x", Type: "counter"},
			}},
			wantCount: 0,
		},
		{
			name: "three distinct types for same name -- 1 violation listing all three",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "y", Type: "counter"},
				{Name: "y", Type: "gauge"},
				{Name: "y", Type: "histogram"},
			}},
			wantCount:   1,
			wantTypesIn: "counter, gauge, histogram",
		},
		{
			name: "different names with different types -- no violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter"},
				{Name: "b", Type: "gauge"},
			}},
			wantCount: 0,
		},
		{
			name: "two conflicting names -- 2 violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
				{Name: "a", Type: "counter"}, {Name: "a", Type: "gauge"},
				{Name: "b", Type: "histogram"}, {Name: "b", Type: "summary"},
			}},
			wantCount: 2,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
	}
	rule := &MetricTypeConsistencyRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.type-consistency" {
					t.Errorf("RuleID: got %q, want metric.type-consistency", v.RuleID)
				}
			}
			if tc.wantTypesIn != "" && !strings.Contains(vios[0].Message, tc.wantTypesIn) {
				t.Errorf("message missing %q (alphabetical order expected): %q", tc.wantTypesIn, vios[0].Message)
			}
		})
	}
}

// TestMetricTypeConsistencyRule_LocationAndMessage pins the rule's
// Location.MetricName and message contents against a simple two-type
// conflict on name "foo". The engine relies on the Location field to
// attach source info; the stderr summary relies on the name being in the
// message.
func TestMetricTypeConsistencyRule_LocationAndMessage(t *testing.T) {
	rule := &MetricTypeConsistencyRule{}
	snap := &model.MetricSnapshot{Metrics: []model.MetricDescriptor{
		{Name: "foo", Type: "counter"},
		{Name: "foo", Type: "gauge"},
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
