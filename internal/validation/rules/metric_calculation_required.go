package rules

import (
	"fmt"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricCalculationRequiredRule flags metrics whose @metric calculation
// annotation is missing or empty. The calculation field documents how
// the metric is computed (e.g. "count of HTTP 5xx responses per 10s
// window") and is the second mandatory piece of prose alongside
// description.
type MetricCalculationRequiredRule struct{}

// ID implements validation.Rule.
func (MetricCalculationRequiredRule) ID() string { return "metric.calculation-required" }

// DefaultSeverity implements validation.Rule.
func (MetricCalculationRequiredRule) DefaultSeverity() validation.Severity {
	return validation.SeverityError
}

// Description implements validation.Rule.
func (MetricCalculationRequiredRule) Description() string {
	return "Annotation calculation must be set"
}

// Validate flags metrics whose Calculation is nil or empty, collapsing
// both absent-annotation and blank-value cases into one violation —
// downstream consumers treat them identically.
func (r *MetricCalculationRequiredRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Calculation != nil && *m.Calculation != "" {
			continue
		}
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityError,
			Message:  fmt.Sprintf("%s has no calculation annotation (@metric calculation)", metricLabel(m)),
			Location: &validation.Location{MetricName: m.Name},
		})
	}
	return out
}
