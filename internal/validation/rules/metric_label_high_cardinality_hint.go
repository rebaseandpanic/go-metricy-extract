package rules

import (
	"fmt"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// defaultHighCardinalityLabels lists label names that are well-known
// cardinality killers in real-world Prometheus deployments. The list
// intentionally includes short/long aliases (ip + ip_address, session +
// session_id, email + email_address, hostname + host) because teams
// pick one arbitrarily — we flag whichever shows up in source.
//
// Matches are exact and case-sensitive — a label named "user_id" is
// flagged, "user_id_v2" or "UserID" is not. Users override the whole
// list via --high-cardinality-labels (see CLI wiring in main.go);
// passing a non-nil empty slice turns the rule into a no-op.
//
// Stored as a set for O(1) lookup. The map is read-only — Validate
// must never mutate it, since when Context.HighCardinalityLabels is
// nil we hand this map out directly (no defensive copy).
var defaultHighCardinalityLabels = map[string]struct{}{
	"user_id": {}, "userid": {},
	"email": {}, "email_address": {},
	"ip": {}, "ip_address": {}, "client_ip": {}, "remote_ip": {},
	"uuid": {}, "guid": {},
	"session_id": {}, "session": {},
	"request_id": {}, "trace_id": {}, "span_id": {},
	"path": {}, "url": {}, "uri": {}, "request_path": {},
	"query": {}, "query_string": {},
	"hostname": {}, "host": {},
}

// MetricLabelHighCardinalityHintRule flags labels whose names match a
// known high-cardinality pattern (user_id, email, ip, uuid, session_id,
// etc.). Default-off — activate via --enable-rule
// metric.label-high-cardinality-hint. Matching is exact and
// case-sensitive: "user_id" triggers, "user_id_v2" and "UserID" do not.
//
// Scalar metrics (no labels) and metrics with empty Name produce no
// violations. The pattern set is supplied via
// Context.HighCardinalityLabels; a nil value means "use the built-in
// default list", an explicit non-nil empty slice means "no patterns"
// and the rule becomes a no-op.
//
// Warning-severity. Off by default (see DefaultOffIDs).
type MetricLabelHighCardinalityHintRule struct{}

// ID implements validation.Rule.
func (MetricLabelHighCardinalityHintRule) ID() string {
	return "metric.label-high-cardinality-hint"
}

// DefaultSeverity implements validation.Rule.
func (MetricLabelHighCardinalityHintRule) DefaultSeverity() validation.Severity {
	return validation.SeverityWarning
}

// Description implements validation.Rule.
func (MetricLabelHighCardinalityHintRule) Description() string {
	return "Label name matches a known high-cardinality pattern (user_id, email, ip, etc.)"
}

// Validate iterates every label of every metric, emitting a violation
// when the label name exactly matches one of the configured
// high-cardinality patterns. One violation per matching label.
func (r *MetricLabelHighCardinalityHintRule) Validate(snapshot *model.MetricSnapshot, ctx validation.Context) []validation.Violation {
	if snapshot == nil {
		return nil
	}

	// nil → use the hardcoded default map directly (no copy, read-only).
	// Non-nil (even empty) → build a fresh map from the caller's slice.
	// This lets users disable the rule's pattern set (by passing an
	// empty non-nil slice) without disabling the rule itself.
	//
	// Build a set for O(1) label lookup. The default list is ~20 entries; at
	// that size, map build cost is negligible compared to label scanning.
	var patterns map[string]struct{}
	if ctx.HighCardinalityLabels == nil {
		patterns = defaultHighCardinalityLabels
	} else {
		patterns = make(map[string]struct{}, len(ctx.HighCardinalityLabels))
		for _, p := range ctx.HighCardinalityLabels {
			patterns[p] = struct{}{}
		}
	}
	if len(patterns) == 0 {
		return nil
	}

	var out []validation.Violation
	for _, m := range snapshot.Metrics {
		if m.Name == "" {
			// Unnamed metrics can't be reported usefully — they would
			// also be flagged by metric.name-required.
			continue
		}
		for _, lbl := range m.Labels {
			if _, ok := patterns[lbl.Name]; !ok {
				continue
			}
			out = append(out, validation.Violation{
				RuleID:   r.ID(),
				Severity: validation.SeverityWarning,
				Message: fmt.Sprintf(
					"%s label %q matches a known high-cardinality pattern — this can explode Prometheus memory usage; consider aggregating or removing",
					metricLabel(m), lbl.Name,
				),
				Location: &validation.Location{MetricName: m.Name, LabelName: lbl.Name},
			})
		}
	}
	return out
}
