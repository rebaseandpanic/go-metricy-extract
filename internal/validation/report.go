package validation

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
)

// ReportSchemaVersion identifies the wire format of the validation report
// document. Bump the major component on breaking changes to the report
// shape; minor bumps cover additive, backward-compatible changes (new
// fields that consumers may safely ignore).
//
// 1.1 — added generated_at (ISO-8601 UTC) and by_rule (per-rule counts)
// alongside the existing envelope. Pre-1.1 consumers that ignore unknown
// keys read these documents correctly.
const ReportSchemaVersion = "1.1"

// Report is the machine-readable output written by --validation-report.
// Its JSON shape is the contract for downstream tooling (agents, CI
// dashboards) and is versioned via SchemaVersion.
type Report struct {
	SchemaVersion string      `json:"schema_version"`
	GeneratedAt   string      `json:"generated_at"`
	Violations    []Violation `json:"violations"`
	ErrorCount    int         `json:"error_count"`
	WarningCount  int         `json:"warning_count"`
	ByRule        []RuleCount `json:"by_rule"`
}

// RuleCount aggregates the violations emitted by a single rule, split by
// effective severity. Rules that did not fire at all are omitted from the
// parent Report.ByRule slice — zero-rows would only bloat the report for
// consumers counting the "rules that had issues" cardinality.
type RuleCount struct {
	// RuleID is the stable dotted identifier (same value that appears on
	// each Violation.RuleID emitted by the rule).
	RuleID string `json:"rule_id"`
	// Severity is the effective severity label for this rule group. In the
	// common case the engine re-stamps every violation of a given RuleID
	// with the same effective severity, so one of ErrorCount /
	// WarningCount is zero and Severity matches the non-zero side. If a
	// hand-constructed Result routes mixed-severity violations through a
	// single RuleID (possible when bypassing the engine), WriteReport
	// promotes Severity to "error" so the label reflects the worst
	// observed violation. An "unknown" label appears only when an
	// out-of-range Severity value reaches the report.
	Severity string `json:"severity"`
	// ErrorCount is the number of violations in this group at SeverityError.
	ErrorCount int `json:"error_count"`
	// WarningCount is the number of violations in this group at SeverityWarning.
	WarningCount int `json:"warning_count"`
}

// WriteReport writes res as indented JSON to w. Violations is normalised to
// an empty slice (never null), counts are computed from the effective
// severity on each violation. by_rule is sorted by RuleID.
//
// now is a clock override used to stamp generated_at; passing nil falls
// back to time.Now. Golden tests inject a fixed clock so the on-disk
// fixture stays byte-stable across runs.
func WriteReport(w io.Writer, res *Result, now func() time.Time) error {
	vios := []Violation{}
	var errs, warns int
	byRuleMap := map[string]*RuleCount{}
	if res != nil {
		if res.Violations != nil {
			vios = res.Violations
		}
		for _, v := range res.Violations {
			sevStr := severityString(v.Severity)
			rc, ok := byRuleMap[v.RuleID]
			if !ok {
				rc = &RuleCount{RuleID: v.RuleID, Severity: sevStr}
				byRuleMap[v.RuleID] = rc
			}
			// Unknown severities (out-of-range Severity values) are
			// intentionally not counted into errs/warns or the per-rule
			// ErrorCount/WarningCount buckets: the totals must equal the
			// sum of the two buckets for any single-severity view of the
			// report, and silently bucketing unknown values as warnings
			// would make that invariant misleading. The violation still
			// appears in Violations and by_rule carries Severity="unknown"
			// for the group, so the anomaly remains visible.
			switch v.Severity {
			case SeverityError:
				errs++
				rc.ErrorCount++
			case SeverityWarning:
				warns++
				rc.WarningCount++
			}
		}
	}

	// Defensive promotion: if the engine ever routes mixed-severity
	// violations through a single RuleID (currently impossible — engine
	// re-stamps after override resolution), pick the strongest observed
	// severity. "error" wins over "warning" so the label reflects the
	// worst violation in the group.
	for _, rc := range byRuleMap {
		if rc.ErrorCount > 0 && rc.WarningCount > 0 {
			rc.Severity = "error"
		}
	}

	// Deterministic order for golden files and stable diffs: sort by RuleID
	// (ordinal string compare, matches the Violations sort key).
	byRule := make([]RuleCount, 0, len(byRuleMap))
	for _, rc := range byRuleMap {
		byRule = append(byRule, *rc)
	}
	sort.Slice(byRule, func(i, j int) bool { return byRule[i].RuleID < byRule[j].RuleID })

	clock := now
	if clock == nil {
		clock = time.Now
	}
	// Second-precision UTC matches MetricSnapshot.ExtractedAt so timestamps
	// across the two documents are comparable without format massaging.
	generatedAt := clock().UTC().Format(model.ExtractedAtLayout)

	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		GeneratedAt:   generatedAt,
		Violations:    vios,
		ErrorCount:    errs,
		WarningCount:  warns,
		ByRule:        byRule,
	}

	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

// severityString maps a typed Severity to its canonical wire string.
// Out-of-range Severity values surface as "unknown" so reports never
// silently bucket an anomalous value under "warning". WriteReport's
// counting switch pairs with this: unknown-severity violations land in
// by_rule with Severity="unknown" but do not increment
// ErrorCount/WarningCount — that keeps the (errors, warnings) totals
// equal to the sum of the two per-rule buckets.
func severityString(s Severity) string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return "unknown"
	}
}

// FormatStderrSummary returns a short human-readable summary of violations
// — one line per violation — suitable for writing to stderr when
// --validation-report is not set. Returns an empty string when res has no
// violations so callers can cheaply skip the write.
//
// Line format:
//
//	[<severity>] <rule_id>: <message> at <metric>:<label>  (<file>:<line>)
//
// Empty Metric/Label/File/Line fields are elided so the output stays tidy
// for rules that fire at the document level rather than on a specific
// metric.
func FormatStderrSummary(res *Result) string {
	if res == nil || len(res.Violations) == 0 {
		return ""
	}
	var b strings.Builder
	var errs, warns int
	for _, v := range res.Violations {
		line := formatViolationLine(v)
		b.WriteString(line)
		b.WriteByte('\n')
		switch v.Severity {
		case SeverityError:
			errs++
		case SeverityWarning:
			warns++
		}
	}
	// Short footer so the user sees the totals without having to count.
	fmt.Fprintf(&b, "validation: %d error(s), %d warning(s)\n", errs, warns)
	return b.String()
}

// formatViolationLine renders a single violation in the stderr-summary
// format. Extracted for unit-testability.
func formatViolationLine(v Violation) string {
	target := ""
	file := ""
	if v.Location != nil {
		switch {
		case v.Location.MetricName != "" && v.Location.LabelName != "":
			target = v.Location.MetricName + ":" + v.Location.LabelName
		case v.Location.MetricName != "":
			target = v.Location.MetricName
		case v.Location.LabelName != "":
			target = ":" + v.Location.LabelName
		}
		if v.Location.File != "" {
			if v.Location.Line != nil {
				file = fmt.Sprintf(" (%s:%d)", v.Location.File, *v.Location.Line)
			} else {
				file = fmt.Sprintf(" (%s)", v.Location.File)
			}
		}
	}

	base := fmt.Sprintf("[%s] %s: %s", v.Severity.String(), v.RuleID, v.Message)
	if target != "" {
		base += " at " + target
	}
	base += file
	return base
}
