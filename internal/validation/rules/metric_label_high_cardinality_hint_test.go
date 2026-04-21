package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// TestMetricLabelHighCardinalityHintRule exercises the full matrix:
// default patterns, exact-match semantics (no substring, no case-fold),
// CLI-style overrides via Context.HighCardinalityLabels, the nil-vs-empty
// distinction, and the no-labels / empty-snapshot edge cases.
func TestMetricLabelHighCardinalityHintRule(t *testing.T) {
	cases := []struct {
		name       string
		snapshot   *model.MetricSnapshot
		ctx        validation.Context
		wantCount  int
		wantLabels []string
	}{
		{
			name: "default list catches user_id",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "requests_total", Labels: []model.LabelDescriptor{
					{Name: "user_id"},
					{Name: "method"},
				},
			}}},
			wantCount:  1,
			wantLabels: []string{"user_id"},
		},
		{
			name: "multiple high-card labels on one metric -- N violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "events_total", Labels: []model.LabelDescriptor{
					{Name: "user_id"},
					{Name: "email"},
					{Name: "ip"},
					{Name: "method"}, // not high-card
				},
			}}},
			wantCount:  3,
			wantLabels: []string{"email", "ip", "user_id"},
		},
		{
			// tenant_id is a common org-level label but intentionally NOT
			// in the default list — adding it would false-positive for
			// every multi-tenant service. Pins that decision.
			name: "obscure label tenant_id -- 0 violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{{Name: "tenant_id"}},
			}}},
			wantCount: 0,
		},
		{
			// Exact-match contract: user_id_v2 contains "user_id" as a
			// substring but is NOT flagged. Prevents accidental
			// false-positives on versioned / suffixed label names.
			name: "user_id_v2 is NOT a substring match",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{{Name: "user_id_v2"}},
			}}},
			wantCount: 0,
		},
		{
			// Case-sensitivity contract: default list is lowercase, so
			// UserID / USER_ID do not match. If a project uses CamelCase
			// labels it must add them via --high-cardinality-labels.
			name: "UserID is case-sensitive -- does NOT match lowercase default",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{{Name: "UserID"}},
			}}},
			wantCount: 0,
		},
		{
			// Override via Context: when the user passes a non-nil slice,
			// the default list is REPLACED. Labels in the default list
			// (user_id) are no longer flagged; labels in the override
			// (tenant_id) are.
			name: "override replaces default list entirely",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{
					{Name: "user_id"},   // default-only, must NOT fire
					{Name: "tenant_id"}, // override-only, must fire
				},
			}}},
			ctx:        validation.Context{HighCardinalityLabels: []string{"tenant_id"}},
			wantCount:  1,
			wantLabels: []string{"tenant_id"},
		},
		{
			// Empty-but-non-nil override is a "no patterns" signal: the
			// rule becomes a no-op even though it's running. Distinct from
			// the nil default, which falls back to defaultHighCardinalityLabels.
			name: "empty non-nil override silences the rule",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "req", Labels: []model.LabelDescriptor{{Name: "user_id"}},
			}}},
			ctx:       validation.Context{HighCardinalityLabels: []string{}},
			wantCount: 0,
		},
		{
			name: "scalar metric (no labels) -- no violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "scalar", Labels: nil,
			}}},
			wantCount: 0,
		},
		{
			name: "empty labels slice -- no violations",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "scalar", Labels: []model.LabelDescriptor{},
			}}},
			wantCount: 0,
		},
		{
			name:      "nil snapshot -- no violations",
			snapshot:  nil,
			wantCount: 0,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
		{
			// Unnamed metric is skipped — the violation message would
			// render as `metric label "..."` with no useful context, and
			// metric.name-required already covers the missing-name case.
			name: "metric with empty name is skipped",
			snapshot: &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
				Name: "", Labels: []model.LabelDescriptor{{Name: "user_id"}},
			}}},
			wantCount: 0,
		},
	}
	rule := &MetricLabelHighCardinalityHintRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, tc.ctx)
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for i, v := range vios {
				if v.RuleID != "metric.label-high-cardinality-hint" {
					t.Errorf("RuleID[%d]: got %q, want metric.label-high-cardinality-hint", i, v.RuleID)
				}
				if v.Location == nil || v.Location.LabelName == "" || v.Location.MetricName == "" {
					t.Errorf("Location must set MetricName + LabelName: %+v", v.Location)
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

// TestMetricLabelHighCardinalityHintRule_MessageContainsContext pins the
// user-visible shape of the violation message: metric name, label name,
// and the "high-cardinality" keyword all appear so grep-over-stderr
// stays useful.
func TestMetricLabelHighCardinalityHintRule_MessageContainsContext(t *testing.T) {
	snap := &model.MetricSnapshot{Metrics: []model.MetricDescriptor{{
		Name: "http_requests_total", Labels: []model.LabelDescriptor{{Name: "user_id"}},
	}}}
	rule := &MetricLabelHighCardinalityHintRule{}
	vios := rule.Validate(snap, validation.Context{})
	if len(vios) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(vios), vios)
	}
	msg := vios[0].Message
	for _, want := range []string{"http_requests_total", "user_id", "high-cardinality"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q: %q", want, msg)
		}
	}
}

// TestMetricLabelHighCardinalityHintRule_DefaultPatternsCovered exercises
// a representative sample of the default list, guarding against silent
// typos in the `defaultHighCardinalityLabels` map. One sub-test per
// pattern keeps failure output scoped to the exact entry that regressed.
func TestMetricLabelHighCardinalityHintRule_DefaultPatternsCovered(t *testing.T) {
	patterns := []string{
		"user_id", "userid",
		"email", "email_address",
		"ip", "client_ip",
		"uuid", "guid",
		"session_id", "session",
		"request_id", "trace_id", "span_id",
		"path", "url", "uri",
		"query", "query_string",
		"hostname", "host",
	}
	rule := &MetricLabelHighCardinalityHintRule{}
	for _, p := range patterns {
		t.Run(p, func(t *testing.T) {
			snap := &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{{
					Name:   "m_total",
					Type:   "counter",
					Labels: []model.LabelDescriptor{{Name: p}},
				}},
			}
			vios := rule.Validate(snap, validation.Context{})
			if len(vios) != 1 {
				t.Errorf("pattern %q should trigger exactly 1 violation; got %d", p, len(vios))
			}
		})
	}
}

// TestMetricLabelHighCardinalityHintRule_Contract locks in the Rule
// interface contract: stable ID, warning severity, non-empty description.
func TestMetricLabelHighCardinalityHintRule_Contract(t *testing.T) {
	rule := &MetricLabelHighCardinalityHintRule{}
	if id := rule.ID(); id != "metric.label-high-cardinality-hint" {
		t.Errorf("ID: got %q, want metric.label-high-cardinality-hint", id)
	}
	if sev := rule.DefaultSeverity(); sev != validation.SeverityWarning {
		t.Errorf("DefaultSeverity: got %v, want Warning", sev)
	}
	if desc := rule.Description(); desc == "" {
		t.Errorf("Description must not be empty")
	}
}
