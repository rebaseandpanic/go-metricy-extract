// Package pipeline wires the extractor, annotation parser, and directory
// walker into a single entry point that produces a [model.MetricSnapshot].
//
// Run walks a source tree, feeds each .go file into the extractor, merges
// the results, sorts them deterministically, and stamps the snapshot with
// the schema version, project name, and extractor identity. Per-file parse
// errors are non-fatal and surface as warnings; only structural problems
// (missing source directory, walk failure, cancelled context) abort the run.
package pipeline

import (
	"context"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"time"

	"github.com/rebaseandpanic/go-metricy-extract/internal/extractor"
	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/sourceloc"
)

// Options controls the extraction pipeline.
type Options struct {
	// Source is the directory to scan. Required; must exist.
	Source string
	// Project is the project name written into the snapshot. When empty,
	// defaults to the basename of the absolute form of Source.
	Project string
	// RepoRoot is the repository root used to compute repo-relative source
	// paths. When empty, the pipeline auto-detects via
	// [sourceloc.ResolveRepoRoot].
	RepoRoot string
	// Now overrides the clock used to stamp ExtractedAt. When nil,
	// [time.Now] is used.
	Now func() time.Time
	// Version is the extractor version written into the snapshot's extractor
	// block. Empty is allowed.
	Version string
}

// Result bundles the snapshot produced by [Run] with any non-fatal
// diagnostics collected along the way.
type Result struct {
	// Snapshot is the assembled, sorted [model.MetricSnapshot].
	Snapshot *model.MetricSnapshot
	// Warnings aggregates per-file parse failures (prefixed with the file's
	// path) and extractor warnings (annotation issues, non-literal metric
	// metadata, malformed labels, etc.).
	Warnings []string
}

// Run walks opts.Source, extracts metrics from every eligible .go file,
// merges and sorts the results deterministically, and returns a Result.
//
// Fatal errors (nil Result, non-nil error):
//   - opts.Source is empty or does not exist
//   - the directory walk itself fails (permission-denied on the Source root
//     itself is fatal; the walker skips permission-denied errors on nested
//     subdirectories via filepath.SkipDir)
//   - ctx is already cancelled at entry
//
// Per-file parse errors are non-fatal: the offending file is skipped and a
// warning of the form "<path>: parse error: <msg>" is appended to the
// result.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Source == "" {
		return nil, fmt.Errorf("source directory is required")
	}
	info, err := os.Stat(opts.Source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("source directory does not exist: %s", opts.Source)
		}
		return nil, fmt.Errorf("stat source directory %s: %w", opts.Source, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source is not a directory: %s", opts.Source)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		repoRoot = sourceloc.ResolveRepoRoot(opts.Source)
	}

	projectName := opts.Project
	if projectName == "" {
		// Resolve to absolute form first so relative inputs like "." or
		// "./svc/" don't collapse to "." or "" via filepath.Base.
		abs, absErr := filepath.Abs(opts.Source)
		if absErr != nil {
			abs = opts.Source
		}
		projectName = filepath.Base(abs)
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}

	files, err := sourceloc.WalkGoFiles(opts.Source)
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", opts.Source, err)
	}

	fset := token.NewFileSet()
	var (
		metrics  []model.MetricDescriptor
		warnings []string
	)

	extractOpts := extractor.ExtractOptions{RepoRoot: repoRoot}

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		res, parseErr := extractor.ExtractSourceWithOptions(fset, file, nil, extractOpts)
		if parseErr != nil {
			// Per-file parse errors are non-fatal. Report with a repo-relative
			// path when available for consistency with SourceLocation.File.
			displayPath := sourceloc.MakeRelative(file, repoRoot)
			warnings = append(warnings, fmt.Sprintf("%s: parse error: %s", displayPath, parseErr))
			continue
		}
		if res == nil {
			continue
		}
		metrics = append(metrics, res.Metrics...)
		warnings = append(warnings, res.Warnings...)
	}

	// Deterministic output: sort metrics by Name and each metric's labels
	// by Name. The extractor preserves source order for labels, so the
	// pipeline is the single point where lexicographic ordering is enforced.
	model.SortMetrics(metrics)
	for i := range metrics {
		model.SortLabels(metrics[i].Labels)
	}

	// Normalize nil -> empty slice so the in-memory snapshot matches the
	// JSON shape ("metrics":[] rather than "metrics":null). MarshalJSON on
	// MetricSnapshot already coerces this for serialization, but keeping the
	// in-memory field non-nil lets callers (tests, downstream code) rely on
	// the same invariant without going through json.
	if metrics == nil {
		metrics = []model.MetricDescriptor{}
	}

	snapshot := &model.MetricSnapshot{
		SchemaVersion: model.SchemaVersion,
		Project:       projectName,
		ExtractedAt:   now().UTC(),
		Extractor: model.ExtractorInfo{
			Name:    model.ExtractorName,
			Version: opts.Version,
		},
		Metrics: metrics,
	}

	return &Result{
		Snapshot: snapshot,
		Warnings: warnings,
	}, nil
}
