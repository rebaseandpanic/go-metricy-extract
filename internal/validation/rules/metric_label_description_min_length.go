package rules

import (
	"fmt"
	"unicode/utf8"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// labelDescriptionMinLengthDefault is deliberately LOWER than the
// metric description default (10 vs 20): label descriptions are
// traditionally one-liners like "HTTP status code" or "Client region",
// so the 20-char floor from description-min-length would trip on
// perfectly reasonable prose.
//
// Length is counted in Unicode code points (runes), not bytes, so a
// Cyrillic or CJK label description is measured the way a human reader
// counts.
const labelDescriptionMinLengthDefault = 10

// MetricLabelDescriptionMinLengthRule flags label descriptions that
// are set but shorter than the configured minimum. One violation per
// offending label — a metric with two short descriptions produces two
// violations, each pinpointing the specific label via
// Location.LabelName.
//
// Missing / empty label descriptions are delegated to
// metric.label-description-required.
//
// Warning-severity by default.
type MetricLabelDescriptionMinLengthRule struct{}

// ID implements validation.Rule.
func (MetricLabelDescriptionMinLengthRule) ID() string {
	return "metric.label-description-min-length"
}

// DefaultSeverity implements validation.Rule.
func (MetricLabelDescriptionMinLengthRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricLabelDescriptionMinLengthRule) Description() string {
	return "Label description must be at least N characters (default 10, configurable)"
}

// Validate iterates every label of every metric, emitting a violation
// for each label whose Description is set, non-empty, and shorter than
// the resolved minimum length.
func (r *MetricLabelDescriptionMinLengthRule) Validate(snapshot *model.MetricSnapshot, ctx validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	minLen := resolveMinLength(ctx, r.ID(), labelDescriptionMinLengthDefault)
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		for _, lbl := range m.Labels {
			if lbl.Description == nil || *lbl.Description == "" {
				continue
			}
			got := utf8.RuneCountInString(*lbl.Description)
			if got >= minLen {
				continue
			}
			out = append(out, validation.Violation{
				RuleID:   r.ID(),
				Severity: validation.SeverityWarning,
				Message: fmt.Sprintf(
					"%s label %q description is %d characters, minimum is %d",
					metricLabel(m), lbl.Name, got, minLen,
				),
				Location: &validation.Location{MetricName: m.Name, LabelName: lbl.Name},
			})
		}
	}
	return out
}
