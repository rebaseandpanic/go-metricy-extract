package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricLabelDescriptionMinLengthRule(t *testing.T) {
	const ruleID = "metric.label-description-min-length"

	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		ctx       validation.Context
		wantCount int
	}{
		{
			name: "label description longer than default (10) -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(repeat(15))},
						},
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "label description exactly at default boundary (10) -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(repeat(10))},
						},
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "label description one char below default (9) -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(repeat(9))},
						},
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "label description nil -- skipped (delegated to label-description-required)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: nil},
						},
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "label description empty string -- skipped",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr("")},
						},
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "scalar metric (no labels) -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo"},
				},
			},
			wantCount: 0,
		},
		{
			name: "multiple labels on one metric -- one violation per offender",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "ok", Description: strPtr(repeat(15))},
							{Name: "short1", Description: strPtr("hi")},
							{Name: "empty", Description: strPtr("")},
							{Name: "nil", Description: nil},
							{Name: "short2", Description: strPtr("bye")},
						},
					},
				},
			},
			wantCount: 2,
		},
		{
			name:      "empty snapshot -- 0 violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
		{
			name:      "nil snapshot -- 0 violations",
			snapshot:  nil,
			wantCount: 0,
		},
		{
			// Global floor 20 overrides the rule's hardcoded 10.
			// A label of length 15 that passed the hardcoded default
			// now trips.
			name: "global override raises floor -- previously-fine label now trips",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(repeat(15))},
						},
					},
				},
			},
			ctx:       validation.Context{MinDescriptionLength: 20},
			wantCount: 1,
		},
		{
			name: "global override raises floor -- long label still passes",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(repeat(25))},
						},
					},
				},
			},
			ctx:       validation.Context{MinDescriptionLength: 20},
			wantCount: 0,
		},
		{
			name: "per-rule override lowers floor -- short label passes",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr("hi")},
						},
					},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: 2},
			},
			wantCount: 0,
		},
		{
			name: "per-rule override beats global",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr("abc")},
						},
					},
				},
			},
			ctx: validation.Context{
				MinDescriptionLength: 50,
				RuleMinLength:        map[string]int{ruleID: 3},
			},
			wantCount: 0,
		},
		{
			name: "negative per-rule -- clamped, no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr("x")},
						},
					},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: -4},
			},
			wantCount: 0,
		},
		{
			// Per-rule override for description-min-length must NOT
			// change label behaviour.
			name: "unrelated per-rule override -- no effect",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(repeat(9))},
						},
					},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{"metric.description-min-length": 2},
			},
			wantCount: 1,
		},
		{
			// Rune-count boundary: 10 Cyrillic runes sit exactly on
			// the label default. Byte count (~20) would trivially
			// pass a byte-based check too, but the 9-rune case below
			// separates the two.
			name: "Cyrillic label description: 10 runes exactly -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(strings.Repeat("ж", 10))},
						},
					},
				},
			},
			wantCount: 0,
		},
		{
			// 9 runes: below the default of 10. A byte-based check
			// (18 bytes >= 10) would miss this.
			name: "Cyrillic label description: 9 runes -- violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(strings.Repeat("ж", 9))},
						},
					},
				},
			},
			wantCount: 1,
		},
		{
			// CJK stress: 9 Han ideographs = 27 bytes but 9 runes.
			// Byte-based comparison (27 >= 10) would incorrectly pass.
			name: "CJK label description under minimum triggers violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "foo",
						Labels: []model.LabelDescriptor{
							{Name: "method", Description: strPtr(strings.Repeat("漢", 9))},
						},
					},
				},
			},
			wantCount: 1,
		},
		{
			// Aggregation across multiple metrics AND multiple labels
			// per metric. Four metrics, various labels, three total
			// offenders. Catches both per-metric and cross-metric
			// short-circuit regressions.
			name: "multiple metrics with mixed labels -- correct aggregate",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{
						Name: "a",
						Labels: []model.LabelDescriptor{
							{Name: "ok", Description: strPtr(repeat(15))},
							{Name: "short", Description: strPtr("hi")}, // violation
						},
					},
					{
						Name: "b",
						Labels: []model.LabelDescriptor{
							{Name: "nil_desc", Description: nil},
							{Name: "empty", Description: strPtr("")},
						},
					},
					{
						Name: "c",
						Labels: []model.LabelDescriptor{
							{Name: "short_a", Description: strPtr("hi")}, // violation
							{Name: "short_b", Description: strPtr("yo")}, // violation
						},
					},
					{Name: "d"}, // scalar metric, no labels
				},
			},
			wantCount: 3,
		},
	}
	rule := &MetricLabelDescriptionMinLengthRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, tc.ctx)
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != ruleID {
					t.Errorf("RuleID: got %q, want %q", v.RuleID, ruleID)
				}
				if v.Severity != validation.SeverityWarning {
					t.Errorf("Severity: got %v, want SeverityWarning", v.Severity)
				}
			}
		})
	}
}

func TestMetricLabelDescriptionMinLengthRule_LocationAndMessage(t *testing.T) {
	rule := &MetricLabelDescriptionMinLengthRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{
			{
				Name: "requests_total",
				Labels: []model.LabelDescriptor{
					{Name: "method", Description: strPtr("hi")},
				},
			},
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
	if v.Location.MetricName != "requests_total" {
		t.Errorf("Location.MetricName: got %q, want requests_total", v.Location.MetricName)
	}
	if v.Location.LabelName != "method" {
		t.Errorf("Location.LabelName: got %q, want method", v.Location.LabelName)
	}
	if !strings.Contains(v.Message, "requests_total") {
		t.Errorf("Message must contain metric name: got %q", v.Message)
	}
	if !strings.Contains(v.Message, "method") {
		t.Errorf("Message must contain label name: got %q", v.Message)
	}
	if !strings.Contains(v.Message, "2 characters") {
		t.Errorf("Message should mention observed length 2: got %q", v.Message)
	}
	// Default for this rule is 10 (NOT 20). Pin that so a refactor that
	// silently switches to the generic 20 default trips this assertion.
	if !strings.Contains(v.Message, "minimum is 10") {
		t.Errorf("Message should mention minimum 10 (label-specific default): got %q", v.Message)
	}
}

func TestMetricLabelDescriptionMinLengthRule_ID_Severity_Description(t *testing.T) {
	rule := MetricLabelDescriptionMinLengthRule{}
	if got := rule.ID(); got != "metric.label-description-min-length" {
		t.Errorf("ID: got %q, want metric.label-description-min-length", got)
	}
	if got := rule.DefaultSeverity(); got != validation.SeverityWarning {
		t.Errorf("DefaultSeverity: got %v, want SeverityWarning", got)
	}
	if rule.Description() == "" {
		t.Error("Description: must be non-empty")
	}
}
