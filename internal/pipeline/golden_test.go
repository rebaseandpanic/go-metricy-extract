// Golden-file tests for the full extraction + validation pipeline.
//
// These tests pin the JSON output contract against a real, hand-written Go
// service living under testdata/sample-service. They catch silent drift in
// JSON shape, deterministic sort order, ExtractedAt formatting, source-
// location resolution, and validation-report structure. A passing
// pipeline_test.go covers each invariant on synthetic inputs; this file
// covers them end-to-end on a realistic fixture.
//
// To regenerate golden files after intentional output changes:
//
//	UPDATE_GOLDEN=1 go test ./internal/pipeline/... -run Golden
//
// After regeneration, MANUALLY INSPECT the updated golden files — a green
// test on a wrong golden is vacuous.
package pipeline_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/rebaseandpanic/go-metricy-extract/internal/pipeline"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation/rules"
)

// updateGolden is toggled via env var UPDATE_GOLDEN=1 to rewrite the
// on-disk golden files instead of comparing against them. Keep it off in
// CI: the default path is the assertion path.
var updateGolden = os.Getenv("UPDATE_GOLDEN") == "1"

// fixedExtractedAt pins the snapshot's ExtractedAt timestamp across runs so
// the golden file stays byte-stable. Any second-precision UTC value works —
// this one is the session date.
var fixedExtractedAt = time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC)

// sampleServiceDir returns the absolute path to the sample-service fixture
// relative to the test binary's working directory. Go runs tests from the
// package directory, so we walk two levels up from internal/pipeline/ to
// reach the repo root, then descend into testdata/.
func sampleServiceDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "sample-service")
}

// brokenServiceDir returns the absolute path to the sample-service-broken
// fixture. Companion to sampleServiceDir; see its docs for the path shape.
func brokenServiceDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "sample-service-broken")
}

// goldenPath returns the absolute path to a named golden file.
func goldenPath(name string) string {
	return filepath.Join("..", "..", "testdata", "golden", name)
}

// runSampleService executes the extraction pipeline against the clean
// fixture with deterministic inputs (fixed clock, explicit RepoRoot, pinned
// Version). Returns the pipeline result; t.Fatal on error.
func runSampleService(t *testing.T) *pipeline.Result {
	t.Helper()
	return runFixture(t, sampleServiceDir(t), "sample-service")
}

// runBrokenService mirrors runSampleService but targets the broken fixture,
// which is authored to trip a known set of validation rules.
func runBrokenService(t *testing.T) *pipeline.Result {
	t.Helper()
	return runFixture(t, brokenServiceDir(t), "sample-service-broken")
}

// runFixture is the shared extraction driver for fixture-backed golden
// tests. It pins Now, Project, Version, and RepoRoot so snapshots stay
// byte-stable across invocations and fixture source_location paths are
// emitted relative to the fixture itself (not the outer repo).
func runFixture(t *testing.T, srcDir, project string) *pipeline.Result {
	t.Helper()
	res, err := pipeline.Run(context.Background(), pipeline.Options{
		Source:  srcDir,
		Project: project,
		// RepoRoot: pin to srcDir so source_location paths are fixture-relative
		// (main.go, middleware/metrics.go). Without this the auto-detect would
		// walk up to the project's own .git and emit paths like
		// "testdata/sample-service/main.go", coupling the golden to repo layout.
		RepoRoot: srcDir,
		Now:      func() time.Time { return fixedExtractedAt },
		Version:  "0.1.0-dev",
	})
	if err != nil {
		t.Fatalf("pipeline.Run(%s): %v", srcDir, err)
	}
	return res
}

// readGolden loads a golden file, giving a specific hint when the file is
// missing (first-time generation) versus some other I/O failure. Missing-
// file hint is keyed on os.IsNotExist — permission errors etc. fall through
// to the generic path so the user sees the real cause.
func readGolden(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("golden missing: %s (run with UPDATE_GOLDEN=1 to create)", path)
		}
		t.Fatalf("read golden %s: %v", path, err)
	}
	return data
}

// compareGolden is the shared body of a golden-file assertion: write-or-
// compare, with a diff message that includes (a) the regenerate command and
// (b) the path to a dump of the `got` payload so an external tool
// (e.g. `diff`, `vimdiff`) can be pointed at the on-disk bytes. Put in one
// place so every golden test emits the same failure UX.
func compareGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		t.Logf("updated golden: %s", path)
		return
	}
	want := readGolden(t, path)
	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		dump := filepath.Join(t.TempDir(), filepath.Base(path)+".got.json")
		_ = os.WriteFile(dump, got, 0o644)
		t.Errorf("golden drift for %s (-want +got):\n%s\n\n"+
			"got dumped to %s\n"+
			"If intentional, regenerate:\n\tUPDATE_GOLDEN=1 go test ./internal/pipeline/... -run Golden",
			path, diff, dump)
	}
}

// TestGolden_SnapshotMatchesFixture is the primary contract test: extract
// from sample-service, marshal the snapshot as indented JSON, and compare
// byte-for-byte against testdata/golden/snapshot.json. Any drift — new
// field, reordered metric, changed timestamp format — produces a diff.
//
// The clean fixture is authored to produce zero extractor warnings; we
// assert that explicitly so a regression (new spurious warning) fails here
// rather than surfacing only in the stderr of production runs.
func TestGolden_SnapshotMatchesFixture(t *testing.T) {
	res := runSampleService(t)

	if len(res.Warnings) != 0 {
		t.Errorf("clean fixture should produce zero warnings; got %d:\n%s",
			len(res.Warnings), strings.Join(res.Warnings, "\n"))
	}

	got, err := json.MarshalIndent(res.Snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	got = append(got, '\n')

	compareGolden(t, goldenPath("snapshot.json"), got)
}

// TestGolden_ValidationReportMatchesFixture runs the full set of built-in
// rules against the sample-service snapshot and pins the resulting report
// shape. The fixture is authored to have zero violations, so this file
// locks in the canonical "clean report" output — future rule additions
// that accidentally trip on well-formed input will show up as a diff.
func TestGolden_ValidationReportMatchesFixture(t *testing.T) {
	res := runSampleService(t)

	// Guard against a registry that's been accidentally truncated — without
	// this, a rules.All() that returned [] would trivially pass the "zero
	// violations" expectation and silently make the golden vacuous.
	if len(rules.All()) < 7 {
		t.Fatalf("registry empty or truncated: got %d rules, want >= 7", len(rules.All()))
	}

	valRes := validation.Run(res.Snapshot, validation.Options{
		Rules: rules.All(),
	})

	var buf bytes.Buffer
	if err := validation.WriteReport(&buf, valRes); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	compareGolden(t, goldenPath("validation_report.json"), buf.Bytes())
}

// TestGolden_BrokenFixtureValidationReport pins the wire format of a
// non-empty validation report. The broken fixture is authored to trip a
// deterministic, small set of rules (description/calculation missing on a
// bare counter; name/type conflict on a deliberately-duplicated name), so
// any new rule that accidentally fires on this shape — or any regression
// in violation JSON structure, severity serialization, or location
// enrichment — surfaces as a diff here.
func TestGolden_BrokenFixtureValidationReport(t *testing.T) {
	res := runBrokenService(t)

	if len(rules.All()) < 7 {
		t.Fatalf("registry empty or truncated: got %d rules, want >= 7", len(rules.All()))
	}

	valRes := validation.Run(res.Snapshot, validation.Options{
		Rules: rules.All(),
	})

	var buf bytes.Buffer
	if err := validation.WriteReport(&buf, valRes); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	compareGolden(t, goldenPath("validation_report_broken.json"), buf.Bytes())
}

// TestGolden_VendorMetricsSkipped is a positive sanity check on the
// walker's vendor/ skip rule. The fixture contains a fully-annotated metric
// at testdata/sample-service/vendor/foo/skip.go named "vendor_metric_bug";
// if it ever appears in the snapshot, the skip rule has regressed.
//
// This test is decoupled from the snapshot golden so a single failure here
// points directly at the walker rather than being one line of a larger
// diff.
func TestGolden_VendorMetricsSkipped(t *testing.T) {
	res := runSampleService(t)

	// Canary-check: the assertion below is "none of the metrics have this
	// name." If the walker returned zero metrics entirely — e.g. because a
	// regression broke the whole walker and the fixture happens to produce
	// nothing — the loop would trivially pass. Guard against that here.
	if len(res.Snapshot.Metrics) == 0 {
		t.Fatalf("walker returned zero metrics; canary is moot — investigate walker regression")
	}

	for _, m := range res.Snapshot.Metrics {
		if m.Name == "vendor_metric_bug" {
			t.Fatalf("vendor metric leaked into snapshot: %q (walker's vendor/ skip regressed)", m.Name)
		}
	}
}

// TestGolden_PromautoWithRegSkipped is the chain-form counterpart to
// TestGolden_VendorMetricsSkipped. The extractor deliberately does not
// support `promauto.With(reg).NewX(...)` (the selector's receiver is a
// CallExpr, not a bare Ident — see internal/extractor/extractor.go).
// Today's contract is "silent skip"; this test pins that contract so a
// future change that wires up chain-form extraction has to consciously
// regenerate the golden alongside updating this test.
func TestGolden_PromautoWithRegSkipped(t *testing.T) {
	res := runSampleService(t)

	if len(res.Snapshot.Metrics) == 0 {
		t.Fatalf("walker returned zero metrics; canary is moot — investigate walker regression")
	}

	for _, m := range res.Snapshot.Metrics {
		if m.Name == "should_not_appear_in_snapshot_with_reg" {
			t.Errorf("promauto.With(reg) metric leaked into snapshot: %+v", m)
		}
	}
}

// TestGolden_DeterministicAcrossRuns verifies that two back-to-back runs
// with identical inputs produce byte-identical JSON. This is a stronger
// check than the snapshot golden (which only covers one run) — it rules
// out non-determinism that could happen to match the golden on the first
// try but drift on retry (map-iteration order, unstable sorts, etc.).
func TestGolden_DeterministicAcrossRuns(t *testing.T) {
	res1 := runSampleService(t)
	res2 := runSampleService(t)

	b1, err := json.MarshalIndent(res1.Snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal #1: %v", err)
	}
	b2, err := json.MarshalIndent(res2.Snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal #2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("non-deterministic output across runs:\n#1:\n%s\n#2:\n%s", b1, b2)
	}
}
