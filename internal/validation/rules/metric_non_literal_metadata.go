package rules

import (
	"strings"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// MetricNonLiteralMetadataRule surfaces the two extractor warning families
// that cause a metric to be silently dropped due to non-literal Name or Help:
//
//   - "non-literal Name; skipping metric"
//   - "non-literal Help; skipping metric"
//
// Other silent-skip paths ("non-literal options argument", "invalid string
// literal") are out of scope — this rule catches runtime-computed metric
// identifiers specifically, not every skip.
//
// Warning-severity, on-by-default. Pairs with the extractor warnings that
// are already visible on stderr; promoting them to validation errors gives
// CI a single contract-check failure surface.
type MetricNonLiteralMetadataRule struct{}

// nonLiteralNeedles are the substrings this rule recognises inside
// warning strings. Kept in one place so future extractor phrasing can
// be added without touching the Validate loop.
var nonLiteralNeedles = []string{"non-literal Name", "non-literal Help"}

// ID implements validation.Rule.
func (MetricNonLiteralMetadataRule) ID() string { return "metric.non-literal-metadata" }

// DefaultSeverity implements validation.Rule.
func (MetricNonLiteralMetadataRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricNonLiteralMetadataRule) Description() string {
	return "Metric name or help is computed at runtime and could not be statically resolved"
}

// Validate walks snapshot.ExtractionWarnings for entries that report a
// non-literal Name or Help and emits one violation per entry. The
// violation message is the raw warning string (already prefixed with
// the var name by the extractor). When the warning is of the
// conventional "<varName>: ..." form the varName is forwarded into
// Location.MetricName as a best-guess handle for downstream tooling;
// otherwise Location is left nil.
func (r *MetricNonLiteralMetadataRule) Validate(snapshot *model.MetricSnapshot, _ validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}
	if len(snapshot.ExtractionWarnings) == 0 {
		return nil
	}
	var out []validation.Violation
	for _, w := range snapshot.ExtractionWarnings {
		if !containsAny(w, nonLiteralNeedles) {
			continue
		}
		v := validation.Violation{
			RuleID:   r.ID(),
			Severity: validation.SeverityWarning,
			Message:  w,
		}
		if varName := extractVarName(w); varName != "" {
			v.Location = &validation.Location{MetricName: varName}
		}
		out = append(out, v)
	}
	return out
}

// containsAny reports whether s contains any substring from needles.
func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

// extractVarName парсит префикс "VarName: " из extractor warning.
// ASCII-only by design: Go-identifier в реальных кодовых базах всегда ASCII;
// Unicode-identifier'ы теоретически разрешены Go-грамматикой, но на практике
// не встречаются в prometheus-annotated var'ах. Если попадётся — Location
// просто остаётся nil, не ломается extraction.
//
// A lenient contract is on purpose: misattributing a location to a
// varName that is not in snapshot.Metrics is harmless — the engine's
// enrichLocation silently no-ops when the name is unknown.
func extractVarName(warning string) string {
	idx := strings.Index(warning, ": ")
	if idx <= 0 {
		return ""
	}
	candidate := warning[:idx]
	if !isGoIdentifier(candidate) {
		return ""
	}
	return candidate
}

// isGoIdentifier reports whether s looks like a Go identifier (letter
// or underscore, then letters / digits / underscores). Keeps the
// prefix parser from grabbing sentence fragments that happen to
// contain ": ".
func isGoIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, ch := range s {
		switch {
		case ch == '_':
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case i > 0 && ch >= '0' && ch <= '9':
		default:
			return false
		}
	}
	return true
}
