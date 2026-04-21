package rules

import (
	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// descriptionMinLengthDefault is the hardcoded fallback for the
// description-min-length rule when neither Context.RuleMinLength[ruleID]
// nor Context.MinDescriptionLength is set. 20 characters is enough to
// encode a short but meaningful sentence like "Requests served total".
//
// Length is counted in Unicode code points (runes), not bytes, so a
// Cyrillic or CJK description is measured the way a human reader counts.
const descriptionMinLengthDefault = 20

// MetricDescriptionMinLengthRule flags metrics whose description
// annotation is shorter than the configured minimum. The rule is
// deliberately LAYERED on top of metric.description-required:
// missing / empty descriptions are NOT re-flagged here. That separation
// lets a user silence the length rule (e.g. --skip-rule) without losing
// the stronger "description must exist" contract.
//
// Warning-severity by default.
type MetricDescriptionMinLengthRule struct{}

// ID implements validation.Rule.
func (MetricDescriptionMinLengthRule) ID() string { return "metric.description-min-length" }

// DefaultSeverity implements validation.Rule.
func (MetricDescriptionMinLengthRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricDescriptionMinLengthRule) Description() string {
	return "Annotation description must be at least N characters (default 20, configurable)"
}

// Validate emits one violation per metric whose Description is set and
// non-empty but shorter than the resolved minimum length. nil/"" cases
// are delegated to metric.description-required. Length is measured in
// runes via checkStringMinLength.
func (r *MetricDescriptionMinLengthRule) Validate(snapshot *model.MetricSnapshot, ctx validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	minLen := resolveMinLength(ctx, r.ID(), descriptionMinLengthDefault)
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if v := checkStringMinLength(m, "description", m.Description, r.ID(), minLen); v != nil {
			out = append(out, *v)
		}
	}
	return out
}
