package rules

import (
	"fmt"
	"sort"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricDuplicateNameRule flags metric names that appear more than once
// in the snapshot. Prometheus requires a single canonical registration
// per metric name — duplicates typically indicate either a copy-paste
// error or two different packages registering against the default
// registry with the same name.
//
// Emits one violation per duplicated name (not per duplicate site) so
// a name that appears N times produces 1 violation, not N-1.
type MetricDuplicateNameRule struct{}

// ID implements validation.Rule.
func (MetricDuplicateNameRule) ID() string { return "metric.duplicate-name" }

// DefaultSeverity implements validation.Rule.
func (MetricDuplicateNameRule) DefaultSeverity() validation.Severity {
	return validation.SeverityError
}

// Description implements validation.Rule.
func (MetricDuplicateNameRule) Description() string {
	return "Same metric name must not appear more than once in the snapshot"
}

// Validate counts occurrences of each metric name and emits one
// violation for every name with count >= 2. Names are sorted
// alphabetically in the output so golden-file tests stay stable even
// before the engine's final sort pass runs.
func (r *MetricDuplicateNameRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	counts := make(map[string]int, len(snapshot.Metrics))
	for _, m := range snapshot.Metrics {
		counts[m.Name]++
	}

	// Stable name ordering. Not strictly required (engine re-sorts), but
	// makes debugging output readable without a trip through Run.
	names := make([]string, 0, len(counts))
	for name, c := range counts {
		if c >= 2 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	out := make([]validation.Violation, 0, len(names))
	for _, name := range names {
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityError,
			Message:  fmt.Sprintf("metric name %q appears %d times", name, counts[name]),
			Location: &validation.Location{MetricName: name},
		})
	}
	return out
}
