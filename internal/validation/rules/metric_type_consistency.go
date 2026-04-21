package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricTypeConsistencyRule flags metric names registered with two or
// more distinct types (e.g. a counter and a gauge sharing the name
// "requests_total"). Prometheus rejects such mixed registrations at
// runtime; surfacing it statically saves a deploy-time failure.
//
// Complementary to metric.duplicate-name — both rules may fire on the
// same name when the duplicates also disagree on type. That's
// intentional: the messages answer different questions ("the name
// repeats" vs "the types conflict").
type MetricTypeConsistencyRule struct{}

// ID implements validation.Rule.
func (MetricTypeConsistencyRule) ID() string { return "metric.type-consistency" }

// DefaultSeverity implements validation.Rule.
func (MetricTypeConsistencyRule) DefaultSeverity() validation.Severity {
	return validation.SeverityError
}

// Description implements validation.Rule.
func (MetricTypeConsistencyRule) Description() string {
	return "The same metric name must not be registered with two different types"
}

// Validate groups metrics by name and, for every group whose Type set
// has more than one distinct entry, emits one violation listing all
// observed types (alphabetically sorted) for a stable, diff-friendly
// message.
func (r *MetricTypeConsistencyRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}

	// name → set of types
	typesByName := make(map[string]map[string]struct{}, len(snapshot.Metrics))
	for _, m := range snapshot.Metrics {
		s, ok := typesByName[m.Name]
		if !ok {
			s = map[string]struct{}{}
			typesByName[m.Name] = s
		}
		s[m.Type] = struct{}{}
	}

	// Stable name order — see MetricDuplicateNameRule for rationale.
	names := make([]string, 0, len(typesByName))
	for name, ts := range typesByName {
		if len(ts) > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	out := make([]validation.Violation, 0, len(names))
	for _, name := range names {
		ts := typesByName[name]
		sorted := make([]string, 0, len(ts))
		for t := range ts {
			sorted = append(sorted, t)
		}
		sort.Strings(sorted)
		out = append(out, validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityError,
			Message: fmt.Sprintf(
				"metric name %q declared with conflicting types: %s",
				name, strings.Join(sorted, ", "),
			),
			Location: &validation.Location{MetricName: name},
		})
	}
	return out
}
