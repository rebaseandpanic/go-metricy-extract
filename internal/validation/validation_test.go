package validation

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
)

// fixedClock returns a deterministic clock closure pinned to a stable
// timestamp. Shared across report-tests so generated_at assertions don't
// depend on wall time.
func fixedClock() func() time.Time {
	t := time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// fakeRule is a configurable Rule implementation used across the engine
// tests. Test cases build one (or several) fakeRules, wire them into
// Options.Rules, call Run, and assert on the Result.
type fakeRule struct {
	id          string
	severity    Severity
	description string
	violations  []Violation
}

func (r *fakeRule) ID() string                 { return r.id }
func (r *fakeRule) DefaultSeverity() Severity  { return r.severity }
func (r *fakeRule) Description() string        { return r.description }
func (r *fakeRule) Validate(_ *model.MetricSnapshot, _ Context) []Violation {
	// Return a fresh copy so the engine's in-place severity stamping does
	// not mutate the test-owned slice between calls.
	out := make([]Violation, len(r.violations))
	copy(out, r.violations)
	return out
}

// makeSnapshot builds a minimal snapshot with a single metric and an
// optional SourceLocation — enough to exercise enrichment.
func makeSnapshot(metricName, file string, line int, class, member string) *model.MetricSnapshot {
	lineCopy := line
	classCopy := class
	memberCopy := member
	return &model.MetricSnapshot{
		SchemaVersion: model.SchemaVersion,
		Metrics: []model.MetricDescriptor{
			{
				Name: metricName,
				Type: "counter",
				Help: "h",
				SourceLocation: &model.SourceLocation{
					File:   file,
					Line:   &lineCopy,
					Class:  &classCopy,
					Member: &memberCopy,
				},
			},
		},
	}
}

func TestRun_NoRulesReturnsEmptyResult(t *testing.T) {
	snap := &model.MetricSnapshot{}
	res := Run(snap, Options{})
	if res == nil {
		t.Fatal("Run returned nil")
	}
	if len(res.Violations) != 0 {
		t.Errorf("violations: got %d, want 0", len(res.Violations))
	}
	if len(res.SkippedRules) != 0 {
		t.Errorf("skipped: got %v, want empty", res.SkippedRules)
	}
}

func TestRun_SingleRuleOneViolation(t *testing.T) {
	r := &fakeRule{
		id:       "fake.rule",
		severity: SeverityError,
		violations: []Violation{
			{RuleID: "fake.rule", Message: "boom"},
		},
	}
	res := Run(&model.MetricSnapshot{}, Options{Rules: []Rule{r}})
	if len(res.Violations) != 1 {
		t.Fatalf("violations: got %d, want 1", len(res.Violations))
	}
	if res.Violations[0].Severity != SeverityError {
		t.Errorf("severity: got %v, want SeverityError", res.Violations[0].Severity)
	}
}

func TestRun_SeverityRestampedToEffective(t *testing.T) {
	r := &fakeRule{
		id:       "fake.rule",
		severity: SeverityWarning,
		// Rule returns Warning in the violation; engine should re-stamp it
		// to Error because of the explicit SeverityOverride below.
		violations: []Violation{
			{RuleID: "fake.rule", Severity: SeverityWarning, Message: "x"},
		},
	}
	res := Run(&model.MetricSnapshot{}, Options{
		Rules:            []Rule{r},
		SeverityOverride: map[string]Severity{"fake.rule": SeverityError},
	})
	if res.Violations[0].Severity != SeverityError {
		t.Errorf("severity: got %v, want SeverityError (override)", res.Violations[0].Severity)
	}
}

func TestRun_SkipRuleExcludesViolations(t *testing.T) {
	r := &fakeRule{
		id:         "fake.rule",
		severity:   SeverityError,
		violations: []Violation{{RuleID: "fake.rule", Message: "x"}},
	}
	res := Run(&model.MetricSnapshot{}, Options{
		Rules: []Rule{r},
		Skip:  map[string]bool{"fake.rule": true},
	})
	if len(res.Violations) != 0 {
		t.Errorf("violations: got %d, want 0 (rule skipped)", len(res.Violations))
	}
	if len(res.SkippedRules) != 1 || res.SkippedRules[0] != "fake.rule" {
		t.Errorf("skipped: got %v, want [fake.rule]", res.SkippedRules)
	}
}

func TestRun_StrictPromotesWarningsToErrors(t *testing.T) {
	r := &fakeRule{
		id:         "fake.rule",
		severity:   SeverityWarning,
		violations: []Violation{{RuleID: "fake.rule", Message: "x"}},
	}
	res := Run(&model.MetricSnapshot{}, Options{
		Rules:  []Rule{r},
		Strict: true,
	})
	if res.Violations[0].Severity != SeverityError {
		t.Errorf("severity: got %v, want SeverityError (strict promotion)", res.Violations[0].Severity)
	}
}

func TestRun_StrictDoesNotOverrideExplicitWarnRule(t *testing.T) {
	r := &fakeRule{
		id:         "fake.rule",
		severity:   SeverityWarning,
		violations: []Violation{{RuleID: "fake.rule", Message: "x"}},
	}
	res := Run(&model.MetricSnapshot{}, Options{
		Rules:            []Rule{r},
		Strict:           true,
		SeverityOverride: map[string]Severity{"fake.rule": SeverityWarning},
	})
	if res.Violations[0].Severity != SeverityWarning {
		t.Errorf("severity: got %v, want SeverityWarning (explicit beats strict)", res.Violations[0].Severity)
	}
}

func TestRun_ViolationsSortedDeterministically(t *testing.T) {
	// Three rules, intentionally declared out of order with violations in
	// equally chaotic order — Run must surface them sorted by
	// (RuleID, MetricName, LabelName, Message).
	rC := &fakeRule{id: "c.rule", severity: SeverityError,
		violations: []Violation{
			{RuleID: "c.rule", Message: "z", Location: &Location{MetricName: "m2"}},
			{RuleID: "c.rule", Message: "a", Location: &Location{MetricName: "m1"}},
		}}
	rA := &fakeRule{id: "a.rule", severity: SeverityError,
		violations: []Violation{
			{RuleID: "a.rule", Message: "b", Location: &Location{MetricName: "m1", LabelName: "y"}},
			{RuleID: "a.rule", Message: "a", Location: &Location{MetricName: "m1", LabelName: "x"}},
		}}
	rB := &fakeRule{id: "b.rule", severity: SeverityError,
		violations: []Violation{{RuleID: "b.rule", Message: "x"}},
	}

	res := Run(&model.MetricSnapshot{}, Options{Rules: []Rule{rC, rA, rB}})

	wantOrder := []string{"a.rule", "a.rule", "b.rule", "c.rule", "c.rule"}
	if len(res.Violations) != len(wantOrder) {
		t.Fatalf("violations count: got %d, want %d", len(res.Violations), len(wantOrder))
	}
	for i, want := range wantOrder {
		if res.Violations[i].RuleID != want {
			t.Errorf("violation[%d].RuleID: got %q, want %q", i, res.Violations[i].RuleID, want)
		}
	}
	// Within a.rule, the (LabelName) tiebreaker should put x before y.
	if res.Violations[0].Location.LabelName != "x" {
		t.Errorf("a.rule first violation: got LabelName=%q, want x", res.Violations[0].Location.LabelName)
	}
	// Within c.rule, the (MetricName) tiebreaker should put m1 before m2.
	if res.Violations[3].Location.MetricName != "m1" {
		t.Errorf("c.rule first violation: got MetricName=%q, want m1", res.Violations[3].Location.MetricName)
	}
}

func TestRun_ViolationsEnrichedWithSourceLocation(t *testing.T) {
	snap := makeSnapshot("http_requests_total", "pkg/x.go", 5, "Svc", "Counter")
	r := &fakeRule{
		id:         "fake.rule",
		severity:   SeverityError,
		violations: []Violation{{RuleID: "fake.rule", Message: "x", Location: &Location{MetricName: "http_requests_total"}}},
	}
	res := Run(snap, Options{Rules: []Rule{r}})
	loc := res.Violations[0].Location
	if loc.File != "pkg/x.go" {
		t.Errorf("loc.File: got %q, want pkg/x.go", loc.File)
	}
	if loc.Line == nil || *loc.Line != 5 {
		t.Errorf("loc.Line: got %v, want 5", loc.Line)
	}
	if loc.ClassName != "Svc" {
		t.Errorf("loc.ClassName: got %q, want Svc", loc.ClassName)
	}
	if loc.MemberName != "Counter" {
		t.Errorf("loc.MemberName: got %q, want Counter", loc.MemberName)
	}
}

func TestRun_ViolationsWithoutMetricNameNotEnriched(t *testing.T) {
	snap := makeSnapshot("http_requests_total", "pkg/x.go", 5, "Svc", "Counter")
	// Violation has no MetricName → must not pick up File/Line from any
	// metric in the snapshot.
	r := &fakeRule{
		id:         "fake.rule",
		severity:   SeverityError,
		violations: []Violation{{RuleID: "fake.rule", Message: "x", Location: &Location{}}},
	}
	res := Run(snap, Options{Rules: []Rule{r}})
	loc := res.Violations[0].Location
	if loc.File != "" || loc.Line != nil {
		t.Errorf("loc enriched despite missing MetricName: %+v", loc)
	}
}

// TestRun_EnrichPreservesRuleSuppliedLocation — when a rule already set
// Location.File (e.g. the exact label line), the engine must not overwrite
// it with the metric-level SourceLocation.
func TestRun_EnrichPreservesRuleSuppliedLocation(t *testing.T) {
	snap := makeSnapshot("m", "metric.go", 10, "C", "M")
	customLine := 42
	r := &fakeRule{
		id:       "fake.rule",
		severity: SeverityError,
		violations: []Violation{{RuleID: "fake.rule", Message: "x", Location: &Location{
			MetricName: "m",
			File:       "label.go",
			Line:       &customLine,
		}}},
	}
	res := Run(snap, Options{Rules: []Rule{r}})
	loc := res.Violations[0].Location
	if loc.File != "label.go" {
		t.Errorf("File clobbered: got %q, want label.go", loc.File)
	}
	if loc.Line == nil || *loc.Line != 42 {
		t.Errorf("Line clobbered: got %v, want 42", loc.Line)
	}
}

func TestBuildOverrides_ExplicitErrorBeatsStrict(t *testing.T) {
	rules := []Rule{
		&fakeRule{id: "x", severity: SeverityWarning},
		&fakeRule{id: "y", severity: SeverityWarning},
	}
	overrides, conflicts := BuildOverrides(rules, true /*strict*/, nil, []string{"x"})
	if overrides["x"] != SeverityError {
		t.Errorf("x: got %v, want Error (explicit)", overrides["x"])
	}
	// y remains Error too via strict promotion.
	if overrides["y"] != SeverityError {
		t.Errorf("y: got %v, want Error (strict promotion)", overrides["y"])
	}
	if len(conflicts) != 0 {
		t.Errorf("conflicts: got %v, want empty", conflicts)
	}
}

func TestBuildOverrides_ErrorRuleWinsOverWarnRule(t *testing.T) {
	rules := []Rule{&fakeRule{id: "x", severity: SeverityWarning}}
	overrides, conflicts := BuildOverrides(rules, false, []string{"x"}, []string{"x"})
	if overrides["x"] != SeverityError {
		t.Errorf("x: got %v, want Error (error-rule wins)", overrides["x"])
	}
	if len(conflicts) != 1 || conflicts[0] != "x" {
		t.Errorf("conflicts: got %v, want [x]", conflicts)
	}
}

func TestBuildOverrides_NoConflictReturnsEmpty(t *testing.T) {
	rules := []Rule{
		&fakeRule{id: "a", severity: SeverityError},
		&fakeRule{id: "b", severity: SeverityError},
	}
	_, conflicts := BuildOverrides(rules, false, []string{"a"}, []string{"b"})
	if len(conflicts) != 0 {
		t.Errorf("conflicts: got %v, want empty", conflicts)
	}
}

// TestBuildOverrides_UnknownIDsIgnored — unknown rule IDs are silently
// dropped from the override map; the CLI layer handles the user-facing
// warning separately.
func TestBuildOverrides_UnknownIDsIgnored(t *testing.T) {
	rules := []Rule{&fakeRule{id: "known", severity: SeverityError}}
	overrides, conflicts := BuildOverrides(rules, false, []string{"unknown"}, []string{"also-unknown"})
	if _, ok := overrides["unknown"]; ok {
		t.Errorf("unknown ID leaked into overrides: %v", overrides)
	}
	if _, ok := overrides["also-unknown"]; ok {
		t.Errorf("also-unknown ID leaked into overrides: %v", overrides)
	}
	if len(conflicts) != 0 {
		t.Errorf("conflicts: got %v, want empty", conflicts)
	}
}

func TestSeverity_StringMethod(t *testing.T) {
	if SeverityError.String() != "error" {
		t.Errorf("SeverityError.String(): got %q, want error", SeverityError.String())
	}
	if SeverityWarning.String() != "warning" {
		t.Errorf("SeverityWarning.String(): got %q, want warning", SeverityWarning.String())
	}
	if Severity(99).String() != "unknown" {
		t.Errorf("unknown severity: got %q, want unknown", Severity(99).String())
	}
}

func TestViolation_JSONSerializesAsSeverityString(t *testing.T) {
	v := Violation{RuleID: "r", Severity: SeverityError, Message: "m"}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"severity":"error"`) {
		t.Errorf("json: got %q, want to contain \"severity\":\"error\"", data)
	}
}

func TestReport_SchemaVersionIs11(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, &Result{}, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, buf.String())
	}
	if rep.SchemaVersion != "1.1" {
		t.Errorf("schema_version: got %q, want 1.1", rep.SchemaVersion)
	}
}

func TestWriteReport_EmptyResultWritesValidJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, &Result{}, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	// Must be valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, buf.String())
	}
	// "violations" must serialize as [] — not null — so downstream tools
	// don't trip over nil.
	if vs, ok := parsed["violations"].([]any); !ok || len(vs) != 0 {
		t.Errorf("violations: got %v (%T), want empty array", parsed["violations"], parsed["violations"])
	}
	if parsed["error_count"].(float64) != 0 || parsed["warning_count"].(float64) != 0 {
		t.Errorf("counts: got errors=%v warnings=%v, want 0/0", parsed["error_count"], parsed["warning_count"])
	}
	// Additive v1.1 envelope fields: generated_at must be present,
	// by_rule must be an empty array (never null).
	if _, ok := parsed["generated_at"].(string); !ok {
		t.Errorf("generated_at: got %v (%T), want non-empty string", parsed["generated_at"], parsed["generated_at"])
	}
	if br, ok := parsed["by_rule"].([]any); !ok || len(br) != 0 {
		t.Errorf("by_rule: got %v (%T), want empty array", parsed["by_rule"], parsed["by_rule"])
	}
}

func TestWriteReport_IncludesCounts(t *testing.T) {
	res := &Result{
		Violations: []Violation{
			{RuleID: "a", Severity: SeverityError, Message: "m1"},
			{RuleID: "b", Severity: SeverityError, Message: "m2"},
			{RuleID: "c", Severity: SeverityWarning, Message: "m3"},
			{RuleID: "d", Severity: SeverityWarning, Message: "m4"},
			{RuleID: "e", Severity: SeverityWarning, Message: "m5"},
		},
	}
	var buf bytes.Buffer
	if err := WriteReport(&buf, res, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rep.ErrorCount != 2 {
		t.Errorf("error_count: got %d, want 2", rep.ErrorCount)
	}
	if rep.WarningCount != 3 {
		t.Errorf("warning_count: got %d, want 3", rep.WarningCount)
	}
}

func TestFormatStderrSummary_EmptyReturnsEmpty(t *testing.T) {
	if got := FormatStderrSummary(nil); got != "" {
		t.Errorf("nil: got %q, want empty", got)
	}
	if got := FormatStderrSummary(&Result{}); got != "" {
		t.Errorf("empty result: got %q, want empty", got)
	}
}

func TestFormatStderrSummary_OneLinePerViolation(t *testing.T) {
	res := &Result{
		Violations: []Violation{
			{RuleID: "a", Severity: SeverityError, Message: "m1", Location: &Location{MetricName: "x"}},
			{RuleID: "b", Severity: SeverityWarning, Message: "m2"},
		},
	}
	got := FormatStderrSummary(res)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// 2 violations + 1 footer line = 3 lines.
	if len(lines) != 3 {
		t.Fatalf("lines: got %d, want 3 (got=%q)", len(lines), got)
	}
	if !strings.Contains(lines[0], "[error]") || !strings.Contains(lines[0], "a:") {
		t.Errorf("line 0: got %q", lines[0])
	}
	if !strings.Contains(lines[1], "[warning]") {
		t.Errorf("line 1: got %q", lines[1])
	}
	if !strings.Contains(lines[2], "1 error") {
		t.Errorf("footer: got %q", lines[2])
	}
}

// TestFormatViolationLine_IncludesFileAndLine verifies the file:line tail
// of the summary line — covers the File-present / Line-present branches of
// the formatter.
func TestFormatViolationLine_IncludesFileAndLine(t *testing.T) {
	lineNum := 7
	v := Violation{
		RuleID:   "a",
		Severity: SeverityError,
		Message:  "m",
		Location: &Location{MetricName: "x", File: "f.go", Line: &lineNum},
	}
	got := formatViolationLine(v)
	if !strings.Contains(got, "f.go:7") {
		t.Errorf("line missing file:line: got %q", got)
	}
}

// TestRun_DefaultOffSkippedWithoutEnable — a rule flagged default-off must
// be invisible to Run unless --enable-rule names it. It must not appear in
// SkippedRules either (Skip is for explicit --skip-rule, not filtering).
func TestRun_DefaultOffSkippedWithoutEnable(t *testing.T) {
	r := &fakeRule{
		id:         "r1",
		severity:   SeverityWarning,
		violations: []Violation{{RuleID: "r1", Message: "m"}},
	}
	res := Run(&model.MetricSnapshot{}, Options{
		Rules:      []Rule{r},
		DefaultOff: map[string]bool{"r1": true},
	})
	if len(res.Violations) != 0 {
		t.Errorf("violations: got %d, want 0 (rule off by default)", len(res.Violations))
	}
	for _, id := range res.SkippedRules {
		if id == "r1" {
			t.Errorf("default-off rule leaked into SkippedRules: %v", res.SkippedRules)
		}
	}
}

// TestRun_DefaultOffRunsWhenEnabled — Enable must unlock a default-off rule
// and let its violations flow through.
func TestRun_DefaultOffRunsWhenEnabled(t *testing.T) {
	r := &fakeRule{
		id:         "r1",
		severity:   SeverityWarning,
		violations: []Violation{{RuleID: "r1", Message: "m"}},
	}
	res := Run(&model.MetricSnapshot{}, Options{
		Rules:      []Rule{r},
		DefaultOff: map[string]bool{"r1": true},
		Enable:     map[string]bool{"r1": true},
	})
	if len(res.Violations) != 1 {
		t.Errorf("violations: got %d, want 1 (enabled)", len(res.Violations))
	}
}

// TestRun_NilSnapshotReturnsEmptyResult — nil snapshot must be absorbed by
// the engine so rules never see nil themselves.
func TestRun_NilSnapshotReturnsEmptyResult(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Run panicked on nil snapshot: %v", r)
		}
	}()
	res := Run(nil, Options{Rules: []Rule{&fakeRule{id: "r", severity: SeverityError}}})
	if res == nil {
		t.Fatal("Run returned nil result")
	}
	if len(res.Violations) != 0 || len(res.SkippedRules) != 0 {
		t.Errorf("non-empty result on nil snapshot: %+v", res)
	}
}

// TestRun_EnrichmentMissingMetricKeepsLocation — when a rule references a
// metric that is not in the snapshot, enrichLocation must leave the
// rule-supplied Location untouched (no clobber, no panic).
func TestRun_EnrichmentMissingMetricKeepsLocation(t *testing.T) {
	snap := makeSnapshot("a", "a.go", 1, "C", "M")
	r := &fakeRule{
		id:       "fake.rule",
		severity: SeverityError,
		violations: []Violation{{
			RuleID:   "fake.rule",
			Message:  "x",
			Location: &Location{MetricName: "b"}, // "b" absent from snapshot
		}},
	}
	res := Run(snap, Options{Rules: []Rule{r}})
	if len(res.Violations) != 1 {
		t.Fatalf("violations: got %d, want 1", len(res.Violations))
	}
	loc := res.Violations[0].Location
	if loc == nil || loc.MetricName != "b" {
		t.Fatalf("Location lost: %+v", loc)
	}
	if loc.File != "" || loc.Line != nil || loc.ClassName != "" || loc.MemberName != "" {
		t.Errorf("missing-metric violation got enrichment: %+v", loc)
	}
}

// TestRun_EnrichLocationNilNotPanics — a rule that emits a violation with
// no Location at all must flow through the sort and enrichment paths
// without panicking, and the violation must be preserved.
func TestRun_EnrichLocationNilNotPanics(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic on nil Location: %v", rec)
		}
	}()
	r := &fakeRule{
		id:         "fake.rule",
		severity:   SeverityError,
		violations: []Violation{{RuleID: "fake.rule", Message: "m"}},
	}
	res := Run(&model.MetricSnapshot{}, Options{Rules: []Rule{r}})
	if len(res.Violations) != 1 {
		t.Fatalf("violations: got %d, want 1", len(res.Violations))
	}
	if res.Violations[0].Location != nil {
		t.Errorf("Location: got %+v, want nil", res.Violations[0].Location)
	}
}

// TestWriteReport_NilResultProducesEmptyEnvelope — nil Result must be
// normalised to the same envelope shape as an empty Result.
func TestWriteReport_NilResultProducesEmptyEnvelope(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, nil, fixedClock()); err != nil {
		t.Fatalf("WriteReport(nil): %v", err)
	}
	var rep map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, buf.String())
	}
	if rep["schema_version"] != "1.1" {
		t.Errorf("schema_version: got %v, want 1.1", rep["schema_version"])
	}
	if vs, ok := rep["violations"].([]any); !ok || len(vs) != 0 {
		t.Errorf("violations: got %v, want empty array", rep["violations"])
	}
	if rep["error_count"].(float64) != 0 || rep["warning_count"].(float64) != 0 {
		t.Errorf("counts: got %v/%v, want 0/0", rep["error_count"], rep["warning_count"])
	}
	// Additive v1.1 fields: by_rule must be [] (not null) on nil input.
	if br, ok := rep["by_rule"].([]any); !ok || len(br) != 0 {
		t.Errorf("by_rule: got %v (%T), want empty array", rep["by_rule"], rep["by_rule"])
	}
	// Trailing newline is part of the contract.
	if b := buf.Bytes(); len(b) == 0 || b[len(b)-1] != '\n' {
		t.Errorf("trailing newline missing: %q", buf.String())
	}
}

// TestWriteReport_OutputShape pins the byte-level shape of the report:
// opens with `{\n  "schema_version"` (2-space indent) and ends with `}\n`
// (closing brace + trailing newline).
func TestWriteReport_OutputShape(t *testing.T) {
	res := &Result{
		Violations: []Violation{{RuleID: "a", Severity: SeverityError, Message: "m"}},
	}
	var buf bytes.Buffer
	if err := WriteReport(&buf, res, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	s := buf.String()
	if !strings.HasPrefix(s, "{\n  \"schema_version\"") {
		t.Errorf("prefix: got %q, want {\\n  \"schema_version\"…", s[:min(len(s), 40)])
	}
	if !strings.HasSuffix(s, "}\n") {
		tail := s
		if len(tail) > 20 {
			tail = tail[len(tail)-20:]
		}
		t.Errorf("suffix: got %q, want }\\n", tail)
	}
}

// TestFormatViolationLine_ElisionBranches covers the three renderings that
// were previously untested: label-only target, File-without-Line tail,
// and entirely nil Location.
func TestFormatViolationLine_ElisionBranches(t *testing.T) {
	cases := []struct {
		name string
		v    Violation
		want []string // substrings that must appear
		deny []string // substrings that must NOT appear
	}{
		{
			name: "LabelName only (no MetricName)",
			v: Violation{
				RuleID:   "r",
				Severity: SeverityError,
				Message:  "m",
				Location: &Location{LabelName: "labelname"},
			},
			want: []string{" at :labelname"},
		},
		{
			name: "File set, Line nil",
			v: Violation{
				RuleID:   "r",
				Severity: SeverityError,
				Message:  "m",
				Location: &Location{MetricName: "x", File: "file.go"},
			},
			want: []string{" (file.go)"},
		},
		{
			name: "Location nil",
			v: Violation{
				RuleID:   "r",
				Severity: SeverityError,
				Message:  "m",
			},
			want: []string{"[error] r: m"},
			deny: []string{" at ", "("},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatViolationLine(tc.v)
			for _, s := range tc.want {
				if !strings.Contains(got, s) {
					t.Errorf("missing substring %q in %q", s, got)
				}
			}
			// File-without-Line case: check the `(…)` tail separately to
			// avoid false positives from message/rule substring matching.
			if tc.name == "File set, Line nil" {
				if !strings.HasSuffix(got, "(file.go)") {
					t.Errorf("suffix: got %q, want to end with (file.go)", got)
				}
			}
			if tc.name == "Location nil" {
				for _, s := range tc.deny {
					if strings.Contains(got, s) {
						t.Errorf("unexpected substring %q in %q", s, got)
					}
				}
			}
		})
	}
}

// TestBuildOverrides_StrictWithExplicitWarnPreservesWarning — explicit
// --warn-rule wins over --strict at the BuildOverrides boundary (not just
// at the Run layer).
func TestBuildOverrides_StrictWithExplicitWarnPreservesWarning(t *testing.T) {
	rules := []Rule{&fakeRule{id: "x", severity: SeverityWarning}}
	overrides, conflicts := BuildOverrides(rules, true /*strict*/, []string{"x"}, nil)
	if overrides["x"] != SeverityWarning {
		t.Errorf("x: got %v, want Warning (explicit warn beats strict)", overrides["x"])
	}
	if len(conflicts) != 0 {
		t.Errorf("conflicts: got %v, want empty", conflicts)
	}
}

// TestBuildOverrides_EmptyRulesNoPanic — empty rules slice must return an
// empty/nil map with no conflicts and no panic.
func TestBuildOverrides_EmptyRulesNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	overrides, conflicts := BuildOverrides(nil, false, nil, nil)
	if len(overrides) != 0 {
		t.Errorf("overrides: got %v, want empty", overrides)
	}
	if conflicts != nil {
		t.Errorf("conflicts: got %v, want nil", conflicts)
	}
}

// TestWriteReport_ByRuleAggregates — by_rule must surface one entry per
// distinct rule ID, with per-severity counts that match the violations
// list. Mixed severities within the same engine run exercise the
// rare-but-valid case where explicit overrides land on the same rule
// across violations of different origin — use two distinct rule IDs so the
// grouping is clearly observable.
func TestWriteReport_ByRuleAggregates(t *testing.T) {
	res := &Result{
		Violations: []Violation{
			{RuleID: "a.b", Severity: SeverityError, Message: "e1"},
			{RuleID: "a.b", Severity: SeverityError, Message: "e2"},
			{RuleID: "c.d", Severity: SeverityWarning, Message: "w1"},
		},
	}
	var buf bytes.Buffer
	if err := WriteReport(&buf, res, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rep.ByRule) != 2 {
		t.Fatalf("by_rule entries: got %d, want 2 (%+v)", len(rep.ByRule), rep.ByRule)
	}
	// Sort by RuleID is tested separately; rely on the contract here.
	if rep.ByRule[0].RuleID != "a.b" || rep.ByRule[0].Severity != "error" ||
		rep.ByRule[0].ErrorCount != 2 || rep.ByRule[0].WarningCount != 0 {
		t.Errorf("a.b entry: got %+v, want {a.b error 2/0}", rep.ByRule[0])
	}
	if rep.ByRule[1].RuleID != "c.d" || rep.ByRule[1].Severity != "warning" ||
		rep.ByRule[1].ErrorCount != 0 || rep.ByRule[1].WarningCount != 1 {
		t.Errorf("c.d entry: got %+v, want {c.d warning 0/1}", rep.ByRule[1])
	}
}

// TestWriteReport_ByRuleSortedDeterministically — violations added in
// random RuleID order must produce a by_rule list sorted ordinally by
// RuleID regardless of input order. Guards golden stability.
func TestWriteReport_ByRuleSortedDeterministically(t *testing.T) {
	res := &Result{
		Violations: []Violation{
			{RuleID: "z.last", Severity: SeverityError, Message: "m"},
			{RuleID: "a.first", Severity: SeverityError, Message: "m"},
			{RuleID: "m.mid", Severity: SeverityWarning, Message: "m"},
		},
	}
	var buf bytes.Buffer
	if err := WriteReport(&buf, res, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantOrder := []string{"a.first", "m.mid", "z.last"}
	for i, want := range wantOrder {
		if rep.ByRule[i].RuleID != want {
			t.Errorf("by_rule[%d]: got %q, want %q", i, rep.ByRule[i].RuleID, want)
		}
	}
}

// TestWriteReport_MixedSeverityByRule_PromotesToError — the defensive W1
// loop promotes a mixed-severity rule group to Severity="error" so the
// label reflects the worst observed violation. Engine re-stamping makes
// this impossible in practice, but a hand-constructed Result (bypassing
// the engine) can still land mixed severities under one RuleID; the
// report contract must handle it deterministically. Supersedes the
// earlier "first-violation wins" contract.
func TestWriteReport_MixedSeverityByRule_PromotesToError(t *testing.T) {
	res := &Result{
		Violations: []Violation{
			{RuleID: "mixed.rule", Severity: SeverityError, Message: "err-one"},
			{RuleID: "mixed.rule", Severity: SeverityWarning, Message: "warn-one"},
			{RuleID: "mixed.rule", Severity: SeverityError, Message: "err-two"},
		},
	}
	var buf bytes.Buffer
	if err := WriteReport(&buf, res, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rep.ByRule) != 1 {
		t.Fatalf("want 1 rule entry, got %d (%+v)", len(rep.ByRule), rep.ByRule)
	}
	rc := rep.ByRule[0]
	if rc.Severity != "error" {
		t.Errorf("Severity: got %q, want error (promoted from mixed)", rc.Severity)
	}
	if rc.ErrorCount != 2 {
		t.Errorf("ErrorCount: got %d, want 2", rc.ErrorCount)
	}
	if rc.WarningCount != 1 {
		t.Errorf("WarningCount: got %d, want 1", rc.WarningCount)
	}
}

// TestWriteReport_NonUTCClockNormalizedToUTC — clocks in a non-UTC zone
// (e.g. Moscow +03:00) must be converted to UTC before formatting so the
// wire output always ends with the literal "Z" suffix. Without this
// conversion, Format would emit an offset like "+0300" and break the
// byte-stable golden contract shared with MetricSnapshot.ExtractedAt.
func TestWriteReport_NonUTCClockNormalizedToUTC(t *testing.T) {
	moscow := time.FixedZone("MSK", 3*3600)
	fixedTime := time.Date(2026, 4, 21, 15, 0, 0, 0, moscow) // 15:00 MSK = 12:00 UTC
	clock := func() time.Time { return fixedTime }

	var buf bytes.Buffer
	if err := WriteReport(&buf, &Result{}, clock); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v (body=%q)", err, buf.String())
	}
	const want = "2026-04-21T12:00:00Z"
	if rep.GeneratedAt != want {
		t.Errorf("GeneratedAt: got %q, want %q (UTC)", rep.GeneratedAt, want)
	}
	if !strings.HasSuffix(rep.GeneratedAt, "Z") {
		t.Errorf("GeneratedAt should end with Z; got %q", rep.GeneratedAt)
	}
}

// TestWriteReport_FieldOrderIsStable pins the order of top-level fields
// in the emitted JSON: schema_version, generated_at, violations,
// error_count, warning_count, by_rule. Go's encoding/json writes struct
// fields in declaration order, so this test guards against accidental
// reshuffling of the Report struct that would break byte-stable golden
// files in downstream consumers.
func TestWriteReport_FieldOrderIsStable(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, &Result{}, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	s := buf.String()
	order := []string{
		`"schema_version"`,
		`"generated_at"`,
		`"violations"`,
		`"error_count"`,
		`"warning_count"`,
		`"by_rule"`,
	}

	lastPos := -1
	for _, key := range order {
		pos := strings.Index(s, key)
		if pos == -1 {
			t.Errorf("key %s missing from output", key)
			continue
		}
		if pos <= lastPos {
			t.Errorf("key %s at pos %d appears after previous (pos %d) — field order reshuffled", key, pos, lastPos)
		}
		lastPos = pos
	}
}

// TestWriteReport_GeneratedAtUsesClockOverride — when a fixed clock is
// injected, generated_at matches the pinned instant in ISO-8601 UTC
// second-precision. This is the mechanism golden-file tests rely on for
// byte stability.
func TestWriteReport_GeneratedAtUsesClockOverride(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, &Result{}, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	const want = "2026-04-20T10:00:00Z"
	if rep.GeneratedAt != want {
		t.Errorf("generated_at: got %q, want %q", rep.GeneratedAt, want)
	}
}

// TestWriteReport_GeneratedAtDefaultsToTimeNow — a nil clock falls back to
// time.Now. We can't assert the exact value without racing, so we verify
// the shape (non-empty, parseable in the documented format).
func TestWriteReport_GeneratedAtDefaultsToTimeNow(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, &Result{}, nil); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	var rep Report
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rep.GeneratedAt == "" {
		t.Fatal("generated_at: got empty, want non-empty")
	}
	if _, err := time.Parse(model.ExtractedAtLayout, rep.GeneratedAt); err != nil {
		t.Errorf("generated_at not in expected format: %q (%v)", rep.GeneratedAt, err)
	}
}

// TestWriteReport_EmptyResultHasByRuleAsEmptyArray — a Result with no
// violations must serialize by_rule as [] (not null). Repeats the same
// check the envelope test performs, but at the typed-struct level to
// catch any accidental *RuleCount conversions.
func TestWriteReport_EmptyResultHasByRuleAsEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, &Result{}, fixedClock()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	// Raw substring check: "null" would indicate the nil-slice path
	// leaked through; "[]" is the contract.
	s := buf.String()
	if !strings.Contains(s, "\"by_rule\": []") {
		t.Errorf("by_rule not [] in output: %s", s)
	}
}

// TestWriteReport_NilResultHasByRuleAsEmptyArray — same check as the
// EmptyResult variant, but for the nil-Result path through WriteReport.
func TestWriteReport_NilResultHasByRuleAsEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, nil, fixedClock()); err != nil {
		t.Fatalf("WriteReport(nil): %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "\"by_rule\": []") {
		t.Errorf("by_rule not [] in output: %s", s)
	}
}
