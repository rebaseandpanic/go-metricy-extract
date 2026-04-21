# go-metricy-extract

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

CLI tool that extracts Prometheus metric metadata from Go source code via static AST analysis — **no application execution required**. Companion tool to [`dotnet-metricy-extract`](https://www.nuget.org/packages/Metricy.Extract) for Go services.

## Installation

```bash
go install github.com/rebaseandpanic/go-metricy-extract/cmd/go-metricy-extract@latest
```

## Usage

```bash
go-metricy-extract --source ./path/to/service --output metrics.json
```

With validation and a machine-readable report:

```bash
go-metricy-extract --source ./service --validate --validation-report report.json
```

## Why

Runtime metric registries (the typical approach — boot the process, let `prometheus.MustRegister` run, then walk the default registry) require the application to start. In practice that means a real database, message queues, external APIs, background jobs, and every environment variable the service needs at boot. Without infrastructure the process crashes and no metadata is emitted.

`go-metricy-extract` reads metric declarations directly from source files via the `go/ast` and `go/parser` packages from the standard library. It never executes any code from your service. All it needs is the directory of `.go` files. This makes extraction work:

- In CI/CD without any infrastructure.
- On developer machines without Docker, databases, or a running service.
- Against any Go version — source is parsed, not compiled.

Annotate metrics once in doc comments, run the tool in CI on every build, post the JSON snapshot to a metric catalog of your choice. The catalog stays current without re-scanning source trees on every task.

## Quick Example

**Service code:**

```go
package mysvc

import (
    "github.com/prometheus/client_golang/prometheus"
)

// HttpRequests counts incoming HTTP requests.
//
// @metric description Total incoming HTTP requests across all endpoints.
// @metric calculation Incremented in LoggingMiddleware on each completed request.
// @label method HTTP method: GET, POST, PUT, DELETE
// @label status_code HTTP response status code
var HttpRequests = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total HTTP requests processed",
    },
    []string{"method", "status_code"},
)
```

**Extracted JSON (excerpt):**

```json
{
  "schema_version": "1.0",
  "project": "mysvc",
  "extracted_at": "2026-04-20T10:00:00Z",
  "extractor": { "name": "go-metricy-extract", "version": "0.2.0" },
  "metrics": [
    {
      "name": "http_requests_total",
      "type": "counter",
      "help": "Total HTTP requests processed",
      "description": "Total incoming HTTP requests across all endpoints.",
      "calculation": "Incremented in LoggingMiddleware on each completed request.",
      "labels": [
        { "name": "method", "description": "HTTP method: GET, POST, PUT, DELETE" },
        { "name": "status_code", "description": "HTTP response status code" }
      ],
      "source_location": {
        "file": "main.go",
        "line": 14,
        "class": null,
        "member": "HttpRequests"
      }
    }
  ]
}
```

## Annotation Format

Three directives, each on its own line inside a doc comment attached to the metric declaration:

| Directive | Example |
|-----------|---------|
| `@metric description <text>` | `@metric description Total incoming HTTP requests across all endpoints.` |
| `@metric calculation <text>` | `@metric calculation Incremented in LoggingMiddleware on each completed request.` |
| `@label <name> <description>` | `@label method HTTP method: GET, POST, PUT, DELETE` |

Conventions — influenced by [`swaggo/swag`](https://github.com/swaggo/swag), which uses the same `@verb key value` style in Go doc comments:

- One directive per line. Multi-line values are not supported; continuation lines are dropped, but since v0.3.0 a warning is emitted to stderr ("possible multi-line continuation after `<directive>`; only the first line is captured") so you can notice the truncation. Blank lines reset the tracker, and leading `///` triple-slash lines are treated as blank so there are no false positives on mixed prose.
- Directives are case-sensitive. `@Metric` is ignored.
- Duplicate `@metric description` or `@metric calculation` emits a warning to stderr and overwrites the previous value.
- Unknown `@tags` (for example `@api`, `@param`, `@deprecated`) are silently skipped so they can coexist with other tooling.

## CLI Parameters

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `--source <dir>` | yes | — | Directory scanned recursively for `.go` files. |
| `--output <path>` | no | stdout | Output file path. When omitted, JSON is written to stdout. |
| `--project <name>` | no | basename of `--source` | Project name written into the snapshot. |
| `--repo-root <dir>` | no | auto-detect | Repository root for computing repo-relative paths in `source_location.file`. Walks up from `--source` looking for `.git` or `go.mod`. |
| `--validate` | no | off | Enable validation against built-in rules. Any error-severity violation returns exit 1. |
| `--strict` | no | off | Treat warnings as errors (CI-strict mode). Any warning-severity violation becomes an error and returns exit 1. |
| `--skip-rule <id>` | no | — | Disable a rule by ID. Repeatable. Unknown IDs print a warning to stderr. |
| `--warn-rule <id>` | no | — | Demote a rule from error to warning. Repeatable. |
| `--error-rule <id>` | no | — | Promote a rule from warning to error. Repeatable. Wins over `--warn-rule` on conflict. |
| `--enable-rule <id>` | no | — | Enable an off-by-default rule. Repeatable. |
| `--validation-report <path>` | no | stderr summary | Write a machine-readable JSON report to this path. Without it, a short summary is written to stderr. |
| `--min-description-length <N>` | no | `20` | Global minimum length for description/calculation/label min-length rules. Counted in Unicode runes. Setting `0` prints a stderr warning and is treated as "unset". |
| `--rule-min-length <id>:<N>` | no | — | Per-rule minimum-length override, e.g. `--rule-min-length metric.label-description-min-length:5`. Repeatable. Wins over `--min-description-length`. |
| `--high-cardinality-labels <csv>` | no | built-in list | Override default high-cardinality label patterns for `metric.label-high-cardinality-hint` (comma-separated, e.g. `tenant_id,device_id`). Setting this without `--enable-rule metric.label-high-cardinality-hint` prints a stderr warning — the rule is off by default. |
| `--list-rules` | no | off | Print all validation rules (ID, severity, default on/off, description) and exit 0. Does not require `--source`. |

## Exit Codes

`go-metricy-extract` uses distinct exit codes so CI scripts can
differentiate between user error, validation findings, and tool failures:

| Code | Meaning |
|------|---------|
| `0` | Success — extraction completed and (if requested) validation passed |
| `1` | Validation failed — at least one error-severity violation present |
| `2` | CLI usage error — invalid flag, missing `--source`, malformed argument |
| `3` | Tool crashed — pipeline, serialization, or I/O failure |

**Breaking change in v0.3.1:** earlier versions returned `1` for both
validation failures and tool crashes. CI scripts that relied on `exit
code != 0` continue to work. Scripts that distinguish failure types should
update to the new taxonomy.

**Note on `--source` path issues:** a non-existent `--source` directory, a
`--source` pointing at a file (not a directory), or permission/I/O failures
during the directory walk are classified as tool crashes (exit 3), not CLI
usage errors (exit 2). The CLI only validates the *presence* of required
flags; path semantics are checked at runtime and failures indicate the
environment does not match what the caller expected.

## Validation

Running `--validate` checks the snapshot against **15 built-in rules** — 7 errors + 7 warnings on by default + 1 warning off by default. Any error-severity violation returns exit 1. Warnings do not affect the exit code unless `--strict` is set.

| Rule ID | Severity | Default | Description |
|---------|----------|---------|-------------|
| `metric.name-required` | error | on | Metric name must be a non-empty string |
| `metric.help-required` | error | on | Metric help text must be a non-empty string |
| `metric.description-required` | error | on | Annotation description must be set |
| `metric.calculation-required` | error | on | Annotation calculation must be set |
| `metric.label-description-required` | error | on | Every declared label must have a description annotation |
| `metric.duplicate-name` | error | on | Metric name must not appear more than once |
| `metric.type-consistency` | error | on | Same metric name must not be registered with two different types |
| `metric.counter-total-suffix` | warning | on | Counter metric names must end with `_total` |
| `metric.histogram-unit-suffix` | warning | on | Histogram names must end with a unit suffix (`_seconds`, `_bytes`, `_ratio`, ...) |
| `metric.name-snake-case` | warning | on | Metric name must be snake_case |
| `metric.non-literal-metadata` | warning | on | Metric name or help is computed at runtime and was silently dropped |
| `metric.description-min-length` | warning | on | Annotation description must be at least N characters (default 20) |
| `metric.calculation-min-length` | warning | on | Annotation calculation must be at least N characters (default 20) |
| `metric.label-description-min-length` | warning | on | Label description must be at least N characters (default 10) |
| `metric.label-high-cardinality-hint` | warning | **off** | Label name matches a known high-cardinality pattern (`user_id`, `email`, `ip`, ...) |

Rules marked **off** are not run by default. Enable them with `--enable-rule <id>`.

Run `go-metricy-extract --list-rules` at any time to print this list from the
installed binary — includes the current severity/default state.

### Typical CI usage

```bash
# Block on errors (default validation mode)
go-metricy-extract --source ./service --validate

# Strict mode: treat all warnings as errors
go-metricy-extract --source ./service --validate --strict

# Skip a noisy rule
go-metricy-extract --source ./service --validate --skip-rule metric.calculation-required

# Demote a rule from error to warning
go-metricy-extract --source ./service --validate --warn-rule metric.label-description-required

# Enable high-cardinality label detection (off by default)
go-metricy-extract --source ./service --validate \
  --enable-rule metric.label-high-cardinality-hint

# Customize the high-cardinality label list
go-metricy-extract --source ./service --validate \
  --enable-rule metric.label-high-cardinality-hint \
  --high-cardinality-labels tenant_id,workspace_id,device_id

# Tune min-length rules (global + per-rule override)
go-metricy-extract --source ./service --validate \
  --min-description-length 30 \
  --rule-min-length metric.label-description-min-length:5

# Discover all available rules (no --source required)
go-metricy-extract --list-rules

# Write machine-readable JSON report for agent-driven autofix
go-metricy-extract --source ./service --validate --validation-report report.json
```

### Validation report format (schema_version 1.1)

The `--validation-report` JSON has:

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Always `"1.1"`. Bumped from `"1.0"` in v0.3.0 (additive changes only — pre-1.1 consumers that ignore unknown keys continue to read these documents correctly). |
| `generated_at` | string | ISO-8601 UTC timestamp, second precision. Same layout as snapshot `extracted_at` so timestamps across the two documents are comparable without format massaging. |
| `violations[]` | array | Every violation with `rule_id`, `severity`, `message`, and `location` (file / line / metric / label / member). Sorted deterministically by `rule_id`. Empty array when there are no violations (never `null`). |
| `error_count` | int | Total error-severity violations. Zero when clean. |
| `warning_count` | int | Total warning-severity violations. Zero when clean. |
| `by_rule[]` | array | Per-rule aggregation: one entry per rule that fired, with `rule_id`, `severity` (effective severity label for the group), `error_count`, and `warning_count`. Sorted by `rule_id`. Empty array when there are no violations. |

Example (clean fixture):

```json
{
  "schema_version": "1.1",
  "generated_at": "2026-04-20T10:00:00Z",
  "violations": [],
  "error_count": 0,
  "warning_count": 0,
  "by_rule": []
}
```

Example (broken fixture, excerpt):

```json
{
  "schema_version": "1.1",
  "generated_at": "2026-04-20T10:00:00Z",
  "violations": [ /* ... */ ],
  "error_count": 4,
  "warning_count": 2,
  "by_rule": [
    { "rule_id": "metric.calculation-required", "severity": "error",   "error_count": 1, "warning_count": 0 },
    { "rule_id": "metric.counter-total-suffix", "severity": "warning", "error_count": 0, "warning_count": 1 }
  ]
}
```

## What It Extracts

- All four Prometheus metric types: `Counter`, `Gauge`, `Histogram`, `Summary`.
- Vec variants: `CounterVec`, `GaugeVec`, `HistogramVec`, `SummaryVec` — including their label name lists.
- All three factory call shapes (since v0.3.0):
  - `prometheus.NewX(...)` — direct call on the `prometheus` package.
  - `promauto.NewX(...)` — direct call on the `promauto` package (uses the default registry).
  - `promauto.With(reg).NewX(...)` — chained form that binds to a custom registry. All 8 factories × both receiver forms extract identically.
- Labels declared as a `[]string{...}` literal inline at the call site, or as a single-level local `var labels = []string{...}` reference in the same file (since v0.3.0). Supports package-level vars that hold a literal slice; multi-name pairwise specs work too.
- `Name` and `Help` from the inline `prometheus.*Opts` struct literal.
- Doc-comment annotations: `@metric description`, `@metric calculation`, `@label <name> <description>`.
- Source location for each metric: file path (repo-root-relative, forward slashes), 1-based line, and declaring identifier (`member`).

## Output JSON Format

Top-level fields:

| Field | Type | Description |
|-------|------|-------------|
| `schema_version` | string | Always `"1.0"`. Consumers must treat unknown keys as additive. |
| `project` | string | Project name from `--project` or basename of `--source`. |
| `extracted_at` | string | ISO-8601 UTC timestamp, second precision. |
| `extractor.name` | string | Always `"go-metricy-extract"`. |
| `extractor.version` | string | Tool version. |
| `metrics[]` | array | Metrics sorted alphabetically by name. |
| `metrics[].name` | string | Prometheus metric name. |
| `metrics[].type` | string | One of `counter`, `gauge`, `histogram`, `summary`. |
| `metrics[].help` | string | Help string from the native call. |
| `metrics[].description` | string\|null | Business description from `@metric description`. |
| `metrics[].calculation` | string\|null | Calculation algorithm from `@metric calculation`. |
| `metrics[].labels[]` | array | Labels sorted alphabetically by name. |
| `metrics[].labels[].name` | string | Label name. |
| `metrics[].labels[].description` | string\|null | Description from `@label`. |
| `metrics[].source_location` | object\|omitted | File/line/member of the declaration, omitted when unresolved. |

Example snapshot fragment:

```json
{
  "schema_version": "1.0",
  "project": "sample-service",
  "extracted_at": "2026-04-20T10:00:00Z",
  "extractor": {
    "name": "go-metricy-extract",
    "version": "0.2.0"
  },
  "metrics": [
    {
      "name": "active_connections",
      "type": "gauge",
      "help": "Number of active connections",
      "description": "Number of currently active client connections.",
      "calculation": "Incremented on connect, decremented on disconnect.",
      "labels": [],
      "source_location": {
        "file": "main.go",
        "line": 35,
        "class": null,
        "member": "ActiveConnections"
      }
    },
    {
      "name": "bytes_processed_total",
      "type": "counter",
      "help": "Total bytes processed",
      "description": "Total bytes processed across all connections.",
      "calculation": "Incremented in Read/Write methods with payload length.",
      "labels": [
        { "name": "direction", "description": "Either \"in\" or \"out\"" }
      ],
      "source_location": {
        "file": "middleware/metrics.go",
        "line": 11,
        "class": null,
        "member": "BytesProcessed"
      }
    }
  ]
}
```

## Limitations

| Pattern | Limitation |
|---------|------------|
| Dot-import `import . "prometheus"` + bare `NewCounter(...)` | Not recognized. Use the `prometheus.` or `promauto.` receiver prefix. |
| `NewCounter(opts)` where `opts` is a variable | Config cannot be resolved statically. Emits a warning to stderr and skips the metric. |
| Non-literal `Name` or `Help` (computed at runtime) | Static analysis reads string literals only. Emits a warning to stderr and skips the metric. |
| Label `[]string{...}` with non-literal *elements* (e.g. `[]string{buildLabel()}`) | Non-literal elements are dropped with a warning; the metric is still emitted with the remaining literal labels. (Element-level — distinct from slice-level var resolution below.) |
| Two-level label chains (`var b = a; var a = []string{...}`) | Only single-level `var labels = []string{...}` is resolved. Chains through intermediate variables are not followed. Emits a warning and the metric ships without labels. |
| Alias-typed labels (`type MyLabels []string; var labels = MyLabels{...}`) | Only the raw `[]string{...}` composite literal is recognized. Aliased types fall back to warning. |
| Function-local `var` labels for top-level metrics | Only package-level vars in the same file as the metric are resolved. Function-scoped vars fall back to warning. |
| Cross-file or cross-package label vars | Labels must live in the same file as the metric declaration. Warning when the referenced identifier is elsewhere. |
| Multi-line `@metric description` / `@metric calculation` / `@label` | Only the first line is captured. Since v0.3.0 a warning is emitted to stderr ("possible multi-line continuation…"); the continuation is still dropped. Use single-line values, or split into multiple directives. |
| System metrics (`expvar`, `runtime/metrics`, `go.opentelemetry.io/otel/metric`) | Not supported. Only `prometheus/client_golang` `New*` factory calls are recognized. |

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/google/go-cmp` | Test-only: golden-file diff output. |

Does **not** depend on: `prometheus/client_golang`, any HTTP framework, Docker, or any runtime infrastructure.

## Requirements

- Go 1.26 or later to build and run the tool.
- A directory of `.go` source files to scan. The target project does not need to be buildable — the tool parses sources independently.

## Building

```bash
go build ./...
go test ./...

# Regenerate golden fixtures after intentional output changes:
UPDATE_GOLDEN=1 go test ./internal/pipeline/... -run Golden
```

## License

MIT
