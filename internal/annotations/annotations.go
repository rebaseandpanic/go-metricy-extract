// Package annotations parses swag-style directives from Go doc comments.
//
// Directives:
//
//	@metric description <text>   — business-level description
//	@metric calculation <text>   — calculation algorithm
//	@label <name> <description>  — per-label description
//
// Unrecognized @-directives are ignored. One directive per line; multi-line
// directives are not supported. A plain-text line immediately following an
// @metric/@label directive (without a blank line between) is treated as a
// likely continuation and produces a warning so the author notices that only
// the first line was captured.
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

	// lastDirectiveKind remembers the kind of the most recently processed
	// @metric/@label directive (e.g. "@metric description", "@label method").
	// It is cleared on blank lines (paragraph separator) and on non-directive
	// lines (after emitting the continuation warning, so a multi-line paragraph
	// produces a single warning, not one per line). Empty string means "the
	// previous line was not a recognized directive", so a plain text line does
	// not trigger a continuation warning.
	var lastDirectiveKind string

	for _, rawLine := range strings.Split(doc, "\n") {
		// go/ast's CommentGroup.Text() strips "// " and block-comment delimiters;
		// the belt-and-braces TrimLeft below handles hand-assembled raw lines with
		// any number of leading '/' characters (including triple-slash "///" used
		// by some autogenerators) so "///" normalizes to an empty line (paragraph
		// reset) rather than a stray "/" that would fire a continuation warning.
		line := strings.TrimSpace(rawLine)
		line = strings.TrimLeft(line, "/")
		line = strings.TrimSpace(line)
		if line == "" {
			// Blank line is a paragraph separator — reset directive tracking.
			lastDirectiveKind = ""
			continue
		}
		if !strings.HasPrefix(line, "@") {
			// Non-directive line. If it immediately follows a directive, warn
			// about a likely multi-line continuation (the second line of the
			// description/calculation/label is silently dropped today).
			if lastDirectiveKind != "" {
				warnings = append(warnings, fmt.Sprintf("possible multi-line continuation after %q directive; only the first line is captured", lastDirectiveKind))
				lastDirectiveKind = "" // suppress cascade on multi-line paragraphs
			}
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
				lastDirectiveKind = "@metric description"
			case "calculation":
				setStringField(&ann.Calculation, "calculation", value, &warnings)
				lastDirectiveKind = "@metric calculation"
			case "":
				warnings = append(warnings, "empty @metric directive; expected '@metric description|calculation <text>'")
				lastDirectiveKind = ""
			default:
				warnings = append(warnings, fmt.Sprintf("unknown @metric key: %s", key))
				lastDirectiveKind = ""
			}

		case "@label":
			name, desc := splitFirstWord(rest)
			if name == "" || desc == "" {
				warnings = append(warnings, "invalid @label directive; expected '@label <name> <description>'")
				lastDirectiveKind = ""
				continue
			}
			if ann.Labels == nil {
				ann.Labels = map[string]string{}
			}
			if _, dup := ann.Labels[name]; dup {
				warnings = append(warnings, fmt.Sprintf("duplicate @label %q; overwriting", name))
			}
			ann.Labels[name] = desc
			lastDirectiveKind = fmt.Sprintf("@label %s", name)

		default:
			// Other @-directives (e.g. @api, @param) are for other tools.
			// They are not ours, so a following plain-text line is not a
			// continuation of an @metric/@label — clear the tracker.
			lastDirectiveKind = ""
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
