package rules

import (
	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// calculationMinLengthDefault mirrors descriptionMinLengthDefault —
// calculation annotations document how a metric is computed and tend to
// be at least as verbose as a description.
//
// Length is counted in Unicode code points (runes), not bytes, so a
// Cyrillic or CJK calculation is measured the way a human reader counts.
const calculationMinLengthDefault = 20

// MetricCalculationMinLengthRule flags metrics whose calculation
// annotation is shorter than the configured minimum. Like the
// description-min-length rule, missing / empty calculations are
// delegated to metric.calculation-required and are NOT re-flagged here.
//
// Warning-severity by default.
type MetricCalculationMinLengthRule struct{}

// ID implements validation.Rule.
func (MetricCalculationMinLengthRule) ID() string { return "metric.calculation-min-length" }

// DefaultSeverity implements validation.Rule.
func (MetricCalculationMinLengthRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricCalculationMinLengthRule) Description() string {
	return "Annotation calculation must be at least N characters (default 20, configurable)"
}

// Validate emits one violation per metric whose Calculation is set and
// non-empty but shorter than the resolved minimum length. Length is
// measured in runes via checkStringMinLength.
func (r *MetricCalculationMinLengthRule) Validate(snapshot *model.MetricSnapshot, ctx validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	minLen := resolveMinLength(ctx, r.ID(), calculationMinLengthDefault)
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if v := checkStringMinLength(m, "calculation", m.Calculation, r.ID(), minLen); v != nil {
			out = append(out, *v)
		}
	}
	return out
}
