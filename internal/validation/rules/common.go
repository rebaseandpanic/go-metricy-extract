package rules

import (
	"fmt"
	"unicode/utf8"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// metricLabel returns a human-readable description of a metric for
// inclusion in violation messages. Returns `metric "foo"` when the metric
// has a name, and the bare word `metric` when the name is empty so the
// message does not render as `metric ""` with stray quotes around an
// empty string.
func metricLabel(m model.MetricDescriptor) string {
	if m.Name == "" {
		return "metric"
	}
	return fmt.Sprintf("metric %q", m.Name)
}

// resolveMinLength returns the effective minimum length for a rule,
// checking per-rule override first, then the context global, then the
// hardcoded default.
//
// Resolution order:
//  1. ctx.RuleMinLength[ruleID] — per-rule override always wins.
//  2. ctx.MinDescriptionLength — global default (if > 0).
//  3. hardcodedDefault — rule-specific fallback.
//
// Negative values from Context are clamped to 0 as defence-in-depth: the
// CLI parser already rejects negative values, but a direct caller of the
// validation package could still inject one and we would rather return a
// permissive 0 (no violations) than compare against a nonsensical
// negative bound. The same defence applies to the global — a negative
// MinDescriptionLength falls through to the hardcoded default rather
// than flipping the comparison into "everything passes".
func resolveMinLength(ctx validation.Context, ruleID string, hardcodedDefault int) int {
	if n, ok := ctx.RuleMinLength[ruleID]; ok {
		if n < 0 {
			return 0
		}
		return n
	}
	if ctx.MinDescriptionLength > 0 {
		return ctx.MinDescriptionLength
	}
	return hardcodedDefault
}

// checkStringMinLength evaluates a *string field against a minimum rune
// count and returns a violation if it's set but too short. Returns nil
// when:
//   - field is nil or empty (delegated to the corresponding
//     required-rule so length enforcement layers cleanly on top of
//     presence enforcement)
//   - rune count >= minLen (passes)
//
// Length is counted in Unicode code points (runes), not bytes, so a
// Cyrillic or CJK description is measured the way a human reader counts.
//
// The `kind` argument ("description" / "calculation") is embedded into
// the violation message for developer clarity — one helper serves both
// the description-min-length and calculation-min-length rules, and the
// message disambiguates which annotation the autofixer should look at.
func checkStringMinLength(m model.MetricDescriptor, kind string, field *string, ruleID string, minLen int) *validation.Violation {
	if field == nil || *field == "" {
		return nil
	}
	got := utf8.RuneCountInString(*field)
	if got >= minLen {
		return nil
	}
	return &validation.Violation{
		RuleID:   ruleID,
		Severity: validation.SeverityWarning,
		Message: fmt.Sprintf("%s %s is %d characters, minimum is %d",
			metricLabel(m), kind, got, minLen),
		Location: &validation.Location{MetricName: m.Name},
	}
}
