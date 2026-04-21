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
//
// Ordering convention:
//  1. error-severity rules (v0.1) — grouped by concern (presence,
//     cross-metric consistency).
//  2. warning-severity rules (v0.2 stage 1 — naming conventions +
//     extractor-diagnostic surfacing).
//  3. warning-severity rules (v0.2 stage 2 — min-length checks driven
//     by Context.MinDescriptionLength / RuleMinLength).
//  4. warning-severity rules (v0.2 stage 3 — off-by-default opt-in
//     diagnostics; enabled via --enable-rule).
//
// Appending new rules must preserve existing positions so golden
// fixtures and --list-rules output stay diff-friendly.
func All() []validation.Rule {
	return []validation.Rule{
		// v0.1 — error severity
		&MetricNameRequiredRule{},
		&MetricHelpRequiredRule{},
		&MetricDescriptionRequiredRule{},
		&MetricCalculationRequiredRule{},
		&MetricLabelDescriptionRequiredRule{},
		&MetricDuplicateNameRule{},
		&MetricTypeConsistencyRule{},
		// v0.2 stage 1 — warning severity, on by default
		&MetricCounterTotalSuffixRule{},
		&MetricHistogramUnitSuffixRule{},
		&MetricNameSnakeCaseRule{},
		&MetricNonLiteralMetadataRule{},
		// v0.2 stage 2 — min-length rules, warning severity, on by default
		&MetricDescriptionMinLengthRule{},
		&MetricCalculationMinLengthRule{},
		&MetricLabelDescriptionMinLengthRule{},
		// v0.2 stage 3 — warning severity, off by default
		&MetricLabelHighCardinalityHintRule{},
	}
}

// DefaultOffIDs returns the set of rule IDs that are disabled by default
// and require --enable-rule to activate.
func DefaultOffIDs() map[string]bool {
	return map[string]bool{
		"metric.label-high-cardinality-hint": true,
	}
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
	_ validation.Rule = (*MetricCounterTotalSuffixRule)(nil)
	_ validation.Rule = (*MetricHistogramUnitSuffixRule)(nil)
	_ validation.Rule = (*MetricNameSnakeCaseRule)(nil)
	_ validation.Rule = (*MetricNonLiteralMetadataRule)(nil)
	_ validation.Rule = (*MetricDescriptionMinLengthRule)(nil)
	_ validation.Rule = (*MetricCalculationMinLengthRule)(nil)
	_ validation.Rule = (*MetricLabelDescriptionMinLengthRule)(nil)
	_ validation.Rule = (*MetricLabelHighCardinalityHintRule)(nil)
)
