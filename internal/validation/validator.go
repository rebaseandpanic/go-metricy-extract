package validation

import (
	"sort"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
)

// Options controls rule execution for a single Run call.
//
// Skip, Enable, and SeverityOverride are all keyed by rule ID; unknown IDs
// are ignored here (the CLI layer is responsible for warning the user about
// typos before calling Run).
type Options struct {
	// Rules is the full set of rules to consider. Rules that are not in
	// Enable but carry a "default-off" marker must be filtered out by the
	// caller — this engine treats every element of Rules as enabled unless
	// Skip says otherwise.
	Rules []Rule
	// Skip is the set of rule IDs to exclude entirely. Skipped rules do not
	// execute and appear in Result.SkippedRules.
	Skip map[string]bool
	// Enable is the set of rule IDs to force-enable. A rule whose ID is in
	// Options.DefaultOff runs only when Enable[id] == true; other rules are
	// unaffected by this set. Callers may also inspect Enable independently
	// of the engine's filtering decision.
	Enable map[string]bool
	// DefaultOff is the set of rule IDs that are disabled by default.
	// A rule whose ID is in DefaultOff runs only if Enable[id] == true.
	// Typical caller: CLI populates DefaultOff from the rule registry
	// (e.g. validation/rules.DefaultOffIDs) and Enable from --enable-rule flags.
	DefaultOff map[string]bool
	// SeverityOverride maps rule ID → effective severity. An explicit entry
	// here wins over both the rule's DefaultSeverity and the Strict flag.
	// Explicit entries here win over Strict. BuildOverrides constructs this map to implement that precedence.
	SeverityOverride map[string]Severity
	// Strict, when true, promotes every warning-default rule to error
	// severity. Explicit SeverityOverride entries still win.
	Strict bool
	// Context is forwarded verbatim to each rule's Validate call.
	Context Context
}

// Result aggregates the output of a Run. Violations are sorted
// deterministically (see Run) so callers can diff reports across runs.
type Result struct {
	// Violations is the flat, sorted list of findings across all rules.
	Violations []Violation
	// SkippedRules is the subset of rule IDs from Options.Rules that were
	// excluded via Options.Skip. Sorted alphabetically.
	SkippedRules []string
	// UnknownIDs is reserved for future engine-level unknown-ID detection.
	// The CLI currently handles unknown-ID warnings itself; this field is
	// kept as part of the public shape so existing consumers do not break
	// when engine-side detection is added.
	UnknownIDs []string
}

// Run executes all registered rules against snapshot and returns an
// aggregated Result.
//
// Behaviour:
//   - Rules in Options.Skip are not invoked; their IDs land in
//     Result.SkippedRules.
//   - Each violation's Severity is re-stamped to the effective severity:
//     Options.SeverityOverride (if present) > Strict-promotion (if the
//     rule's default is Warning) > the rule's DefaultSeverity.
//   - When a violation carries a Location.MetricName, the engine enriches
//     File/Line/ClassName/MemberName from the snapshot's SourceLocation for
//     that metric. Violations without MetricName are left as the rule
//     produced them.
//   - The final violation list is sorted by (RuleID, MetricName, LabelName,
//     Message) so golden-file tests are stable.
//
// A nil snapshot returns an empty Result (no rules executed, no panics) —
// rules do not need to guard against nil themselves.
func Run(snapshot *model.MetricSnapshot, opts Options) *Result {
	if snapshot == nil {
		return &Result{}
	}

	res := &Result{}

	// Build a lookup for source-location enrichment once up-front; rules
	// that return many violations against the same metric thus pay O(1)
	// per enrich rather than O(N) over snapshot.Metrics each time.
	locs := buildLocationIndex(snapshot)

	for _, rule := range opts.Rules {
		id := rule.ID()
		if opts.Skip[id] {
			res.SkippedRules = append(res.SkippedRules, id)
			continue
		}
		// Off-by-default rules are dropped silently (not listed in
		// SkippedRules) unless the caller explicitly enabled them.
		if opts.DefaultOff[id] && !opts.Enable[id] {
			continue
		}

		effective := resolveSeverity(rule, opts)

		vios := rule.Validate(snapshot, opts.Context)
		for i := range vios {
			vios[i].Severity = effective
			enrichLocation(&vios[i], locs)
		}
		res.Violations = append(res.Violations, vios...)
	}

	// Sorted skipped-rule list — alphabetical, ordinal comparison.
	sort.Strings(res.SkippedRules)

	// Deterministic violation order. Sorting by the full tuple avoids
	// ambiguity when two rules fire on the same metric/label with different
	// messages.
	sort.SliceStable(res.Violations, func(i, j int) bool {
		a, b := res.Violations[i], res.Violations[j]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		ai, bi := locField(a.Location, func(l *Location) string { return l.MetricName }),
			locField(b.Location, func(l *Location) string { return l.MetricName })
		if ai != bi {
			return ai < bi
		}
		al, bl := locField(a.Location, func(l *Location) string { return l.LabelName }),
			locField(b.Location, func(l *Location) string { return l.LabelName })
		if al != bl {
			return al < bl
		}
		return a.Message < b.Message
	})

	return res
}

// resolveSeverity applies the precedence rules described on Run.
// Separated out for readability and unit-testability.
func resolveSeverity(rule Rule, opts Options) Severity {
	if sev, ok := opts.SeverityOverride[rule.ID()]; ok {
		return sev
	}
	def := rule.DefaultSeverity()
	if opts.Strict && def == SeverityWarning {
		return SeverityError
	}
	return def
}

// buildLocationIndex maps metric name → pointer to the snapshot's
// SourceLocation for that metric. Returns nil when snapshot is nil, saving
// callers a nil-check.
func buildLocationIndex(snapshot *model.MetricSnapshot) map[string]*model.SourceLocation {
	if snapshot == nil {
		return nil
	}
	out := make(map[string]*model.SourceLocation, len(snapshot.Metrics))
	for i := range snapshot.Metrics {
		out[snapshot.Metrics[i].Name] = snapshot.Metrics[i].SourceLocation
	}
	return out
}

// enrichLocation copies File/Line/Class/Member from the snapshot into
// v.Location when v.Location.MetricName is set and exists in the index.
// Fields already set on the violation win — rules are allowed to point at a
// specific site inside the metric declaration (e.g. a label line) and the
// engine must not clobber that.
func enrichLocation(v *Violation, locs map[string]*model.SourceLocation) {
	if v.Location == nil || v.Location.MetricName == "" {
		return
	}
	src, ok := locs[v.Location.MetricName]
	if !ok || src == nil {
		return
	}
	if v.Location.File == "" {
		v.Location.File = src.File
	}
	if v.Location.Line == nil && src.Line != nil {
		// Copy through an intermediate so later mutations to src don't
		// leak through the pointer alias.
		n := *src.Line
		v.Location.Line = &n
	}
	if v.Location.ClassName == "" && src.Class != nil {
		v.Location.ClassName = *src.Class
	}
	if v.Location.MemberName == "" && src.Member != nil {
		v.Location.MemberName = *src.Member
	}
}

// locField returns get(l) when l != nil and "" otherwise. Tiny helper that
// keeps the sort closure readable.
func locField(l *Location, get func(*Location) string) string {
	if l == nil {
		return ""
	}
	return get(l)
}

// BuildOverrides merges --warn-rule / --error-rule flags and Strict into a
// severity map keyed by rule ID.
//
// Precedence (highest first):
//  1. explicit --error-rule  (wins over --warn-rule on conflict)
//  2. explicit --warn-rule
//  3. --strict promotion of warning-default rules to error
//
// The returned conflicts slice contains every rule ID that appeared in both
// warnRules and errorRules — callers can surface these to the user. The
// returned map already reflects "error wins" for those IDs.
//
// Rule IDs that are not present in rules are silently ignored: the CLI
// layer is expected to warn about unknown IDs before calling this function.
func BuildOverrides(rules []Rule, strict bool, warnRules, errorRules []string) (map[string]Severity, []string) {
	known := make(map[string]Rule, len(rules))
	for _, r := range rules {
		known[r.ID()] = r
	}

	out := map[string]Severity{}

	// 1. Strict: promote warning-default rules to error, subject to later
	//    explicit overrides below.
	if strict {
		for _, r := range rules {
			if r.DefaultSeverity() == SeverityWarning {
				out[r.ID()] = SeverityError
			}
		}
	}

	// 2. warn-rule: demote (or pin) to warning. Applied before error-rule so
	//    a subsequent error-rule entry for the same ID wins cleanly.
	warnSet := map[string]bool{}
	for _, id := range warnRules {
		if _, ok := known[id]; !ok {
			continue
		}
		warnSet[id] = true
		out[id] = SeverityWarning
	}

	// 3. error-rule: always error. Conflicts with warn-rule are reported.
	var conflicts []string
	for _, id := range errorRules {
		if _, ok := known[id]; !ok {
			continue
		}
		if warnSet[id] {
			conflicts = append(conflicts, id)
		}
		out[id] = SeverityError
	}

	// Deterministic conflict order for stable stderr output.
	sort.Strings(conflicts)
	return out, conflicts
}
