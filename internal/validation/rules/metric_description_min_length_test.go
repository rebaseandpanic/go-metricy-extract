package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// repeat returns a string of n 'x' characters. Helps author descriptions
// of a precise length for boundary-condition tests without obscuring the
// intent behind concrete prose. strPtr lives in
// metric_description_required_test.go (same package).
func repeat(n int) string { return strings.Repeat("x", n) }

func TestMetricDescriptionMinLengthRule(t *testing.T) {
	const ruleID = "metric.description-min-length"

	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		ctx       validation.Context
		wantCount int
	}{
		{
			name: "description longer than default (20) -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(30))},
				},
			},
			wantCount: 0,
		},
		{
			name: "description exactly at default boundary (20) -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(20))},
				},
			},
			wantCount: 0,
		},
		{
			name: "description one char below default (19) -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(19))},
				},
			},
			wantCount: 1,
		},
		{
			name: "description nil -- skipped (delegated to description-required)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: nil},
				},
			},
			wantCount: 0,
		},
		{
			name: "description empty string -- skipped (delegated to description-required)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr("")},
				},
			},
			wantCount: 0,
		},
		{
			name: "empty metric name -- still evaluated (length is independent)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "", Description: strPtr("short")},
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
			// Global override: description of length 25 is fine against
			// default 20, but should trip when the global floor is 30.
			name: "global MinDescriptionLength override -- short description now violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(25))},
				},
			},
			ctx:       validation.Context{MinDescriptionLength: 30},
			wantCount: 1,
		},
		{
			name: "global MinDescriptionLength override -- long description still passes",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(35))},
				},
			},
			ctx:       validation.Context{MinDescriptionLength: 30},
			wantCount: 0,
		},
		{
			// Per-rule override: user lowered the floor to 5, so a
			// description of length 10 should pass.
			name: "per-rule override lowers the bar -- description passes",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(10))},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: 5},
			},
			wantCount: 0,
		},
		{
			// Per-rule override WINS over global: global says 50,
			// per-rule says 5, description of length 10 passes.
			name: "per-rule override beats global",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(10))},
				},
			},
			ctx: validation.Context{
				MinDescriptionLength: 50,
				RuleMinLength:        map[string]int{ruleID: 5},
			},
			wantCount: 0,
		},
		{
			// Negative per-rule value is clamped to 0 — no violations
			// possible because every string has len >= 0.
			name: "negative per-rule -- clamped, no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr("x")},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: -10},
			},
			wantCount: 0,
		},
		{
			// Per-rule override for a DIFFERENT rule id must not
			// influence this rule's threshold.
			name: "unrelated per-rule override -- no effect",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(repeat(19))},
				},
			},
			ctx: validation.Context{
				RuleMinLength: map[string]int{"metric.calculation-min-length": 5},
			},
			wantCount: 1,
		},
		{
			name: "mixed metrics -- one violation per offender",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "ok", Description: strPtr(repeat(40))},
					{Name: "short_a", Description: strPtr(repeat(5))},
					{Name: "empty", Description: strPtr("")},
					{Name: "nil_desc", Description: nil},
					{Name: "short_b", Description: strPtr(repeat(3))},
				},
			},
			wantCount: 2,
		},
		{
			// Rune-count boundary: 20 Cyrillic letters fit inside the
			// default floor even though their UTF-8 byte length is ~40.
			// If the rule ever regresses to `len(string)` this case
			// flips to 1 violation.
			name: "Cyrillic description: 20 runes exactly -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(strings.Repeat("ж", 20))},
				},
			},
			wantCount: 0,
		},
		{
			// One rune under the floor -- must violate. Byte count
			// (38 bytes) is still way above 20; only a rune-based
			// comparison catches this.
			name: "Cyrillic description: 19 runes -- violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr(strings.Repeat("ж", 19))},
				},
			},
			wantCount: 1,
		},
		{
			// CJK stress case: 5 Han ideographs are 15 bytes but only
			// 5 runes, so a byte-based check would erroneously pass.
			name: "CJK description under minimum triggers violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo", Description: strPtr("处理请求的")},
				},
			},
			wantCount: 1,
		},
	}
	rule := &MetricDescriptionMinLengthRule{}
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

func TestMetricDescriptionMinLengthRule_LocationAndMessage(t *testing.T) {
	rule := &MetricDescriptionMinLengthRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{
			{Name: "requests_total", Description: strPtr("hi")},
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
	// Message carries both observed and required length so autofixers
	// can surface "got X, need Y" without re-deriving either side.
	if !strings.Contains(v.Message, "2 characters") {
		t.Errorf("Message should mention observed length 2: got %q", v.Message)
	}
	if !strings.Contains(v.Message, "minimum is 20") {
		t.Errorf("Message should mention the configured minimum 20: got %q", v.Message)
	}
}

func TestMetricDescriptionMinLengthRule_ID_Severity_Description(t *testing.T) {
	rule := MetricDescriptionMinLengthRule{}
	if got := rule.ID(); got != "metric.description-min-length" {
		t.Errorf("ID: got %q, want metric.description-min-length", got)
	}
	if got := rule.DefaultSeverity(); got != validation.SeverityWarning {
		t.Errorf("DefaultSeverity: got %v, want SeverityWarning", got)
	}
	if rule.Description() == "" {
		t.Error("Description: must be non-empty")
	}
}
