// Package rules contains built-in validation rules for the MetricSnapshot.
//
// Each rule implements [validation.Rule] and is wired into the CLI through
// [All]. Keeping rules in a sibling package (rather than inside
// internal/validation) lets the engine stay agnostic of which concrete
// rules exist — the engine only knows the interface.
package rules

import "github.com/rebaseandpanic/go-metricy-extract/internal/validation"

// All returns the canonical list of built-in rules in stable order.
// The order is purely for display (e.g. --list-rules); the engine sorts
// violations independently.
func All() []validation.Rule {
	return []validation.Rule{
		&MetricNameRequiredRule{},
		&MetricHelpRequiredRule{},
		&MetricDescriptionRequiredRule{},
		&MetricCalculationRequiredRule{},
		&MetricLabelDescriptionRequiredRule{},
		&MetricDuplicateNameRule{},
		&MetricTypeConsistencyRule{},
	}
}

// DefaultOffIDs returns the set of rule IDs that are disabled by default
// and require --enable-rule to activate. For v0.1 all rules are on by
// default, so this returns an empty map.
func DefaultOffIDs() map[string]bool {
	return map[string]bool{}
}

// Compile-time guarantees: every rule satisfies validation.Rule. If a
// rule's signature drifts away from the interface, the build breaks at
// this file rather than at a test.
var (
	_ validation.Rule = (*MetricNameRequiredRule)(nil)
	_ validation.Rule = (*MetricHelpRequiredRule)(nil)
	_ validation.Rule = (*MetricDescriptionRequiredRule)(nil)
	_ validation.Rule = (*MetricCalculationRequiredRule)(nil)
	_ validation.Rule = (*MetricLabelDescriptionRequiredRule)(nil)
	_ validation.Rule = (*MetricDuplicateNameRule)(nil)
	_ validation.Rule = (*MetricTypeConsistencyRule)(nil)
)
