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
  "extractor": { "name": "go-metricy-extract", "version": "0.1.0" },
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

- One directive per line. Multi-line values are not supported in v0.1 — continuation lines are silently dropped.
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
| `--strict` | no | off | Treat warnings as errors (CI-strict mode). No-op in v0.1 — all rules are error-severity. |
| `--skip-rule <id>` | no | — | Disable a rule by ID. Repeatable. Unknown IDs print a warning to stderr. |
| `--warn-rule <id>` | no | — | Demote a rule from error to warning. Repeatable. |
| `--error-rule <id>` | no | — | Promote a rule from warning to error. Repeatable. Wins over `--warn-rule` on conflict. |
| `--enable-rule <id>` | no | — | Enable an off-by-default rule. Repeatable. No-op in v0.1 — reserved for future rules. |
| `--validation-report <path>` | no | stderr summary | Write a machine-readable JSON report to this path. Without it, a short summary is written to stderr. |
| `--min-description-length <N>` | no | `20` | Global minimum length for description/calculation length rules. Reserved for v0.2 — no current rule consumes it. |
| `--rule-min-length <id>:<N>` | no | — | Per-rule minimum-length override. Repeatable. Reserved for v0.2. |

## Validation

Running `--validate` checks the snapshot against **7 built-in rules** — all error-severity in v0.1. Any error-severity violation returns exit 1.

| Rule ID | Severity | Description |
|---------|----------|-------------|
| `metric.name-required` | Error | Metric name must be a non-empty string. |
| `metric.help-required` | Error | Metric help text must be a non-empty string. |
| `metric.description-required` | Error | `@metric description` annotation must be set. |
| `metric.calculation-required` | Error | `@metric calculation` annotation must be set. |
| `metric.label-description-required` | Error | Every declared label must have an `@label <name> <description>` annotation. |
| `metric.duplicate-name` | Error | The same metric name must not appear more than once in the snapshot. |
| `metric.type-consistency` | Error | The same metric name must not be registered with two different types. |

### Typical CI usage

```bash
# Block on errors (default validation mode)
go-metricy-extract --source ./service --validate

# Skip a noisy rule
go-metricy-extract --source ./service --validate --skip-rule metric.calculation-required

# Demote a rule from error to warning
go-metricy-extract --source ./service --validate --warn-rule metric.label-description-required

# Write machine-readable JSON report for agent-driven autofix
go-metricy-extract --source ./service --validate --validation-report report.json
```

## What It Extracts

- All four Prometheus metric types: `Counter`, `Gauge`, `Histogram`, `Summary`.
- Vec variants: `CounterVec`, `GaugeVec`, `HistogramVec`, `SummaryVec` — including their label name lists.
- Both factory receivers: `prometheus.NewX(...)` and `promauto.NewX(...)`.
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
    "version": "0.1.0"
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
| `promauto.With(reg).NewX(...)` | Not supported in v0.1. Silently skipped. Planned for v0.2. |
| Dot-import `import . "prometheus"` + bare `NewCounter(...)` | Not recognized. Use the `prometheus.` or `promauto.` receiver prefix. |
| `NewCounter(opts)` where `opts` is a variable | Config cannot be resolved statically. Emits a warning to stderr and skips the metric. |
| Non-literal `Name` or `Help` (computed at runtime) | Static analysis reads string literals only. Emits a warning to stderr and skips the metric. |
| Label names from variables or `[]string{...}` with non-literal elements | Non-literal labels are dropped with a warning; the metric is still emitted with the remaining literal labels. |
| Multi-line `@metric description` | Not supported in v0.1. Continuation lines are silently dropped. |
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
