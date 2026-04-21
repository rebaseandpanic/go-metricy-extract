package rules

import (
	"fmt"
	"strings"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// histogramUnitSuffixes is the set of allowed trailing units for
// histogram metric names, consistent with Prometheus "base unit" naming
// conventions (seconds, bytes, ratios, etc.). The check is a literal
// strings.HasSuffix match; callers who want a different unit can skip
// the rule via --skip-rule.
var histogramUnitSuffixes = []string{
	"_seconds", "_milliseconds", "_microseconds", "_nanoseconds",
	"_bytes", "_kilobytes", "_megabytes",
	"_ratio", "_percent", "_fraction",
	"_bits", "_celsius", "_meters",
}

// MetricHistogramUnitSuffixRule flags histogram metric names that do
// not end with any of the conventional unit suffixes (see
// histogramUnitSuffixes). Histograms measure distributions and the
// unit must be readable from the name alone; "request_duration"
// without "_seconds" leaves the unit ambiguous in Grafana tooltips
// and alerting rules.
//
// Warning-severity by default.
type MetricHistogramUnitSuffixRule struct{}

// ID implements validation.Rule.
func (MetricHistogramUnitSuffixRule) ID() string { return "metric.histogram-unit-suffix" }

// DefaultSeverity implements validation.Rule.
func (MetricHistogramUnitSuffixRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricHistogramUnitSuffixRule) Description() string {
	return fmt.Sprintf(
		"Histogram metric names must end with a unit suffix (%s)",
		strings.Join(histogramUnitSuffixes, ", "))
}

// Validate emits one violation per histogram metric whose Name does
// not end with any of the recognised unit suffixes. Metrics with an
// empty Name are skipped (delegated to metric.name-required).
// Non-histogram types are ignored entirely. The suffix check is
// case-sensitive: "_SECONDS" does not satisfy "_seconds".
func (r *MetricHistogramUnitSuffixRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Name == "" {
			continue
		}
		if m.Type != "histogram" {
			continue
		}
		if hasAnySuffix(m.Name, histogramUnitSuffixes) {
			continue
		}
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityWarning,
			Message: fmt.Sprintf(
				"histogram metric %q should end with a unit suffix (e.g. _seconds, _bytes, _ratio)",
				m.Name,
			),
			Location: &validation.Location{MetricName: m.Name},
		})
	}
	return out
}

// hasAnySuffix reports whether s ends with any element of suffixes.
// Used by the histogram-unit check.
func hasAnySuffix(s string, suffixes []string) bool {
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}
