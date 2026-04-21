package rules

import (
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestMetricNameSnakeCaseRule(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  *model.MetricSnapshot
		wantCount int
	}{
		{
			name: "plain snake_case -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo_bar_total", Type: "counter"},
				},
			},
			wantCount: 0,
		},
		{
			name: "lowercase letters + digits -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo_123", Type: "counter"},
				},
			},
			wantCount: 0,
		},
		{
			name: "single lowercase letter -- no violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "x", Type: "gauge"},
				},
			},
			wantCount: 0,
		},
		{
			name: "CamelCase -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "FooBarTotal", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "mixedCase -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "fooBarTotal", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "kebab-case with dashes -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo-bar", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "dotted name -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo.bar", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "leading underscore -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "_foo", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "leading digit -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "1foo", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			// Prometheus allows colons for recording-rule aggregators.
			// Raw source metrics should never contain colons; flag them.
			name: "colon-separated name -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo:bar", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "whitespace inside -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo bar", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "Cyrillic in name -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "метрика_total", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "Latin with diacritic -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo_é", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "emoji in name -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "foo_🔥", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "CJK in name -- 1 violation",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "メトリック_total", Type: "counter"},
				},
			},
			wantCount: 1,
		},
		{
			name: "empty name -- skipped (delegated to name-required)",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "", Type: "counter"},
				},
			},
			wantCount: 0,
		},
		{
			name: "multiple metrics mixed -- one violation per offender",
			snapshot: &model.MetricSnapshot{
				Metrics: []model.MetricDescriptor{
					{Name: "ok_metric", Type: "gauge"},
					{Name: "BadName", Type: "counter"},
					{Name: "also_ok", Type: "counter"},
					{Name: "bad-dashes", Type: "counter"},
				},
			},
			wantCount: 2,
		},
		{
			name:      "empty snapshot -- no violations",
			snapshot:  &model.MetricSnapshot{},
			wantCount: 0,
		},
		{
			name:      "nil snapshot -- no violations",
			snapshot:  nil,
			wantCount: 0,
		},
	}
	rule := &MetricNameSnakeCaseRule{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vios := rule.Validate(tc.snapshot, validation.Context{})
			if len(vios) != tc.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vios), tc.wantCount, vios)
			}
			for _, v := range vios {
				if v.RuleID != "metric.name-snake-case" {
					t.Errorf("RuleID: got %q, want metric.name-snake-case", v.RuleID)
				}
				if v.Severity != validation.SeverityWarning {
					t.Errorf("Severity: got %v, want SeverityWarning", v.Severity)
				}
			}
		})
	}
}

func TestMetricNameSnakeCaseRule_LocationAndMessage(t *testing.T) {
	rule := &MetricNameSnakeCaseRule{}
	snap := &model.MetricSnapshot{
		Metrics: []model.MetricDescriptor{
			{Name: "FooBar", Type: "counter"},
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
	if v.Location.MetricName != "FooBar" {
		t.Errorf("Location.MetricName: got %q, want FooBar", v.Location.MetricName)
	}
	if !strings.Contains(v.Message, "FooBar") {
		t.Errorf("Message must contain metric name: got %q", v.Message)
	}
	// S4: message must mention "snake_case" so autofixers and readers can
	// key on a characteristic token rather than only the metric name.
	if !strings.Contains(v.Message, "snake_case") {
		t.Errorf("message %q should mention snake_case", v.Message)
	}
}
