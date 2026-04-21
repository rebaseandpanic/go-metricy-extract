package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
)

// writeFile writes content to tmpdir/relpath, creating intermediate
// directories as needed. It fails the test on any filesystem error.
func writeFile(t *testing.T, root, relpath, content string) string {
	t.Helper()
	full := filepath.Join(root, relpath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

// fixedClock returns a Now() func that always yields t. Lets tests pin
// ExtractedAt to a known value without touching the real clock.
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestRun_SingleCounter(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "metrics.go", `package p

import "github.com/prometheus/client_golang/prometheus"

// Http requests counter.
// @metric description Total HTTP requests handled by the service.
// @metric calculation Incremented in the logging middleware per request.
var HttpRequests = prometheus.NewCounter(prometheus.CounterOpts{
    Name: "http_requests_total",
    Help: "Total HTTP requests",
})
`)

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := len(res.Snapshot.Metrics); got != 1 {
		t.Fatalf("metrics count: got %d, want 1 (warnings=%v)", got, res.Warnings)
	}
	m := res.Snapshot.Metrics[0]
	if m.Name != "http_requests_total" {
		t.Errorf("Name: got %q, want %q", m.Name, "http_requests_total")
	}
	if m.Type != "counter" {
		t.Errorf("Type: got %q, want %q", m.Type, "counter")
	}
	if m.Help != "Total HTTP requests" {
		t.Errorf("Help: got %q, want %q", m.Help, "Total HTTP requests")
	}
	if m.Description == nil || *m.Description != "Total HTTP requests handled by the service." {
		t.Errorf("Description: got %v, want %q", m.Description, "Total HTTP requests handled by the service.")
	}
	if m.Calculation == nil || *m.Calculation != "Incremented in the logging middleware per request." {
		t.Errorf("Calculation: got %v, want %q", m.Calculation, "Incremented in the logging middleware per request.")
	}
}

func TestRun_MultipleFilesMergedAndSorted(t *testing.T) {
	root := t.TempDir()
	// Files deliberately ordered so the filesystem-order listing does not
	// happen to match the alphabetical metric ordering. "a.go" declares
	// "zzz_total"; "b.go" declares "aaa_total"; the pipeline must reorder.
	writeFile(t, root, "a.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var Z = prometheus.NewCounter(prometheus.CounterOpts{Name: "zzz_total", Help: "z"})
`)
	writeFile(t, root, "b.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var A = prometheus.NewCounter(prometheus.CounterOpts{Name: "aaa_total", Help: "a"})
var M = prometheus.NewCounter(prometheus.CounterOpts{Name: "mmm_total", Help: "m"})
`)

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := make([]string, len(res.Snapshot.Metrics))
	for i, m := range res.Snapshot.Metrics {
		got[i] = m.Name
	}
	want := []string{"aaa_total", "mmm_total", "zzz_total"}
	if !equalStrings(got, want) {
		t.Errorf("metric order: got %v, want %v", got, want)
	}
}

func TestRun_ProjectNameDefaultsToSourceBasename(t *testing.T) {
	root := t.TempDir()
	svcDir := filepath.Join(root, "myservice")
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, svcDir, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`)

	res, err := Run(context.Background(), Options{Source: svcDir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Snapshot.Project != "myservice" {
		t.Errorf("Project: got %q, want %q", res.Snapshot.Project, "myservice")
	}
}

func TestRun_ProjectNameExplicit(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`)

	res, err := Run(context.Background(), Options{Source: root, Project: "custom-name"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Snapshot.Project != "custom-name" {
		t.Errorf("Project: got %q, want %q", res.Snapshot.Project, "custom-name")
	}
}

func TestRun_ExtractedAtUsesClockOverride(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`)

	fixed := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	res, err := Run(context.Background(), Options{Source: root, Now: fixedClock(fixed)})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Snapshot.ExtractedAt.Equal(fixed) {
		t.Errorf("ExtractedAt: got %s, want %s", res.Snapshot.ExtractedAt, fixed)
	}
	if res.Snapshot.ExtractedAt.Location() != time.UTC {
		t.Errorf("ExtractedAt location: got %s, want UTC", res.Snapshot.ExtractedAt.Location())
	}
}

func TestRun_ExtractorInfoPopulated(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "m.go", `package p`)

	res, err := Run(context.Background(), Options{Source: root, Version: "1.2.3"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Snapshot.Extractor.Name != model.ExtractorName {
		t.Errorf("Extractor.Name: got %q, want %q", res.Snapshot.Extractor.Name, model.ExtractorName)
	}
	if res.Snapshot.Extractor.Version != "1.2.3" {
		t.Errorf("Extractor.Version: got %q, want %q", res.Snapshot.Extractor.Version, "1.2.3")
	}
	if res.Snapshot.SchemaVersion != model.SchemaVersion {
		t.Errorf("SchemaVersion: got %q, want %q", res.Snapshot.SchemaVersion, model.SchemaVersion)
	}
}

func TestRun_MissingSourceDirReturnsError(t *testing.T) {
	// Build a path under TempDir whose parent does not exist; portable across
	// OSes and auto-cleaned by the test harness.
	missing := filepath.Join(t.TempDir(), "does-not-exist", "path")
	res, err := Run(context.Background(), Options{Source: missing})
	if err == nil {
		t.Fatalf("Run: expected error for missing source dir, got nil")
	}
	if res != nil {
		t.Errorf("Run: expected nil result on error, got %+v", res)
	}
}

func TestRun_NoGoFilesReturnsEmptyMetrics(t *testing.T) {
	root := t.TempDir()
	// No .go files at all.

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Snapshot.Metrics == nil {
		t.Errorf("Metrics: got nil, want empty slice (non-nil)")
	}
	if len(res.Snapshot.Metrics) != 0 {
		t.Errorf("Metrics: got %d entries, want 0", len(res.Snapshot.Metrics))
	}
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings: got %d, want 0 (warnings=%v)", len(res.Warnings), res.Warnings)
	}
}

func TestRun_ParseErrorGoesToWarningsNotFatal(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "broken.go", `package main
func{
`)
	writeFile(t, root, "ok.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "ok_total", Help: "ok"})
`)

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: fatal error, want non-fatal: %v", err)
	}
	// At least one warning should reference "broken.go: parse error".
	sawParseWarn := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "broken.go") && strings.Contains(w, "parse error") {
			sawParseWarn = true
			break
		}
	}
	if !sawParseWarn {
		t.Errorf("expected parse-error warning for broken.go; got %v", res.Warnings)
	}
	// Valid file's metric must still appear.
	if len(res.Snapshot.Metrics) != 1 || res.Snapshot.Metrics[0].Name != "ok_total" {
		t.Errorf("metrics: got %+v, want [ok_total]", res.Snapshot.Metrics)
	}
	// S9: parse-error warnings must also be mirrored onto Snapshot.ExtractionWarnings
	// so validation rules that look at diagnostics see the same set the caller does.
	if !reflect.DeepEqual(res.Snapshot.ExtractionWarnings, res.Warnings) {
		t.Errorf("Snapshot.ExtractionWarnings must mirror Result.Warnings exactly;\n got: %v\nwant: %v",
			res.Snapshot.ExtractionWarnings, res.Warnings)
	}
}

// TestRun_ExtractionWarningsNilOnCleanFixture pins the contract that a
// fully-annotated, well-formed fixture produces no extractor warnings and
// therefore leaves Snapshot.ExtractionWarnings empty. Guards against a
// regression where the pipeline injects stray diagnostics on clean input.
func TestRun_ExtractionWarningsNilOnCleanFixture(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "metrics.go", `package p

import "github.com/prometheus/client_golang/prometheus"

// Clean, fully-literal metric — no warnings expected.
// @metric description A fully-annotated counter.
// @metric calculation Incremented in the test harness.
var HttpRequests = prometheus.NewCounter(prometheus.CounterOpts{
    Name: "http_requests_total",
    Help: "Total HTTP requests",
})
`)

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("Result.Warnings: got %v, want empty on clean fixture", res.Warnings)
	}
	if len(res.Snapshot.ExtractionWarnings) != 0 {
		t.Errorf("Snapshot.ExtractionWarnings: got %v, want empty on clean fixture",
			res.Snapshot.ExtractionWarnings)
	}
}

func TestRun_SourceLocationFileIsRepoRelative(t *testing.T) {
	outer := t.TempDir()
	// Build outer/repo/.git + outer/repo/pkg/metrics.go so ResolveRepoRoot
	// walks up from pkg/ and lands on repo/ (the .git marker).
	repo := filepath.Join(outer, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	pkgDir := filepath.Join(repo, "pkg")
	writeFile(t, pkgDir, "metrics.go", `package pkg
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`)

	// Resolve symlinks so macOS/WSL private-prefix paths don't defeat
	// filepath.Rel comparisons inside MakeRelative.
	resolvedPkg, err := filepath.EvalSymlinks(pkgDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	res, err := Run(context.Background(), Options{Source: resolvedPkg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Snapshot.Metrics) != 1 {
		t.Fatalf("metrics: got %d, want 1 (warnings=%v)", len(res.Snapshot.Metrics), res.Warnings)
	}
	sl := res.Snapshot.Metrics[0].SourceLocation
	if sl == nil {
		t.Fatal("SourceLocation: got nil, want populated")
	}
	if sl.File != "pkg/metrics.go" {
		t.Errorf("SourceLocation.File: got %q, want %q", sl.File, "pkg/metrics.go")
	}
}

func TestRun_RepoRootExplicitOverride(t *testing.T) {
	outer := t.TempDir()
	// Build outer/repo/.git (auto-detect would land on repo) plus
	// outer/repo/pkg/metrics.go. Then pass RepoRoot=outer/repo/pkg so the
	// emitted path is relative to pkg/, not repo/ — proving the override won.
	repo := filepath.Join(outer, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	pkgDir := filepath.Join(repo, "pkg")
	writeFile(t, pkgDir, "metrics.go", `package pkg
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x_total", Help: "x"})
`)

	resolvedPkg, err := filepath.EvalSymlinks(pkgDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	res, err := Run(context.Background(), Options{Source: resolvedPkg, RepoRoot: resolvedPkg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	sl := res.Snapshot.Metrics[0].SourceLocation
	if sl == nil {
		t.Fatal("SourceLocation: nil")
	}
	if sl.File != "metrics.go" {
		t.Errorf("SourceLocation.File: got %q, want %q (relative to explicit RepoRoot)", sl.File, "metrics.go")
	}
}

func TestRun_LabelsSortedAlphabetically(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "x_total", Help: "x"}, []string{"z", "a", "m"})
`)

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Snapshot.Metrics) != 1 {
		t.Fatalf("metrics: got %d, want 1", len(res.Snapshot.Metrics))
	}
	labels := res.Snapshot.Metrics[0].Labels
	got := make([]string, len(labels))
	for i, l := range labels {
		got[i] = l.Name
	}
	want := []string{"a", "m", "z"}
	if !equalStrings(got, want) {
		t.Errorf("label order: got %v, want %v", got, want)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "x", Help: "y"})
`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before calling Run

	res, err := Run(ctx, Options{Source: root})
	if err == nil {
		t.Fatalf("Run: expected error from cancelled context, got nil result=%+v", res)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error: got %v, want context.Canceled", err)
	}
}

func TestRun_WarningsFromExtractorPropagated(t *testing.T) {
	root := t.TempDir()
	// Non-literal Name triggers "<var>: non-literal Name; skipping metric".
	writeFile(t, root, "m.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var dynName = "foo_total"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: dynName, Help: "y"})
`)

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Extractor emits a warning prefixed with the var name "X".
	sawWarn := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "X:") && strings.Contains(w, "non-literal Name") {
			sawWarn = true
			break
		}
	}
	if !sawWarn {
		t.Errorf("expected 'X: non-literal Name' warning; got %v", res.Warnings)
	}
	// Metric was skipped, so no metrics in the snapshot.
	if len(res.Snapshot.Metrics) != 0 {
		t.Errorf("metrics: got %d, want 0 (skipped)", len(res.Snapshot.Metrics))
	}
	// v0.2 stage 1: warnings must also be mirrored onto the snapshot so
	// the metric.non-literal-metadata validation rule can surface them.
	// The snapshot field is the pipeline → validation bridge; without
	// this propagation the rule sees an empty list and silently
	// produces no violations.
	if !reflect.DeepEqual(res.Snapshot.ExtractionWarnings, res.Warnings) {
		t.Errorf("Snapshot.ExtractionWarnings must mirror Result.Warnings:\n got: %v\nwant: %v",
			res.Snapshot.ExtractionWarnings, res.Warnings)
	}
}

func TestRun_EmptySourceReturnsError(t *testing.T) {
	res, err := Run(context.Background(), Options{Source: ""})
	if err == nil {
		t.Fatalf("Run: expected error for empty Source, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error message: got %q, want text containing 'required'", err.Error())
	}
	if res != nil {
		t.Errorf("Result: got %+v, want nil on error", res)
	}
}

func TestRun_SourceIsFileReturnsError(t *testing.T) {
	root := t.TempDir()
	// Write a plain file, then pass its path (not the dir) as Source.
	filePath := writeFile(t, root, "file.go", `package p`)

	res, err := Run(context.Background(), Options{Source: filePath})
	if err == nil {
		t.Fatalf("Run: expected error when Source is a file, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error message: got %q, want text containing 'not a directory'", err.Error())
	}
	if res != nil {
		t.Errorf("Result: got %+v, want nil on error", res)
	}
}

func TestRun_MultipleParseErrorsAggregated(t *testing.T) {
	root := t.TempDir()
	// Two broken files with deliberately ordered names so the deterministic
	// sort of filepath.WalkDir yields a_broken.go before b_broken.go.
	writeFile(t, root, "a_broken.go", `package main
func{
`)
	writeFile(t, root, "b_broken.go", `package main
func{
`)
	writeFile(t, root, "ok.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var X = prometheus.NewCounter(prometheus.CounterOpts{Name: "ok_total", Help: "ok"})
`)

	res, err := Run(context.Background(), Options{Source: root})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Snapshot.Metrics) != 1 {
		t.Fatalf("metrics: got %d, want 1", len(res.Snapshot.Metrics))
	}

	// Collect the parse-error warnings in encounter order.
	var parseWarns []string
	for _, w := range res.Warnings {
		if strings.Contains(w, "parse error") {
			parseWarns = append(parseWarns, w)
		}
	}
	if len(parseWarns) != 2 {
		t.Fatalf("parse-error warnings: got %d, want 2 (all warnings=%v)", len(parseWarns), res.Warnings)
	}
	// Both names must appear.
	if !strings.Contains(parseWarns[0], "a_broken.go") {
		t.Errorf("warning 0: got %q, want to contain a_broken.go", parseWarns[0])
	}
	if !strings.Contains(parseWarns[1], "b_broken.go") {
		t.Errorf("warning 1: got %q, want to contain b_broken.go", parseWarns[1])
	}
}

func TestRun_DeterministicBytes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.go", `package p
import "github.com/prometheus/client_golang/prometheus"
var A = prometheus.NewCounter(prometheus.CounterOpts{Name: "aaa_total", Help: "a"})
var Z = prometheus.NewCounter(prometheus.CounterOpts{Name: "zzz_total", Help: "z"})
`)

	fixed := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	opts := Options{Source: root, Now: fixedClock(fixed), Project: "det", Version: "1"}

	res1, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run #1: %v", err)
	}
	res2, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run #2: %v", err)
	}

	b1, err := json.MarshalIndent(res1.Snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal #1: %v", err)
	}
	b2, err := json.MarshalIndent(res2.Snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal #2: %v", err)
	}
	if !bytes.Equal(b1, b2) {
		t.Errorf("deterministic bytes mismatch:\n#1: %s\n#2: %s", b1, b2)
	}
}

func TestRun_ParseErrorWarningUsesRepoRelativePath(t *testing.T) {
	outer := t.TempDir()
	// Repo marker so ResolveRepoRoot lands on <outer>/repo.
	repo := filepath.Join(outer, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	pkgDir := filepath.Join(repo, "pkg")
	writeFile(t, pkgDir, "broken.go", `package main
func{
`)
	resolvedPkg, err := filepath.EvalSymlinks(pkgDir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	res, err := Run(context.Background(), Options{Source: resolvedPkg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("warnings: got 0, want at least 1")
	}
	for _, w := range res.Warnings {
		if strings.HasPrefix(w, "/") {
			t.Errorf("warning starts with absolute path (want repo-relative): %q", w)
		}
	}
}

// equalStrings compares two string slices element-wise. Declared here rather
// than relying on go-cmp to keep the pipeline tests free of an extra
// dependency for trivial ordering checks.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

