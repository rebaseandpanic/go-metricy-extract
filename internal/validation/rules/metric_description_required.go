package rules

import (
	"fmt"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricDescriptionRequiredRule flags metrics whose @metric description
// annotation is missing or empty. The description is the authoritative
// business-level prose for downstream consumers and is the reason the
// opt-in annotation exists — a metric without one should not be exported.
type MetricDescriptionRequiredRule struct{}

// ID implements validation.Rule.
func (MetricDescriptionRequiredRule) ID() string { return "metric.description-required" }

// DefaultSeverity implements validation.Rule.
func (MetricDescriptionRequiredRule) DefaultSeverity() validation.Severity {
	return validation.SeverityError
}

// Description implements validation.Rule.
func (MetricDescriptionRequiredRule) Description() string {
	return "Annotation description must be set"
}

// Validate flags metrics whose Description is nil (annotation absent) or
// an empty pointer (annotation present but blank). Both cases mean
// downstream consumers would see no description — semantically equivalent,
// so we collapse them into one violation per metric.
func (r *MetricDescriptionRequiredRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Description != nil && *m.Description != "" {
			continue
		}
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityError,
			Message:  fmt.Sprintf("%s has no description annotation (@metric description)", metricLabel(m)),
			Location: &validation.Location{MetricName: m.Name},
		})
	}
	return out
}
