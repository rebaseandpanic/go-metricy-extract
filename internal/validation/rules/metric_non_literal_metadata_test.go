package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricNonLiteralMetadataRule(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		wantCount int
	}{
		{
			name: "single non-literal Name warning -- 1 violation",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"X: non-literal Name; skipping metric",
				},
			},
			wantCount: 1,
		},
		{
			name: "single non-literal Help warning -- 1 violation",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"Y: non-literal Help; skipping metric",
				},
			},
			wantCount: 1,
		},
		{
			name: "two warnings of different kinds -- 2 violations",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"X: non-literal Help; skipping metric",
					"Y: non-literal Name; skipping metric",
				},
			},
			wantCount: 2,
		},
		{
			name: "unrelated warnings -- no violations",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"foo.go: parse error: unexpected token",
					"Z: non-literal options argument; skipping metric",
					"some other warning",
				},
			},
			wantCount: 0,
		},
		{
			name: "label-level non-literal warning -- no violation",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"X: non-literal label name in labels slice; continuing with extracted labels",
				},
			},
			wantCount: 0,
		},
		{
			name: "options-argument non-literal warning -- no violation",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"X: non-literal options argument; skipping metric",
				},
			},
			wantCount: 0,
		},
		{
			name: "Vec zero literal labels warning -- no violation",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"X: Vec labels contain zero literal names; emitting metric without labels",
				},
			},
			wantCount: 0,
		},
		{
			name: "mixed warnings -- only non-literal Name/Help counted",
			snapshot: &model.MetricSnapshot{
				ExtractionWarnings: []string{
					"foo.go: parse error: unexpected token",
					"A: non-literal Name; skipping metric",
					"B: some unrelated issue",
					"C: non-literal Help; skipping metric",
				},
			},
			wantCount: 2,
		},
		{
			name: "no ExtractionWarnings field populated -- no violations",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{Name: "foo", Type: "counter"}},
			},
			wantCount: 0,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
		{
			name:      "nil snapshot -- no violations (no panic)",
			snapshot:  nil,
			wantCount: 0,
		},
	}
	rule := &MetricNonLiteralMetadataRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.non-literal-metadata" {
					t.Errorf("RuleID: got %q, want metric.non-literal-metadata", v.RuleID)
				}
				if v.Severity != validation.SeverityWarning {
					t.Errorf("Severity: got %v, want SeverityWarning", v.Severity)
				}
			}
		})
	}
}

// TestMetricNonLiteralMetadataRule_LocationFromPrefix pins the contract
// that when the warning is in the conventional "<varName>: ..." form
// the rule forwards the varName into Location.MetricName.
func TestMetricNonLiteralMetadataRule_LocationFromPrefix(t *testing.T) {
	rule := &MetricNonLiteralMetadataRule{}
	snap := &model.MetricSnapshot{
		ExtractionWarnings: []string{
			"MyMetric: non-literal Name; skipping metric",
		},
	}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 1 {
		t.Fatalf("violations: got %d, want 1", len(vios))
	}
	v := vios[0]
	if v.Location == nil {
		t.Fatalf("Location must be non-nil when var-name prefix is present: %+v", v)
	}
	if v.Location.MetricName != "MyMetric" {
		t.Errorf("Location.MetricName: got %q, want MyMetric", v.Location.MetricName)
	}
}

// TestMetricNonLiteralMetadataRule_NoLocationWithoutPrefix pins the
// contract that warnings without a parseable "<ident>: " prefix land
// as a violation with a nil Location rather than crashing or guessing.
func TestMetricNonLiteralMetadataRule_NoLocationWithoutPrefix(t *testing.T) {
	rule := &MetricNonLiteralMetadataRule{}
	snap := &model.MetricSnapshot{
		ExtractionWarnings: []string{
			"non-literal Name; skipping metric",
		},
	}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 1 {
		t.Fatalf("violations: got %d, want 1", len(vios))
	}
	if vios[0].Location != nil {
		t.Errorf("Location: got %+v, want nil (no var-name prefix)", vios[0].Location)
	}
}

// TestMetricNonLiteralMetadataRule_PreservesOrder pins the contract that
// violations are emitted in the same order as the warnings appear in
// Snapshot.ExtractionWarnings. Order stability matters for golden-file
// reports and for deterministic CLI output.
func TestMetricNonLiteralMetadataRule_PreservesOrder(t *testing.T) {
	snap := &model.MetricSnapshot{
		ExtractionWarnings: []string{
			"A: non-literal Name; skipping metric",
			"B: non-literal Help; skipping metric",
			"C: non-literal Name; skipping metric",
		},
	}
	rule := &MetricNonLiteralMetadataRule{}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 3 {
		t.Fatalf("want 3 violations, got %d", len(vios))
	}
	wantNames := []string{"A", "B", "C"}
	for i, want := range wantNames {
		if vios[i].Location == nil || vios[i].Location.MetricName != want {
			t.Errorf("vios[%d].Location.MetricName: got %q, want %q",
				i, locName(vios[i].Location), want)
		}
	}
}

// locName is a nil-safe accessor for validation.Location.MetricName.
func locName(l *validation.Location) string {
	if l == nil {
		return "<nil>"
	}
	return l.MetricName
}

// TestMetricNonLiteralMetadataRule_InvalidPrefixYieldsNilLocation pins the
// contract that warnings whose prefix is not a valid Go identifier (or is
// empty) still emit a violation, but without a Location — the parser must
// fail closed, not guess. Covers S5/S6.
func TestMetricNonLiteralMetadataRule_InvalidPrefixYieldsNilLocation(t *testing.T) {
	cases := []struct {
		name    string
		warning string
	}{
		{
			name:    "non-identifier prefix (dash)",
			warning: "not-an-ident: non-literal Name; skipping metric",
		},
		{
			name:    "prefix starts with colon",
			warning: ": non-literal Name; skipping metric",
		},
	}
	rule := &MetricNonLiteralMetadataRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snap := &model.MetricSnapshot{
				ExtractionWarnings: []string{tc.warning},
			}
			vios := rule.Validate(snap, validation.Context{})
			if len(vios) != 1 {
				t.Fatalf("violations: got %d, want 1", len(vios))
			}
			if vios[0].Location != nil {
				t.Errorf("Location: got %+v, want nil (invalid prefix must not produce Location)",
					vios[0].Location)
			}
		})
	}
}

// TestMetricNonLiteralMetadataRule_MessageIsVerbatim pins the contract
// that the violation message is the raw warning string — downstream
// tools that parse reports can rely on the extractor phrasing.
func TestMetricNonLiteralMetadataRule_MessageIsVerbatim(t *testing.T) {
	rule := &MetricNonLiteralMetadataRule{}
	want := "Counter1: non-literal Name; skipping metric"
	snap := &model.MetricSnapshot{
		ExtractionWarnings: []string{want},
	}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 1 {
		t.Fatalf("violations: got %d, want 1", len(vios))
	}
	if vios[0].Message != want {
		t.Errorf("Message: got %q, want %q", vios[0].Message, want)
	}
	if !strings.Contains(vios[0].Message, "Counter1") {
		t.Errorf("Message should contain var name; got %q", vios[0].Message)
	}
}
