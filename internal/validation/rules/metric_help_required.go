package rules

import (
	"fmt"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricHelpRequiredRule flags metrics missing the Help text that
// prometheus client libraries render into /metrics output. A metric
// without help is useless in Grafana tooltips and dashboards.
type MetricHelpRequiredRule struct{}

// ID implements validation.Rule.
func (MetricHelpRequiredRule) ID() string { return "metric.help-required" }

// DefaultSeverity implements validation.Rule.
func (MetricHelpRequiredRule) DefaultSeverity() validation.Severity {
	return validation.SeverityError
}

// Description implements validation.Rule.
func (MetricHelpRequiredRule) Description() string {
	return "Metric help text must be a non-empty string"
}

// Validate emits one violation per metric with an empty Help field.
// The message includes the metric name when available so the stderr
// summary is readable in multi-metric reports.
func (r *MetricHelpRequiredRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Help != "" {
			continue
		}
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityError,
			Message:  fmt.Sprintf("%s has no help text", metricLabel(m)),
			Location: &validation.Location{MetricName: m.Name},
		})
	}
	return out
}
