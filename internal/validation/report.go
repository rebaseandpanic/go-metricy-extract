package validation

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ReportSchemaVersion identifies the wire format of the validation report
// document. Bump the major component on breaking changes to the report
// shape.
const ReportSchemaVersion = "1.0"

// Report is the machine-readable output written by --validation-report.
// Its JSON shape is the contract for downstream tooling (agents, CI
// dashboards) and is versioned via SchemaVersion.
type Report struct {
	SchemaVersion string      `json:"schema_version"`
	Violations    []Violation `json:"violations"`
	ErrorCount    int         `json:"error_count"`
	WarningCount  int         `json:"warning_count"`
}

// WriteReport writes res as indented JSON to w. Violations is normalised to
// an empty slice (never null), counts are computed from the effective
// severity on each violation.
func WriteReport(w io.Writer, res *Result) error {
	vios := []Violation{}
	var errs, warns int
	if res != nil {
		if res.Violations != nil {
			vios = res.Violations
		}
		for _, v := range res.Violations {
			switch v.Severity {
			case SeverityError:
				errs++
			case SeverityWarning:
				warns++
			}
		}
	}

	rep := Report{
		SchemaVersion: ReportSchemaVersion,
		Violations:    vios,
		ErrorCount:    errs,
		WarningCount:  warns,
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
