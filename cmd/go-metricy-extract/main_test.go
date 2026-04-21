package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/validation/rules"
)

// writeGoFile writes content to <dir>/<name>, creating parent dirs as needed.
// Used to build mini test fixtures inside t.TempDir().
func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestRun_SourceRequiredReturnsExit2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code: got %d, want 2 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--source is required") {
		t.Errorf("stderr missing '--source is required': %q", stderr.String())
	}
}

func TestRun_UnknownFlagReturnsExit2(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--foo", "bar"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("exit code: got %d, want 2 (stderr=%q)", code, stderr.String())
	}
}

func TestRun_HelpReturnsExit0(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-h"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("stdout missing 'Usage:' line: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "--source") {
		t.Errorf("stdout missing '--source' flag description: %q", stdout.String())
	}
}

func TestRun_HappyPath(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "requests_total", Help: "total"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}

	// Stdout must be parseable JSON with all required snapshot fields.
	var snap map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		t.Fatalf("json parse: %v (stdout=%q)", err, stdout.String())
	}
	for _, key := range []string{"schema_version", "project", "extracted_at", "extractor", "metrics"} {
		if _, ok := snap[key]; !ok {
			t.Errorf("snapshot missing key %q: %+v", key, snap)
		}
	}
	// Metrics should contain exactly one entry.
	metrics, ok := snap["metrics"].([]any)
	if !ok {
		t.Fatalf("metrics: got %T, want []any", snap["metrics"])
	}
	if len(metrics) != 1 {
		t.Fatalf("metrics count: got %d, want 1", len(metrics))
	}
	m := metrics[0].(map[string]any)
	if m["name"] != "requests_total" {
		t.Errorf("metric name: got %v, want requests_total", m["name"])
	}
}

func TestRun_OutputFile(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`)
	outPath := filepath.Join(root, "out.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--output", outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	// With --output set, stdout should be empty (JSON goes to file).
	if stdout.Len() != 0 {
		t.Errorf("stdout: got %q, want empty when --output is set", stdout.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	var snap map[string]any
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("output file JSON: %v (content=%q)", err, string(data))
	}
	if _, ok := snap["schema_version"]; !ok {
		t.Errorf("output JSON missing schema_version: %+v", snap)
	}
}

func TestRun_OutputFileWriteErrorReturnsExit3(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`)

	var stdout, stderr bytes.Buffer
	// Parent directory does not exist — WriteFile must fail on the .tmp file
	// (atomic-write pattern writes to <output>.tmp first, then renames).
	// Portable: TempDir root exists, a nested subdir beneath it does not.
	// Exit 3 because a filesystem write failure is a tool-crash (I/O), not
	// a validation finding — CI scripts distinguish these cases.
	bad := filepath.Join(t.TempDir(), "does-not-exist", "out.json")
	code := run([]string{"--source", root, "--output", bad}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit code: got %d, want 3 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "failed to write") {
		t.Errorf("stderr missing 'failed to write': %q", stderr.String())
	}
}

// TestRun_WarningsPrintedToStderr doubles as the separation test (formerly
// TestRun_WarningsOnStderrSnapshotOnStdoutSeparately): warnings go to stderr
// with a "warn:" prefix while stdout stays strictly a parseable JSON snapshot
// — no warning lines mixed in.
func TestRun_WarningsPrintedToStderr(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "broken.go", `package main
func{
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (parse error is non-fatal); stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "warn:") {
		t.Errorf("stderr missing 'warn:' line: %q", stderr.String())
	}
	// Stdout must be strictly valid JSON — no warning lines leaked in.
	var snap map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		t.Fatalf("stdout is not valid JSON (warnings leaked in?): %v; stdout=%q", err, stdout.String())
	}
	if _, ok := snap["metrics"]; !ok {
		t.Errorf("snapshot missing 'metrics' key: %+v", snap)
	}
}

func TestRun_MissingSourceDirReturnsExit3(t *testing.T) {
	// --source pointing at a non-existent path makes the pipeline fail
	// before any validation runs. That's a tool-crash (exit 3), distinct
	// from "your code has validation issues" (exit 1) or "you passed bad
	// flags" (exit 2).
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "does-not-exist", "path")
	code := run([]string{"--source", missing}, &stdout, &stderr)
	if code != 3 {
		t.Errorf("exit code: got %d, want 3 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr missing 'error:' prefix: %q", stderr.String())
	}
}

func TestRun_ProjectFlagPropagates(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--project", "custom-svc"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	var snap map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		t.Fatalf("json parse: %v (stdout=%q)", err, stdout.String())
	}
	if got := snap["project"]; got != "custom-svc" {
		t.Errorf("snap.project: got %v, want custom-svc", got)
	}
}

func TestRun_RepoRootFlagPropagates(t *testing.T) {
	outer := t.TempDir()
	// Build outer/.git so auto-detect would otherwise land on outer. Place
	// .go fixture in outer/inner. Override --repo-root=<inner> so the emitted
	// SourceLocation.File is relative to inner, not outer — that proves the
	// flag won over auto-detection.
	if err := os.MkdirAll(filepath.Join(outer, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	inner := filepath.Join(outer, "inner")
	writeGoFile(t, inner, "metrics.go", `package pkg
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", inner, "--repo-root", inner}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	var snap map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &snap); err != nil {
		t.Fatalf("json parse: %v (stdout=%q)", err, stdout.String())
	}
	metrics, ok := snap["metrics"].([]any)
	if !ok || len(metrics) != 1 {
		t.Fatalf("metrics: got %v, want exactly 1 entry", snap["metrics"])
	}
	m := metrics[0].(map[string]any)
	sl, ok := m["source_location"].(map[string]any)
	if !ok {
		t.Fatalf("source_location missing or wrong type: %+v", m)
	}
	if got := sl["file"]; got != "metrics.go" {
		t.Errorf("source_location.file: got %v, want metrics.go (relative to --repo-root)", got)
	}
}

func TestRun_JSONUses2SpaceIndent(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	// MarshalIndent("", "  ") opens with "{\n  \"schema_version\"": 2-space
	// indent on the first field proves the indent setting.
	if !strings.Contains(stdout.String(), "\n  \"schema_version\"") {
		t.Errorf("stdout does not use 2-space indent (want line starting with two spaces before \"schema_version\"): %q", stdout.String())
	}
}

func TestRun_OutputHasTrailingNewline(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	b := stdout.Bytes()
	if len(b) == 0 {
		t.Fatalf("stdout empty")
	}
	if b[len(b)-1] != '\n' {
		t.Errorf("last stdout byte: got %q, want '\\n'", b[len(b)-1])
	}
}

func TestRun_HelpWritesNothingToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-h"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr: got %q, want empty on -h", stderr.String())
	}
}

func TestRun_UnknownFlagMentionsFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--foo", "bar"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code: got %d, want 2 (stderr=%q)", code, stderr.String())
	}
	// The flag package's parse error uses the word "flag" and/or the bad
	// flag's name; either signal is enough to confirm the user sees which
	// flag was wrong.
	s := stderr.String()
	if !strings.Contains(s, "foo") && !strings.Contains(s, "flag") {
		t.Errorf("stderr does not mention the bad flag or the word 'flag': %q", s)
	}
}

// fullyAnnotatedMetric is a fixture source that satisfies every built-in
// error-severity rule: Name, Help, @metric description, @metric calculation
// are all present, and there are no labels (so metric.label-description-required
// has nothing to flag). Used by validation tests that must exit 0 under the
// populated rule registry.
const fullyAnnotatedMetric = `package p
import "github.com/prometheus/client_golang/prometheus"

// X counts things.
//
// @metric description Counts the total number of requests served.
// @metric calculation Incremented once per successful request handler invocation.
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`

// TestRun_ValidateFlagEnabled — with --validate and a fully-annotated metric
// fixture, no rules fire and the CLI exits 0. Proves the validation wiring
// is harmless when every rule is satisfied.
func TestRun_ValidateFlagEnabled(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	// Snapshot still written to stdout.
	if !strings.Contains(stdout.String(), `"schema_version"`) {
		t.Errorf("stdout missing snapshot JSON: %q", stdout.String())
	}
}

// TestRun_ValidateFlagWithUnknownRule — --skip-rule referencing an ID that
// does not exist in the registry must produce a stderr "unknown rule id"
// warning. The metric itself is fully annotated so no real rules fire;
// the test is isolated to the unknown-ID warning path.
func TestRun_ValidateFlagWithUnknownRule(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--skip-rule", "foo.bar"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown rule id: foo.bar") {
		t.Errorf("stderr missing unknown rule warning: %q", stderr.String())
	}
}

// TestRun_ValidateReport_WritesFile — --validation-report writes a valid
// JSON file even when no rules fire. The report carries the versioned
// schema envelope (empty violations array, zero counts).
func TestRun_ValidateReport_WritesFile(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	reportPath := filepath.Join(root, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--validation-report", reportPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var rep map[string]any
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("report JSON: %v (content=%q)", err, string(data))
	}
	if rep["schema_version"] != "1.1" {
		t.Errorf("report schema_version: got %v, want 1.1", rep["schema_version"])
	}
	if vs, ok := rep["violations"].([]any); !ok || len(vs) != 0 {
		t.Errorf("report violations: got %v, want empty array", rep["violations"])
	}
}

// TestRun_ValidateFlagWithUnknownWarnRule — --warn-rule with an ID that is
// not in the registry must surface a stderr "unknown rule id" warning and
// still exit 0.
func TestRun_ValidateFlagWithUnknownWarnRule(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--warn-rule", "unknown.id"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown rule id: unknown.id") {
		t.Errorf("stderr missing 'unknown rule id: unknown.id': %q", stderr.String())
	}
}

// TestRun_ValidateFlagWithUnknownErrorRule — --error-rule unknown-ID path.
func TestRun_ValidateFlagWithUnknownErrorRule(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--error-rule", "unknown.id"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown rule id: unknown.id") {
		t.Errorf("stderr missing 'unknown rule id: unknown.id': %q", stderr.String())
	}
}

// TestRun_ValidateFlagWithUnknownEnableRule — --enable-rule unknown-ID path.
func TestRun_ValidateFlagWithUnknownEnableRule(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--enable-rule", "unknown.id"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown rule id: unknown.id") {
		t.Errorf("stderr missing 'unknown rule id: unknown.id': %q", stderr.String())
	}
}

// TestRun_RuleMinLengthMalformedWarns — no-colon input must produce a
// stderr warning about the expected 'RULE-ID:N' format and continue.
func TestRun_RuleMinLengthMalformedWarns(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--rule-min-length", "foo"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "invalid --rule-min-length") {
		t.Errorf("stderr missing 'invalid --rule-min-length': %q", stderr.String())
	}
}

// TestRun_RuleMinLengthNonNumericWarns — non-numeric N must warn via the
// strconv.Atoi failure branch.
func TestRun_RuleMinLengthNonNumericWarns(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--rule-min-length", "id:abc"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "invalid --rule-min-length") {
		t.Errorf("stderr missing 'invalid --rule-min-length': %q", stderr.String())
	}
}

// TestRun_RuleMinLengthNegativeClampedToZero — negative N must surface a
// "negative value … treated as 0" warning on stderr (W3 fix).
func TestRun_RuleMinLengthNegativeClampedToZero(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--rule-min-length", "id:-5"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "negative value") {
		t.Errorf("stderr missing 'negative value' warning: %q", stderr.String())
	}
}

// TestRun_RuleMinLengthValidSilent — well-formed --rule-min-length must
// not emit any rule-min-length warning. Acts as the negative counterpart
// to the malformed-input tests.
func TestRun_RuleMinLengthValidSilent(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--rule-min-length", "id:30"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "rule-min-length") {
		t.Errorf("stderr unexpectedly mentions 'rule-min-length' for well-formed input: %q", stderr.String())
	}
}

// TestRun_MinDescriptionLengthZeroEmitsWarning — user-supplied
// `--min-description-length 0` is ambiguous (matches the default), so
// the CLI must surface a stderr warning pointing at the right path
// (`--rule-min-length <id>:0`). Exit code stays 0 because the metric
// is fully annotated and nothing else fires. Catches both the
// "warning is missing" regression and the "fs.Visit never fires"
// regression (which would mean we can't detect explicit 0 at all).
func TestRun_MinDescriptionLengthZeroEmitsWarning(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--min-description-length", "0",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "min-description-length 0 is treated as 'unset'") {
		t.Errorf("stderr missing 'min-description-length 0 is treated as unset' warning: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--rule-min-length") {
		t.Errorf("stderr missing pointer to --rule-min-length: %q", stderr.String())
	}
}

// TestRun_MinDescriptionLengthUnsetNoWarning — omitting the flag
// entirely must NOT trigger the zero-warning (fs.Visit must only fire
// on flags actually passed by the user). Negative counterpart to
// TestRun_MinDescriptionLengthZeroEmitsWarning.
func TestRun_MinDescriptionLengthUnsetNoWarning(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "min-description-length 0 is treated as 'unset'") {
		t.Errorf("stderr unexpectedly emits zero-warning when flag not passed: %q", stderr.String())
	}
}

// TestRun_ValidateReport_WriteErrorReturnsExit3 — when --validation-report
// points at a path whose parent dir does not exist, the CLI must fail fast
// with exit 3 (tool-crash I/O failure, NOT exit 1 validation-failure) and
// a stderr "failed to create validation report" message.
func TestRun_ValidateReport_WriteErrorReturnsExit3(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)
	bad := filepath.Join(t.TempDir(), "does-not-exist", "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--validation-report", bad}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit code: got %d, want 3 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "validation report") {
		t.Errorf("stderr missing 'validation report' phrase: %q", stderr.String())
	}
}

// bareMetric is the "nothing annotated" fixture: satisfies only Name and
// Help so the extractor emits one metric, but every @metric / @label
// annotation-based rule fires. Used by the integration tests below that
// must observe specific violations end-to-end.
const bareMetric = `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`

// TestRun_ValidateCatchesMissingDescription — a bare metric (no @metric
// description annotation) must trigger metric.description-required and
// exit 1. The rule ID appears in the stderr summary.
func TestRun_ValidateCatchesMissingDescription(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", bareMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.description-required") {
		t.Errorf("stderr missing 'metric.description-required': %q", stderr.String())
	}
}

// TestRun_ValidateCatchesMissingHelp — a metric with an empty Help field
// (but description + calculation present) must trigger metric.help-required
// and exit 1. Annotations are supplied so the bare-metric rules stay quiet
// and the test isolates the help-required signal.
func TestRun_ValidateCatchesMissingHelp(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"

// X counts things.
//
// @metric description Counts the total number of requests served.
// @metric calculation Incremented once per request handler invocation.
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: ""})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.help-required") {
		t.Errorf("stderr missing 'metric.help-required': %q", stderr.String())
	}
}

// TestRun_ValidateCatchesMissingLabelDescription — a *Vec metric with a
// declared label lacking an @label annotation must trigger
// metric.label-description-required and exit 1. description + calculation
// are supplied so only the label rule fires.
func TestRun_ValidateCatchesMissingLabelDescription(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"

// X counts things per method.
//
// @metric description Counts the total number of requests per HTTP method.
// @metric calculation Incremented per request, labelled by method.
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x_total", Help: "x"}, []string{"method"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.label-description-required") {
		t.Errorf("stderr missing 'metric.label-description-required': %q", stderr.String())
	}
}

// TestRun_ValidateCatchesTypeConsistency — two files declaring metrics
// with the same Name but distinct Prometheus types (counter vs gauge)
// must fire BOTH metric.duplicate-name AND metric.type-consistency in
// the same run. The rules are complementary: one reports the name
// collision, the other the type conflict.
func TestRun_ValidateCatchesTypeConsistency(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "a/one.go", `package a
import "github.com/prometheus/client_golang/prometheus"

// A counts things.
//
// @metric description Description for A.
// @metric calculation Incremented on event A.
var A = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "h"})
`)
	writeGoFile(t, root, "b/two.go", `package b
import "github.com/prometheus/client_golang/prometheus"

// B measures things.
//
// @metric description Description for B.
// @metric calculation Set to current value of B.
var B = prometheus.NewGauge(prometheus.GaugeOpts{Name: "x", Help: "h"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.duplicate-name") {
		t.Errorf("stderr missing 'metric.duplicate-name': %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.type-consistency") {
		t.Errorf("stderr missing 'metric.type-consistency': %q", stderr.String())
	}
}

// TestRun_ValidateStrictIsNoOpInV01 — --strict with a bare metric still
// exits 1 (because error-severity rules fire on their own) and must not
// panic. In v0.1 no warning-default rules exist, so --strict is effectively
// a smoke guard that wiring remains sound when it's toggled on.
func TestRun_ValidateStrictIsNoOpInV01(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", bareMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate", "--strict"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (strict must not swallow error-severity violations); stderr=%q", code, stderr.String())
	}
}

// TestRun_ValidateCatchesDuplicateName — two .go files declaring metrics
// with the same Name must trigger metric.duplicate-name with exit 1.
func TestRun_ValidateCatchesDuplicateName(t *testing.T) {
	root := t.TempDir()
	// Two files in different packages so Go tooling is happy, but both
	// register a Prometheus metric named "dup_total" — the snapshot sees
	// two descriptors with the same Name.
	writeGoFile(t, root, "a/one.go", `package a
import "github.com/prometheus/client_golang/prometheus"

// A counts things.
//
// @metric description Description for A.
// @metric calculation Incremented on event A.
var A = prometheus.NewCounter(prometheus.CounterOpts{Name: "dup_total", Help: "hA"})
`)
	writeGoFile(t, root, "b/two.go", `package b
import "github.com/prometheus/client_golang/prometheus"

// B counts things.
//
// @metric description Description for B.
// @metric calculation Incremented on event B.
var B = prometheus.NewCounter(prometheus.CounterOpts{Name: "dup_total", Help: "hB"})
`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.duplicate-name") {
		t.Errorf("stderr missing 'metric.duplicate-name': %q", stderr.String())
	}
}

// TestRun_ValidateSkipRuleSuppressesViolations — a bare metric that would
// normally trigger description-required must pass (exit 0) when the rule
// is explicitly skipped. Proves --skip-rule reaches the engine's Skip set.
//
// Several other rules (calculation-required, description-required) still
// fire on the bare metric, so the test skips both to isolate the signal.
func TestRun_ValidateSkipRuleSuppressesViolations(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", bareMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--skip-rule", "metric.description-required",
		"--skip-rule", "metric.calculation-required",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "metric.description-required") {
		t.Errorf("stderr still mentions skipped rule 'metric.description-required': %q", stderr.String())
	}
	if strings.Contains(stderr.String(), "metric.calculation-required") {
		t.Errorf("stderr still mentions skipped rule 'metric.calculation-required': %q", stderr.String())
	}
}

// TestRun_ValidateWarnRuleDemotesToWarning — --warn-rule demotes an
// error-severity rule to warning. With every default-error rule demoted,
// the violations remain visible on stderr but exit code is 0.
func TestRun_ValidateWarnRuleDemotesToWarning(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", bareMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--warn-rule", "metric.description-required",
		"--warn-rule", "metric.calculation-required",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 after demotion (stderr=%q)", code, stderr.String())
	}
	// Violation text remains — just at [warning] level rather than [error].
	if !strings.Contains(stderr.String(), "metric.description-required") {
		t.Errorf("stderr missing 'metric.description-required' (demoted, not silenced): %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "[warning]") {
		t.Errorf("stderr missing '[warning]' severity marker after --warn-rule: %q", stderr.String())
	}
}

// highCardMetric is a *Vec metric whose label names (user_id) exactly
// match a pattern in the built-in high-cardinality list. Otherwise
// fully annotated so only the high-cardinality-hint rule fires when
// enabled.
const highCardMetric = `package p
import "github.com/prometheus/client_golang/prometheus"

// X counts things per user.
//
// @metric description Counts the total number of requests per user id.
// @metric calculation Incremented once per successful request handler invocation.
// @label user_id Identifier of the requesting user.
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x_total", Help: "x"}, []string{"user_id"})
`

// tenantCardMetric is the override-test fixture: label name is
// "tenant_id" which is NOT in the built-in default list, so only a CLI
// override can surface it.
const tenantCardMetric = `package p
import "github.com/prometheus/client_golang/prometheus"

// X counts things per tenant.
//
// @metric description Counts the total number of requests per tenant id.
// @metric calculation Incremented once per successful request handler invocation.
// @label tenant_id Identifier of the requesting tenant.
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x_total", Help: "x"}, []string{"tenant_id"})
`

// TestRun_HighCardinalityRuleOffByDefault — with --validate but WITHOUT
// --enable-rule, the high-cardinality-hint rule stays silent even when
// the snapshot has a user_id label. Proves the rule is default-off.
func TestRun_HighCardinalityRuleOffByDefault(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", highCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "metric.label-high-cardinality-hint") {
		t.Errorf("stderr unexpectedly mentions off-by-default rule: %q", stderr.String())
	}
}

// TestRun_HighCardinalityRuleEnabled — --enable-rule activates the
// rule; a user_id label fires it. Exit stays 0 (warning-severity).
func TestRun_HighCardinalityRuleEnabled(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", highCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--enable-rule", "metric.label-high-cardinality-hint",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (warning severity); stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.label-high-cardinality-hint") {
		t.Errorf("stderr missing enabled rule: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "user_id") {
		t.Errorf("stderr missing label name 'user_id': %q", stderr.String())
	}
}

// TestRun_HighCardinalityLabelsOverride — user overrides the default
// list via --high-cardinality-labels. tenant_id (NOT in the default
// list) fires, user_id (in the default list) does not.
func TestRun_HighCardinalityLabelsOverride(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", tenantCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--enable-rule", "metric.label-high-cardinality-hint",
		"--high-cardinality-labels", "tenant_id,something_else",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.label-high-cardinality-hint") {
		t.Errorf("stderr missing rule id: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "tenant_id") {
		t.Errorf("stderr missing overridden label 'tenant_id': %q", stderr.String())
	}
}

// trimmedCardMetric exercises whitespace handling in the CSV parser —
// the label name is "trimmed_label" (no spaces), but the CLI fixture
// passes the override with surrounding whitespace on every element.
const trimmedCardMetric = `package p
import "github.com/prometheus/client_golang/prometheus"

// X counts things per trimmed label.
//
// @metric description Counts the total number of requests per trimmed label.
// @metric calculation Incremented once per successful request handler invocation.
// @label trimmed_label Identifier used to exercise CSV whitespace trimming.
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x_total", Help: "x"}, []string{"trimmed_label"})
`

// myLabelCardMetric is used by the empty-elements-dropped test: a
// single "my_label" label that only fires when the override CSV parser
// correctly discards empty entries.
const myLabelCardMetric = `package p
import "github.com/prometheus/client_golang/prometheus"

// X counts things per my_label.
//
// @metric description Counts the total number of requests per my_label.
// @metric calculation Incremented once per successful request handler invocation.
// @label my_label Identifier used to exercise empty-element skipping in CSV.
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x_total", Help: "x"}, []string{"my_label"})
`

// TestRun_HighCardinalityLabelsWhitespaceTrimmed — the CSV parser must
// strip surrounding whitespace on every element, so a flag value like
// "  other_label  ,  trimmed_label  ,  another  " still matches a
// label literally named "trimmed_label". Pins the trim contract at the
// CLI boundary (unit-level trim is covered, but users interact with
// the CLI).
func TestRun_HighCardinalityLabelsWhitespaceTrimmed(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", trimmedCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--enable-rule", "metric.label-high-cardinality-hint",
		"--high-cardinality-labels", "  other_label  ,  trimmed_label  ,  another  ",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.label-high-cardinality-hint") {
		t.Errorf("stderr missing rule id (whitespace-trimmed override should still fire): %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "trimmed_label") {
		t.Errorf("stderr missing label name 'trimmed_label' (trim contract broken?): %q", stderr.String())
	}
}

// TestRun_HighCardinalityLabelsEmptyElementsDropped — a CSV with empty
// entries (leading, trailing, interleaved, whitespace-only) must drop
// those entries without degenerating the whole override to nil. The
// single real entry "my_label" still triggers a violation.
func TestRun_HighCardinalityLabelsEmptyElementsDropped(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", myLabelCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--enable-rule", "metric.label-high-cardinality-hint",
		"--high-cardinality-labels", ",,  ,my_label,,  ,",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.label-high-cardinality-hint") {
		t.Errorf("stderr missing rule id (empty-elements CSV should still carry my_label): %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "my_label") {
		t.Errorf("stderr missing label name 'my_label': %q", stderr.String())
	}
}

// TestRun_HighCardinalityLabelsWithoutEnableWarns — passing
// --high-cardinality-labels without also enabling the rule is a common
// footgun (typo / copy-paste error). The CLI must surface a stderr
// warning pointing at --enable-rule so the user notices before the
// override silently does nothing.
func TestRun_HighCardinalityLabelsWithoutEnableWarns(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", fullyAnnotatedMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--high-cardinality-labels", "foo",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--high-cardinality-labels is set but") {
		t.Errorf("stderr missing footgun warning prefix: %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.label-high-cardinality-hint is off") {
		t.Errorf("stderr missing 'metric.label-high-cardinality-hint is off' phrase: %q", stderr.String())
	}
}

// TestRun_HighCardinalityLabelsEmptyOverride — `--high-cardinality-labels=""`
// is treated as "unset" (empty string leaves the slice at nil), so the
// built-in default list is used and user_id still fires. Pins the CLI
// contract: an empty flag value != an explicit no-patterns signal.
func TestRun_HighCardinalityLabelsEmptyOverride(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", highCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--enable-rule", "metric.label-high-cardinality-hint",
		"--high-cardinality-labels", "",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	// Empty override behaves like "unset" — defaults apply, user_id fires.
	if !strings.Contains(stderr.String(), "metric.label-high-cardinality-hint") {
		t.Errorf("stderr missing rule id (empty override should fall back to defaults): %q", stderr.String())
	}
}

// TestRun_ValidateReport_ContainsViolations — the JSON report file must
// carry the violation records with correct RuleID fields when rules fire.
func TestRun_ValidateReport_ContainsViolations(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", bareMetric)
	reportPath := filepath.Join(root, "report.json")

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--validate",
		"--validation-report", reportPath,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (stderr=%q)", code, stderr.String())
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var rep struct {
		SchemaVersion string `json:"schema_version"`
		Violations    []struct {
			RuleID   string `json:"rule_id"`
			Severity string `json:"severity"`
			Message  string `json:"message"`
		} `json:"violations"`
		ErrorCount   int `json:"error_count"`
		WarningCount int `json:"warning_count"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("report JSON parse: %v (content=%q)", err, string(data))
	}
	if rep.ErrorCount < 2 {
		t.Errorf("error_count: got %d, want >= 2 (description + calculation)", rep.ErrorCount)
	}

	wantIDs := map[string]bool{
		"metric.description-required": false,
		"metric.calculation-required": false,
	}
	for _, v := range rep.Violations {
		if _, ok := wantIDs[v.RuleID]; ok {
			wantIDs[v.RuleID] = true
		}
		if v.Severity != "error" {
			t.Errorf("violation %q: severity=%q, want 'error'", v.RuleID, v.Severity)
		}
	}
	for id, seen := range wantIDs {
		if !seen {
			t.Errorf("report missing violation with rule_id=%q; violations=%+v", id, rep.Violations)
		}
	}
}

// TestRun_ListRulesExit0 — --list-rules prints a non-empty rule table and
// exits 0. Also proves every column header is present, stderr stays empty
// (pure discoverability command — no diagnostics), and at least a handful
// of known rule IDs show up, so a completely empty registry or a silent
// early-return would be caught here.
func TestRun_ListRulesExit0(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	out := stdout.String()
	if out == "" {
		t.Fatalf("stdout is empty; expected rule table")
	}
	for _, header := range []string{"Rule ID", "Severity", "Default", "Description"} {
		if !strings.Contains(out, header) {
			t.Errorf("stdout must contain header %q; stdout:\n%s", header, out)
		}
	}
	for _, id := range []string{
		"metric.name-required",
		"metric.help-required",
		"metric.description-required",
		"metric.duplicate-name",
		"metric.label-high-cardinality-hint",
	} {
		if !strings.Contains(out, id) {
			t.Errorf("stdout missing expected rule id %q: %q", id, out)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("--list-rules must write nothing to stderr; got:\n%s", stderr.String())
	}
}

// TestRun_ListRulesWithoutSource — --list-rules must succeed even when
// --source is not provided. Enumeration is independent of project layout
// (pure registry dump), so the normal "--source is required" guard must
// not fire before --list-rules is checked.
func TestRun_ListRulesWithoutSource(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "--source is required") {
		t.Errorf("stderr unexpectedly emits source-required error: %q", stderr.String())
	}
}

// TestRun_ListRulesShowsAllRegistered — every rule returned by rules.All()
// must appear in the --list-rules output. Pins the registry-to-CLI
// contract: a newly registered rule is automatically visible without
// needing to update the CLI formatter.
func TestRun_ListRulesShowsAllRegistered(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	out := stdout.String()
	for _, r := range rules.All() {
		if !strings.Contains(out, r.ID()) {
			t.Errorf("stdout missing rule id %q: %q", r.ID(), out)
		}
	}
}

// TestRun_ListRulesShowsSeverity — the severity column must render both
// "error" and "warning" strings when the registry includes rules of each
// kind (which it does today: 7 errors + 8 warnings). A broken String()
// method on Severity would collapse both to the same text.
func TestRun_ListRulesShowsSeverity(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "error") {
		t.Errorf("stdout missing 'error' severity: %q", out)
	}
	if !strings.Contains(out, "warning") {
		t.Errorf("stdout missing 'warning' severity: %q", out)
	}
}

// TestRun_ListRulesShowsDefaultOffOn — the Default column must render
// both "on" and "off" values when the registry has at least one default-
// off rule. Guards against the formatter silently emitting only "on" for
// every rule (e.g. if DefaultOffIDs lookup is misapplied).
func TestRun_ListRulesShowsDefaultOffOn(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "on") {
		t.Errorf("stdout missing 'on' default marker: %q", out)
	}
	if !strings.Contains(out, "off") {
		t.Errorf("stdout missing 'off' default marker (expected for high-cardinality-hint): %q", out)
	}
	// Sanity: the off-by-default rule must render with "off", not "on".
	// The line for that rule carries the ID plus "off" somewhere after it.
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "metric.label-high-cardinality-hint") {
			if !strings.Contains(line, "off") {
				t.Errorf("high-cardinality-hint line missing 'off' marker: %q", line)
			}
		}
	}
}

// TestRun_VersionInjectionViaVarRoundTrips — compile-time guard: `version`
// must be a var, not a const, so that `-ldflags "-X main.version=..."` can
// override it at release time. If this file fails to compile after someone
// changes version to a const, that's the regression caught here (the
// assignment below would be a compile error on a const); the round-trip
// through -h also proves the injected value reaches the usage banner.
func TestRun_VersionInjectionViaVarRoundTrips(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = "test-injected"

	var stdout, stderr bytes.Buffer
	code := run([]string{"-h"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "test-injected") {
		t.Errorf("help output missing injected version; stdout:\n%s", stdout.String())
	}
}

// TestRun_ListRulesIgnoresOutputFlag — --list-rules is a terminal
// discoverability command and must short-circuit before any pipeline
// work, including --output file emission. Pairing the two is almost
// always a user mistake; the CLI silently drops --output in that case,
// which this test pins.
func TestRun_ListRulesIgnoresOutputFlag(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "rules.json")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules", "--output", outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit: got %d, want 0; stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("--output file must NOT be created when --list-rules is used; got stat err: %v", err)
	}
	if !strings.Contains(stdout.String(), "Rule ID") {
		t.Errorf("list-rules output missing from stdout")
	}
}

// TestRun_ListRulesIgnoresValidateFlag — --list-rules must ignore
// --validate and --strict entirely (no rule engine invocation, no
// summary noise on stderr). Pins the short-circuit order: list-rules
// fires before any validation wiring.
func TestRun_ListRulesIgnoresValidateFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules", "--validate", "--strict"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit: got %d, want 0; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Rule ID") {
		t.Errorf("list-rules output missing from stdout")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must be empty; got:\n%s", stderr.String())
	}
}

// TestRun_ListRulesShowsFullDescription — the Description column must
// not be truncated. metric.histogram-unit-suffix carries a dynamic
// description built from all 13 recognised unit suffixes; if the
// formatter ever clips the last column (e.g. naive fixed-width
// formatting), the trailing suffixes would disappear from the output.
func TestRun_ListRulesShowsFullDescription(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit: got %d, want 0; stderr=%s", code, stderr.String())
	}
	longMarkers := []string{
		"_seconds", "_milliseconds", "_microseconds", "_nanoseconds",
		"_bytes", "_kilobytes", "_megabytes",
		"_ratio", "_percent", "_fraction",
		"_bits", "_celsius", "_meters",
	}
	out := stdout.String()
	for _, marker := range longMarkers {
		if !strings.Contains(out, marker) {
			t.Errorf("description for histogram rule appears truncated; missing %q", marker)
		}
	}
}

// TestRun_ListRulesRowOrderMatchesRegistry — rows must appear in the
// order produced by rules.All(). A reordering (e.g. accidental map
// iteration) would make the CLI output non-deterministic and break
// downstream diffing; this test pins the registry → stdout ordering.
func TestRun_ListRulesRowOrderMatchesRegistry(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--list-rules"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit: got %d, want 0; stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	lastPos := -1
	for _, r := range rules.All() {
		pos := strings.Index(out, r.ID())
		if pos == -1 {
			t.Errorf("rule %q missing from stdout", r.ID())
			continue
		}
		if pos <= lastPos {
			t.Errorf("rule %q appears out of order (pos %d after %d)", r.ID(), pos, lastPos)
		}
		lastPos = pos
	}
}

// TestRun_ValidationErrorReturnsExit1 pins the exit-1 contract under the
// v0.3.1 taxonomy: a bare metric (missing @metric description + calculation)
// triggers error-severity rules, so the CLI must exit 1 (validation failed),
// NOT exit 3 (tool crashed). The TestRun_ValidateCatches* tests above also
// assert code == 1, but this test exists as the canonical exit-1 pin after
// the taxonomy split so future refactors can grep for it directly.
func TestRun_ValidationErrorReturnsExit1(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", bareMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{"--source", root, "--validate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code: got %d, want 1 (validation failed, not tool crash); stderr=%q", code, stderr.String())
	}
	// Snapshot must still be written — validation failure does not block
	// snapshot emission (the snapshot may well be the diagnostic artifact
	// the CI pipeline uploads alongside the violation report).
	if !strings.Contains(stdout.String(), `"schema_version"`) {
		t.Errorf("stdout missing snapshot JSON (snapshot must be written even when validation fails): %q", stdout.String())
	}
}

// TestRun_PipelineFailureReturnsExit3 pins the exit-3 contract: when the
// pipeline itself cannot run (e.g. --source points at a non-existent path),
// the CLI must exit 3 (tool crashed) rather than exit 1 (validation found
// issues). This is the taxonomy's headline distinction — CI scripts need
// to tell "your code has issues" from "the extractor is broken".
func TestRun_PipelineFailureReturnsExit3(t *testing.T) {
	var stdout, stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "nonexistent-pipeline-path")
	code := run([]string{"--source", missing, "--validate"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit code: got %d, want 3 (pipeline crash, not validation failure); stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error:") {
		t.Errorf("stderr missing 'error:' prefix: %q", stderr.String())
	}
}

// failingWriter is an io.Writer whose Write always fails. Used to exercise
// error branches that kick in when the CLI cannot flush its own output —
// the only reliable way to reach those branches in-process.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errors.New("broken pipe") }

// TestRun_ListRulesWriteErrorReturnsExit3 — if printRuleList cannot write
// to stdout (e.g. the pipe reader closed early), the CLI must treat that
// as a tool-crash (exit 3), NOT as success or a usage error. Pins the
// error branch inside the --list-rules short-circuit.
func TestRun_ListRulesWriteErrorReturnsExit3(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"--list-rules"}, failingWriter{}, &stderr)
	if code != 3 {
		t.Fatalf("exit: got %d, want 3 (printRuleList I/O failure is a tool-crash); stderr=%q",
			code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "failed to print rule list") {
		t.Errorf("stderr missing 'failed to print rule list': %q", stderr.String())
	}
}

// TestRun_StrictDoesNotAffectWithoutEnable — --strict with --validate but
// without --enable-rule for an off-by-default warning rule must not change
// the exit code. The off-by-default rule stays dormant, so no violations
// exist to promote, so exit is 0. Guards the "strict only promotes
// enabled warnings" semantics at the CLI boundary.
func TestRun_StrictDoesNotAffectWithoutEnable(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", highCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--repo-root", root,
		"--validate",
		"--strict",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit: got %d, want 0 (no warning rules enabled → no violations to promote); stderr=%s",
			code, stderr.String())
	}
}

// TestRun_StrictPromotesWarningToExit1 — --strict must promote a warning-
// severity violation to an error and drive exit 1. The high-cardinality
// rule is enabled so it actually fires; without --strict it would be a
// warning and exit would stay 0. This test is the canonical CLI-level
// pin for the --strict promotion contract.
func TestRun_StrictPromotesWarningToExit1(t *testing.T) {
	root := t.TempDir()
	writeGoFile(t, root, "m.go", highCardMetric)

	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--source", root,
		"--repo-root", root,
		"--validate",
		"--strict",
		"--enable-rule", "metric.label-high-cardinality-hint",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit: got %d, want 1 (--strict should promote high-cardinality warning to error); stderr=%s",
			code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "metric.label-high-cardinality-hint") {
		t.Errorf("stderr missing rule ID: %s", stderr.String())
	}
}
