// Package annotations parses swag-style directives from Go doc comments.
//
// Directives:
//
//	@metric description <text>   — business-level description
//	@metric calculation <text>   — calculation algorithm
//	@label <name> <description>  — per-label description
//
// Unrecognized @-directives are ignored. One directive per line; multi-line
// directives are not supported in v0.1.
//
// Directive keywords are case-sensitive: @Metric, @METRIC, @Label are silently ignored.
package annotations

import (
	"fmt"
	"strings"
)

// Annotations carries metric-level and label-level descriptions parsed from
// a declaration's doc comment. A nil field signals "not present"; an empty
// string would mean "explicitly empty" which is not a distinction the parser
// produces (empty values are rejected with a warning).
type Annotations struct {
	Description *string
	Calculation *string
	// Labels maps label name → description. Map presence is the "parsed" signal;
	// empty-string values are rejected by the parser and should never appear here.
	Labels map[string]string
}

// AnnotationParser parses a doc comment into Annotations plus any warnings
// describing malformed or duplicated directives.
type AnnotationParser interface {
	Parse(doc string) (ann Annotations, warnings []string)
}

// SwagStyleParser implements AnnotationParser for @metric / @label directives.
type SwagStyleParser struct{}

// Parse implements AnnotationParser. It treats each line independently.
// Recognized directives:
//
//	@metric description <text>
//	@metric calculation <text>
//	@label <name> <description>
//
// Unknown @-directives (e.g. @api, @param) are ignored silently. Unknown
// @metric keywords, empty values, and duplicate directives produce warnings.
func (SwagStyleParser) Parse(doc string) (Annotations, []string) {
	var (
		ann      Annotations
		warnings []string
	)

	for _, rawLine := range strings.Split(doc, "\n") {
		// go/ast's CommentGroup.Text() strips "// " and block-comment delimiters;
		// the belt-and-braces TrimPrefix below handles hand-assembled raw lines
		// (single slash prefix only — triple-slash "///" is not normalized).
		line := strings.TrimSpace(rawLine)
		line = strings.TrimPrefix(line, "//")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "@") {
			continue
		}

		// Split on first whitespace token: directive keyword vs. remainder.
		directive, rest := splitFirstWord(line)

		switch directive {
		case "@metric":
			key, value := splitFirstWord(rest)
			switch key {
			case "description":
				setStringField(&ann.Description, "description", value, &warnings)
			case "calculation":
				setStringField(&ann.Calculation, "calculation", value, &warnings)
			case "":
				warnings = append(warnings, "empty @metric directive; expected '@metric description|calculation <text>'")
			default:
				warnings = append(warnings, fmt.Sprintf("unknown @metric key: %s", key))
			}

		case "@label":
			name, desc := splitFirstWord(rest)
			if name == "" || desc == "" {
				warnings = append(warnings, "invalid @label directive; expected '@label <name> <description>'")
				continue
			}
			if ann.Labels == nil {
				ann.Labels = map[string]string{}
			}
			if _, dup := ann.Labels[name]; dup {
				warnings = append(warnings, fmt.Sprintf("duplicate @label %q; overwriting", name))
			}
			ann.Labels[name] = desc

		default:
			// Other @-directives (e.g. @api, @param) are for other tools.
			// Skip silently.
		}
	}

	return ann, warnings
}

// setStringField applies an @metric string directive to dest. Emits a warning
// for empty values (ignored) or duplicates (previous value overwritten).
func setStringField(dest **string, key, value string, warnings *[]string) {
	if value == "" {
		*warnings = append(*warnings, fmt.Sprintf("empty @metric %s; ignoring", key))
		return
	}
	if *dest != nil {
		*warnings = append(*warnings, fmt.Sprintf("duplicate @metric %s; overwriting", key))
	}
	v := value
	*dest = &v
}

// splitFirstWord returns the first whitespace-delimited token of s and the
// trimmed remainder. If s has no whitespace, returns (s, ""). Accepts spaces
// and tabs as separators.
func splitFirstWord(s string) (first, rest string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	idx := strings.IndexAny(s, " \t")
	if idx < 0 {
		return s, ""
	}
	return s[:idx], strings.TrimSpace(s[idx+1:])
}
