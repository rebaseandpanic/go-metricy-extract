# Changelog

All notable changes to this project will be documented in this file.

## [0.3.0] - 2026-04-21

- [FEATURE] Extractor now resolves `promauto.With(registry).NewX(...)` chained form; previously silently skipped. All 8 metric factories × both direct and chained receivers now extract identically
- [FEATURE] Extractor resolves single-level local `var labels = []string{...}` references as the labels argument of Vec constructors. Supports package-level vars in the same file; multi-name pairwise specs work. Function-local, alias-typed (`type MyLabels []string`), two-level chains (`var b = a`), and cross-file/package vars remain fallback (warning + no labels)
- [FEATURE] Validation report schema bumped to "1.1" (additive): new `generated_at` ISO-8601 timestamp field and `by_rule[]` array with per-rule violation counts (rule_id, severity, error_count, warning_count). Existing consumers that ignore unknown JSON keys continue to work unchanged
- [UX] Multi-line `@metric description` / `@metric calculation` / `@label` continuation lines now emit a warning ("possible multi-line continuation after <directive>; only the first line is captured") instead of silently dropping the continuation. Blank lines reset the tracker, so mixed prose + directives stay warning-free
- [UX] Normalized leading `///` in doc comments — triple-slash lines treated as blank, no false-positive continuation warnings
- [ARCHITECTURE] `model.ExtractedAtLayout` exported for reuse across snapshot and validation report timestamps (single source of truth for `"2006-01-02T15:04:05Z"`)
- [ARCHITECTURE] `validation.WriteReport(w, res, now)` now takes a clock function for deterministic `generated_at` timestamps in tests; nil clock falls back to `time.Now`
- [BUGFIX] Sample service fixture canary metric (`chained_promauto_canary_total`) now demonstrates the supported `promauto.With(...)` form and appears in the snapshot; previously pinned as silently skipped

## [0.2.0] - 2026-04-21

- [FEATURE] Eight new validation rules bringing total to 15: four naming/convention checks (`metric.counter-total-suffix`, `metric.histogram-unit-suffix`, `metric.name-snake-case`, `metric.non-literal-metadata`), three min-length checks (`metric.description-min-length`, `metric.calculation-min-length`, `metric.label-description-min-length`), and one off-by-default high-cardinality hint (`metric.label-high-cardinality-hint`)
- [FEATURE] `--list-rules` flag prints all registered validation rules with ID, severity, default on/off state, and description; exits 0 without requiring `--source`
- [FEATURE] `--high-cardinality-labels` flag overrides the default high-cardinality label pattern list (comma-separated)
- [FEATURE] `--min-description-length` and `--rule-min-length` flags are now consumed by the three new min-length rules (previously reserved/no-op)
- [FEATURE] `--strict` flag is now useful: promotes all warning-severity rules to errors (previously no-op in v0.1 which had only error rules)
- [UX] Version string is now build-time injectable via `-ldflags "-X main.version=v0.2.0"`; default is `"dev"` for unreleased builds
- [UX] Stderr warning when `--high-cardinality-labels` is set but `metric.label-high-cardinality-hint` is not enabled
- [UX] Stderr warning when `--min-description-length 0` is set (treated as "unset" sentinel)
- [ARCHITECTURE] `Rule` registry now supports off-by-default rules via `validation.Options.DefaultOff`; engine skips such rules unless explicitly listed in `Options.Enable`
- [ARCHITECTURE] `MetricSnapshot.ExtractionWarnings` field exposes pipeline/extractor warnings to validation rules without polluting the JSON wire shape (`json:"-"`)
- [BUGFIX] Min-length rules count Unicode runes via `utf8.RuneCountInString`, not bytes — "processed 5 characters of Chinese input" is 5, not 15

## [0.1.0] - 2026-04-21

Initial release.

- [FEATURE] CLI tool that extracts Prometheus metric metadata from Go source code via static AST analysis — no application execution required
- [FEATURE] Support for all prometheus/client_golang factories: `NewCounter` / `NewGauge` / `NewHistogram` / `NewSummary` with scalar and Vec variants, under both `prometheus.` and `promauto.` receivers
- [FEATURE] Swag-style doc-comment annotations: `@metric description`, `@metric calculation`, `@label <name> <description>`
- [FEATURE] Source location tracking (file, line, member) with auto-detected repo-relative paths via `.git` / `go.mod` markers
- [FEATURE] Directory walker with skip rules for vendored packages, `testdata/`, generated code, test files, and hidden / underscore-prefixed directories
- [FEATURE] Deterministic JSON snapshot output: alphabetically sorted metrics and labels, second-precision ISO-8601 UTC timestamps, stable byte-for-byte output across runs
- [FEATURE] Validation engine with 7 error-severity rules: `metric.name-required`, `metric.help-required`, `metric.description-required`, `metric.calculation-required`, `metric.label-description-required`, `metric.duplicate-name`, `metric.type-consistency`
- [FEATURE] CI-oriented CLI flags: `--validate`, `--strict`, `--skip-rule`, `--warn-rule`, `--error-rule`, `--enable-rule`, `--validation-report`, `--min-description-length`, `--rule-min-length`
- [FEATURE] Machine-readable JSON validation reports with rule ID, severity, message, location (file / line / metric / label), and error/warning counts for agent-driven autofix
- [FEATURE] Graceful shutdown on SIGINT / SIGTERM with atomic output file write (write-to-tmp + rename) to avoid partial JSON under failure
- [FEATURE] End-to-end golden-file tests against a realistic sample service fixture, with `UPDATE_GOLDEN=1` regeneration workflow
