package rules

import (
	"fmt"
	"regexp"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// snakeCaseRe matches metric names composed of a lowercase letter
// followed by lowercase letters, digits, or underscores. Prometheus
// itself accepts `[a-zA-Z_:][a-zA-Z0-9_:]*` — this rule is stricter on
// purpose: the organisation-wide convention is snake_case for all
// metric names, with colons reserved for recording-rule aggregators
// (which should not appear in raw snapshots anyway).
var snakeCaseRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// MetricNameSnakeCaseRule flags metric names that are not strictly
// snake_case. CamelCase, kebab-case, dotted, colon-separated, or names
// starting with a digit / underscore all trigger the rule.
//
// Warning-severity by default.
type MetricNameSnakeCaseRule struct{}

// ID implements validation.Rule.
func (MetricNameSnakeCaseRule) ID() string { return "metric.name-snake-case" }

// DefaultSeverity implements validation.Rule.
func (MetricNameSnakeCaseRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricNameSnakeCaseRule) Description() string {
	return "Metric name must be snake_case (lowercase letters, digits, underscores)"
}

// Validate emits one violation per metric whose Name does not match
// snakeCaseRe. Empty names are skipped — that's the job of
// metric.name-required. The check is a single regexp evaluation per
// metric and runs in O(N).
func (r *MetricNameSnakeCaseRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Name == "" {
			continue
		}
		if snakeCaseRe.MatchString(m.Name) {
			continue
		}
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityWarning,
			Message: fmt.Sprintf(
				"metric name %q is not snake_case; use lowercase letters, digits, and underscores only",
				m.Name,
			),
			Location: &validation.Location{MetricName: m.Name},
		})
	}
	return out
}
