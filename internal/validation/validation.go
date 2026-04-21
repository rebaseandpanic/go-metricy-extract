// Package validation runs pluggable rule checks against a MetricSnapshot
// and aggregates violations into a machine-readable report.
//
// Rules implement the [Rule] interface and are executed by [Run] against a
// snapshot. Each rule returns a list of [Violation]s; the engine enriches
// each violation with source-location info (file, line, class, member) when
// the violation references a metric by name, applies severity overrides
// (per-rule warn/error overrides plus --strict), and produces a
// deterministic, sorted [Result] suitable for golden-file consumption.
//
// Rules themselves live in a sibling package / registry and are wired in by
// the CLI — this package only provides types, the engine, and the report
// writer.
package validation

import (
	"encoding/json"
	"fmt"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
)

// Severity categorises a rule's violations. The numeric ordering is
// meaningful: Error > Warning, so simple comparisons work for "any errors?"
// checks.
type Severity int

const (
	// SeverityWarning is a non-blocking diagnostic: visible to users and
	// reports, but does not fail CI by itself.
	SeverityWarning Severity = iota
	// SeverityError is a blocking diagnostic: a single error-severity
	// violation causes [Run] callers to exit with a non-zero code.
	SeverityError
)

// String returns the canonical JSON/CLI rendering of s.
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// Location carries the per-violation pointer into the snapshot. All fields
// are best-effort; File/Line/ClassName/MemberName may be empty when
// source-location resolution was unavailable or the violation is not tied
// to a specific metric.
type Location struct {
	MetricName string `json:"metric_name,omitempty"`
	LabelName  string `json:"label_name,omitempty"`
	File       string `json:"file,omitempty"`
	Line       *int   `json:"line,omitempty"`
	ClassName  string `json:"class_name,omitempty"`
	MemberName string `json:"member_name,omitempty"`
}

// Violation is a single rule finding. Severity is re-stamped by the engine
// after rule execution (rules return their "natural" severity; the engine
// applies overrides), so rules should not rely on this field being
// preserved verbatim.
type Violation struct {
	RuleID   string    `json:"rule_id"`
	Severity Severity  `json:"-"` // serialized via MarshalJSON below
	Message  string    `json:"message"`
	Location *Location `json:"location,omitempty"`
}

// MarshalJSON emits severity as a string ("error" / "warning") while keeping
// the Go field as a typed int for cheap comparisons. The field order in the
// output mirrors the struct definition so golden files stay stable.
func (v Violation) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RuleID   string    `json:"rule_id"`
		Severity string    `json:"severity"`
		Message  string    `json:"message"`
		Location *Location `json:"location,omitempty"`
	}{
		RuleID:   v.RuleID,
		Severity: v.Severity.String(),
		Message:  v.Message,
		Location: v.Location,
	})
}

// UnmarshalJSON parses the wire shape produced by MarshalJSON back into a
// typed Violation. An unrecognised severity string returns an error rather
// than silently defaulting to zero — tooling that round-trips reports
// through this type must see a parse failure, not a semantic flip.
func (v *Violation) UnmarshalJSON(data []byte) error {
	aux := struct {
		RuleID   string    `json:"rule_id"`
		Severity string    `json:"severity"`
		Message  string    `json:"message"`
		Location *Location `json:"location,omitempty"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	v.RuleID = aux.RuleID
	v.Message = aux.Message
	v.Location = aux.Location
	switch aux.Severity {
	case "error":
		v.Severity = SeverityError
	case "warning":
		v.Severity = SeverityWarning
	default:
		return fmt.Errorf("unknown severity %q", aux.Severity)
	}
	return nil
}

// Rule is the plug-in point for completeness/correctness checks. Rules must
// be pure functions of (snapshot, ctx): no I/O, no globals, no mutation of
// the snapshot. The engine is free to call Validate concurrently across
// rules, though the current implementation is sequential.
type Rule interface {
	// ID is the stable, dotted identifier used by --skip-rule / --warn-rule /
	// --error-rule / --enable-rule. Must be unique within the registry.
	ID() string
	// DefaultSeverity is the severity emitted when no CLI override applies.
	DefaultSeverity() Severity
	// Description is shown in help output and the violation report header.
	Description() string
	// Validate inspects snapshot and returns a list of violations. Rules
	// should populate Location.MetricName / LabelName when the violation is
	// tied to a specific metric — the engine uses these to enrich File/Line
	// from the snapshot's source_location.
	Validate(snapshot *model.MetricSnapshot, ctx Context) []Violation
}

// Context passes CLI-level knobs into rules. Unknown fields in future
// versions must default to zero-values that preserve existing rule
// behaviour.
type Context struct {
	// MinDescriptionLength is the global default for description-length
	// rules. Zero is treated as "unset"; rules are expected to fall back to
	// their own per-rule default (typically 20) when they see 0 here.
	MinDescriptionLength int
	// RuleMinLength overrides MinDescriptionLength per rule ID. A value set
	// here always wins over the global default.
	RuleMinLength map[string]int
}
