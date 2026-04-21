# Changelog

All notable changes to this project will be documented in this file.

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
