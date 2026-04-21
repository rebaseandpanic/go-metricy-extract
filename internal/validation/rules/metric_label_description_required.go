package rules

import (
	"fmt"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricLabelDescriptionRequiredRule flags declared labels that lack an
// @label description annotation. One violation per missing label: a
// *Vec metric with three undescribed labels produces three violations,
// each pinpointing the specific label via Location.LabelName.
//
// Scalar metrics (no labels) produce no violations — there is nothing
// to describe.
type MetricLabelDescriptionRequiredRule struct{}

// ID implements validation.Rule.
func (MetricLabelDescriptionRequiredRule) ID() string {
	return "metric.label-description-required"
}

// DefaultSeverity implements validation.Rule.
func (MetricLabelDescriptionRequiredRule) DefaultSeverity() validation.Severity {
	return validation.SeverityError
}

// Description implements validation.Rule.
func (MetricLabelDescriptionRequiredRule) Description() string {
	return "Every declared label must have an annotation-provided description"
}

// Validate iterates every label of every metric, emitting a violation
// for each label whose Description pointer is nil or points to an
// empty string.
func (r *MetricLabelDescriptionRequiredRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		for _, lbl := range m.Labels {
			if lbl.Description != nil && *lbl.Description != "" {
				continue
			}
			out = append(out, validation.Violation{
				RuleID:   r.ID(),
				Severity: validation.SeverityError,
				Message: fmt.Sprintf(
					"%s label %q has no description annotation (@label %s ...)",
					metricLabel(m), lbl.Name, lbl.Name,
				),
				Location: &validation.Location{MetricName: m.Name, LabelName: lbl.Name},
			})
		}
	}
	return out
}
