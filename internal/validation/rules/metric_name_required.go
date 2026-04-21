package rules

import (
	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricNameRequiredRule flags any metric whose Name is the empty string.
//
// An unnamed metric cannot be addressed in Prometheus queries or looked up
// by downstream consumers; it is almost always an extractor bug or a
// half-written metric declaration. Error-severity by default.
type MetricNameRequiredRule struct{}

// ID implements validation.Rule.
func (MetricNameRequiredRule) ID() string { return "metric.name-required" }

// DefaultSeverity implements validation.Rule.
func (MetricNameRequiredRule) DefaultSeverity() validation.Severity {
	return validation.SeverityError
}

// Description implements validation.Rule.
func (MetricNameRequiredRule) Description() string {
	return "Metric name must be a non-empty string"
}

// Validate scans snapshot.Metrics and emits one violation per entry with
// an empty Name. Location.MetricName is set to the empty string: the
// engine's enrichment pass keys on metric name, so file/line are not
// filled in for unnamed metrics — that's fine here, the primary signal
// is the missing name itself.
func (r *MetricNameRequiredRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Name == "" {
			out = append(out, validation.Violation{
				RuleID:   r.ID(),
				Severity: validation.SeverityError,
				Message:  "metric has no name",
				Location: &validation.Location{MetricName: m.Name},
			})
		}
	}
	return out
}
