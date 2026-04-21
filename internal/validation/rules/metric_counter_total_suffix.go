package rules

import (
	"fmt"
	"strings"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricCounterTotalSuffixRule flags counter metrics whose names do not
// end with the conventional "_total" suffix. Prometheus naming best
// practice reserves "_total" for monotonically increasing counters so
// rate() / irate() queries are immediately recognizable; counters
// without the suffix trip up dashboards and alerting rules that key on
// the naming convention.
//
// Warning-severity by default: violations are informational — the
// snapshot is still structurally valid.
type MetricCounterTotalSuffixRule struct{}

// ID implements validation.Rule.
func (MetricCounterTotalSuffixRule) ID() string { return "metric.counter-total-suffix" }

// DefaultSeverity implements validation.Rule.
func (MetricCounterTotalSuffixRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricCounterTotalSuffixRule) Description() string {
	return "Counter metric names must end with _total"
}

// Validate emits one violation per counter metric whose Name does not
// end with "_total". Metrics with an empty Name are skipped — that's
// the responsibility of metric.name-required. Non-counter types are
// ignored entirely.
func (r *MetricCounterTotalSuffixRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Name == "" {
			continue
		}
		if m.Type != "counter" {
			continue
		}
		if strings.HasSuffix(m.Name, "_total") {
			continue
		}
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityWarning,
			Message:  fmt.Sprintf("counter metric %q should end with _total suffix", m.Name),
			Location: &validation.Location{MetricName: m.Name},
		})
	}
	return out
}
