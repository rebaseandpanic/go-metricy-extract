package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricCalculationMinLengthRule(t *testing.T) {
	const ruleID = "metric.calculation-min-length"

	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		ctx       validation.Context
		wantCount int
	}{
		{
			name: "calculation longer than default (20) -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(30))},
				},
			},
			wantCount: 0,
		},
		{
			name: "calculation exactly at default boundary (20) -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(20))},
				},
			},
			wantCount: 0,
		},
		{
			name: "calculation one char below default (19) -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(19))},
				},
			},
			wantCount: 1,
		},
		{
			name: "calculation nil -- skipped (delegated to calculation-required)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: nil},
				},
			},
			wantCount: 0,
		},
		{
			name: "calculation empty string -- skipped (delegated to calculation-required)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr("")},
				},
			},
			wantCount: 0,
		},
		{
			name: "empty metric name -- still evaluated",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "", Calculation: strPtr("short")},
				},
			},
			wantCount: 1,
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
			name: "global override raises the floor -- previously-fine calc now trips",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(25))},
				},
			},
			ctx:       validation.Context{MinDescriptionLength: 30},
			wantCount: 1,
		},
		{
			name: "global override raises floor -- long calc still passes",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(35))},
				},
			},
			ctx:       validation.Context{MinDescriptionLength: 30},
			wantCount: 0,
		},
		{
			name: "per-rule override lowers floor -- short calc passes",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(10))},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: 5},
			},
			wantCount: 0,
		},
		{
			name: "per-rule override beats global",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(10))},
				},
			},
			ctx: validation.Context{
				MinDescriptionLength: 50,
				RuleMinLength:        map[string]int{ruleID: 5},
			},
			wantCount: 0,
		},
		{
			name: "negative per-rule -- clamped, no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr("x")},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: -10},
			},
			wantCount: 0,
		},
		{
			// Per-rule override for description-min-length must not
			// leak into this rule's threshold.
			name: "unrelated per-rule override -- no effect",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(repeat(19))},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{"metric.description-min-length": 5},
			},
			wantCount: 1,
		},
		{
			// Rune-count boundary: 20 Cyrillic letters fit inside the
			// default floor even though their UTF-8 byte length is ~40.
			name: "Cyrillic calculation: 20 runes exactly -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(strings.Repeat("ж", 20))},
				},
			},
			wantCount: 0,
		},
		{
			// One rune under the floor -- must violate. Byte count
			// (38 bytes) is still above 20; only a rune-based
			// comparison catches this.
			name: "Cyrillic calculation: 19 runes -- violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr(strings.Repeat("ж", 19))},
				},
			},
			wantCount: 1,
		},
		{
			// CJK stress case: 5 Han ideographs are 15 bytes but only
			// 5 runes, so a byte-based check would erroneously pass.
			name: "CJK calculation under minimum triggers violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Calculation: strPtr("处理请求的")},
				},
			},
			wantCount: 1,
		},
		{
			// Aggregation check: five metrics -- two short, one ok,
			// two missing/empty. Must fire twice, not five times or
			// once. Catches both "short-circuit after first hit" and
			// "counts nil/empty as a violation" regressions.
			name: "multiple metrics mixed -- correct aggregate count",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "a", Calculation: strPtr(repeat(20))},   // ok
					{Name: "b", Calculation: strPtr("short")},      // <20 -> violation
					{Name: "c", Calculation: nil},                  // skip
					{Name: "d", Calculation: strPtr("")},           // skip
					{Name: "e", Calculation: strPtr(repeat(3))},    // <20 -> violation
				},
			},
			wantCount: 2,
		},
	}
	rule := &MetricCalculationMinLengthRule{}
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

func TestMetricCalculationMinLengthRule_LocationAndMessage(t *testing.T) {
	rule := &MetricCalculationMinLengthRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{
			{Name: "requests_total", Calculation: strPtr("sum")},
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
	if !strings.Contains(v.Message, "requests_total") {
		t.Errorf("Message must contain metric name: got %q", v.Message)
	}
	if !strings.Contains(v.Message, "3 characters") {
		t.Errorf("Message should mention observed length 3: got %q", v.Message)
	}
	if !strings.Contains(v.Message, "minimum is 20") {
		t.Errorf("Message should mention minimum 20: got %q", v.Message)
	}
	if !strings.Contains(v.Message, "calculation") {
		t.Errorf("Message should mention 'calculation' so readers can distinguish from description: got %q", v.Message)
	}
}

func TestMetricCalculationMinLengthRule_ID_Severity_Description(t *testing.T) {
	rule := MetricCalculationMinLengthRule{}
	if got := rule.ID(); got != "metric.calculation-min-length" {
		t.Errorf("ID: got %q, want metric.calculation-min-length", got)
	}
	if got := rule.DefaultSeverity(); got != validation.SeverityWarning {
		t.Errorf("DefaultSeverity: got %v, want SeverityWarning", got)
	}
	if rule.Description() == "" {
		t.Error("Description: must be non-empty")
	}
}
