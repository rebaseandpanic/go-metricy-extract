// Command go-metricy-extract extracts Prometheus metric metadata from Go
// source code via static AST analysis and emits a MetricSnapshot JSON
// document. No user code is executed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/pipeline"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation/rules"
)

const version = "0.1.0-dev"

// repeatable is a flag.Value implementation backing every "--foo bar --foo baz"
// CLI flag in this program. Keeping it a simple []string (rather than a map or
// set) preserves caller-supplied order for diagnostics; de-duplication is the
// caller's responsibility.
type repeatable []string

// String renders the current value for flag-package help output. Order is
// preserved so users can see exactly what they passed.
func (r *repeatable) String() string {
	if r == nil {
		return ""
	}
	return strings.Join(*r, ",")
}

// Set appends v to the list on each flag invocation.
func (r *repeatable) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// allValidationRules is the set of rules wired into --validate. The
// registry is populated from the rules sub-package; keeping it here
// (rather than inside internal/validation) lets the engine stay agnostic
// of which concrete rules exist.
var allValidationRules = rules.All()

// defaultOffRuleIDs is the set of rule IDs that require --enable-rule
// to activate. Sourced from the rules package so the CLI and the
// registry agree on default-off status.
var defaultOffRuleIDs = rules.DefaultOffIDs()

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the CLI entry point, factored for testability. Exit codes:
//
//	0 — success (including --help, and --validate when no error-severity
//	    violations were found)
//	1 — fatal error (missing source dir, walk failure, output write error,
//	    serialization failure, validation-report write failure, or any
//	    error-severity validation violation under --validate)
//	2 — CLI usage error (unknown flag, missing --source)
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("go-metricy-extract", flag.ContinueOnError)
	// Silence the flag package's default output; we route usage/errors manually
	// so that -h goes to stdout and parse errors go to stderr.
	fs.SetOutput(io.Discard)

	var (
		source   string
		output   string
		project  string
		repoRoot string

		validate             bool
		strict               bool
		skipRules            repeatable
		warnRules            repeatable
		errorRules           repeatable
		enableRules          repeatable
		validationReport     string
		minDescriptionLength int
		ruleMinLength        repeatable
	)
	fs.StringVar(&source, "source", "", "Path to the Go source directory to scan (required)")
	fs.StringVar(&output, "output", "", "Output file path (defaults to stdout)")
	fs.StringVar(&project, "project", "", "Project name written into the snapshot (defaults to basename of --source)")
	fs.StringVar(&repoRoot, "repo-root", "", "Repository root for computing repo-relative source paths (defaults to auto-detect via .git/go.mod)")

	// Validation flags — wiring is present even before step 9 adds the rule
	// registry, so `--validate` with an empty registry produces an empty,
	// exit-0 report. This lets CI / integration tests exercise the flag
	// surface without depending on specific rules.
	fs.BoolVar(&validate, "validate", false, "Enable validation against built-in rules")
	fs.BoolVar(&strict, "strict", false, "Treat all warnings as errors (CI-strict mode)")
	fs.Var(&skipRules, "skip-rule", "Disable a rule by ID (repeatable)")
	fs.Var(&warnRules, "warn-rule", "Demote rule from error to warning (repeatable)")
	fs.Var(&errorRules, "error-rule", "Promote rule from warning to error (repeatable)")
	fs.Var(&enableRules, "enable-rule", "Enable an off-by-default rule (repeatable)")
	fs.StringVar(&validationReport, "validation-report", "", "Write validation report JSON to path (else stderr summary only)")
	fs.IntVar(&minDescriptionLength, "min-description-length", 20, "Global default for min-length rule checks")
	fs.Var(&ruleMinLength, "rule-min-length", "Per-rule min-length override 'RULE-ID:N' (repeatable)")

	printUsage := func(w io.Writer) {
		fmt.Fprintf(w, "go-metricy-extract %s\n\n", version)
		fmt.Fprintf(w, "Usage: go-metricy-extract --source <dir> [--output <path>] [--project <name>] [--repo-root <dir>]\n")
		fmt.Fprintf(w, "                         [--validate [--strict] [--skip-rule ID]... [--warn-rule ID]... [--error-rule ID]...\n")
		fmt.Fprintf(w, "                          [--enable-rule ID]... [--validation-report PATH]\n")
		fmt.Fprintf(w, "                          [--min-description-length N] [--rule-min-length ID:N]...]\n\n")
		fs.SetOutput(w)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
	}
	// Silence the flag package's auto-invoked usage; we control output manually
	// below so -h goes cleanly to stdout and parse errors stay on stderr.
	fs.Usage = func() {}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		printUsage(stderr)
		return 2
	}

	if source == "" {
		fmt.Fprintln(stderr, "error: --source is required")
		printUsage(stderr)
		return 2
	}

	// Bind SIGINT/SIGTERM to the pipeline context so an interrupted walk exits
	// cleanly. The pipeline polls ctx.Err() between files; on cancellation it
	// returns context.Canceled and the CLI reports it via the fatal-error path.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := pipeline.Run(ctx, pipeline.Options{
		Source:   source,
		Project:  project,
		RepoRoot: repoRoot,
		Version:  version,
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// Warnings are diagnostic noise — keep them on stderr so piping the
	// snapshot to jq / a file does not mix them with JSON output.
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "warn: %s\n", w)
	}

	// Validation — runs before snapshot write so a validation-report write
	// error can abort with exit 1 without leaving a half-configured state.
	// The snapshot itself is still written unless report I/O failed, matching
	// the "fail-fast on I/O error" contract of other CLI flags.
	//
	// hasValidationError distinguishes "report was fine, but a rule flagged
	// an error-severity violation" from "report I/O failed". The former lets
	// the snapshot write proceed; the latter short-circuits to exit 1.
	var hasValidationError bool
	if validate {
		ok, vErr := runValidation(
			res.Snapshot,
			stderr,
			skipRules, warnRules, errorRules, enableRules,
			validationReport,
			strict,
			minDescriptionLength,
			ruleMinLength,
		)
		if !ok {
			return 1
		}
		hasValidationError = vErr
	}

	data, err := json.MarshalIndent(res.Snapshot, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to serialize snapshot: %s\n", err)
		return 1
	}
	data = append(data, '\n')

	if output == "" {
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintf(stderr, "error: failed to write snapshot: %s\n", err)
			return 1
		}
	} else {
		// Atomic write: write to <output>.tmp then rename. Avoids leaving a
		// half-written file on disk if the process is killed mid-write, and makes
		// the update visible to concurrent readers in a single syscall.
		tmp := output + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			fmt.Fprintf(stderr, "error: failed to write %s: %s\n", tmp, err)
			return 1
		}
		if err := os.Rename(tmp, output); err != nil {
			_ = os.Remove(tmp) // best-effort cleanup; ignore removal error
			fmt.Fprintf(stderr, "error: failed to rename %s -> %s: %s\n", tmp, output, err)
			return 1
		}
	}

	// Violation-driven exit code applies last so snapshot output is always
	// written before we fail the run.
	if hasValidationError {
		return 1
	}
	return 0
}

// runValidation runs the validation engine and handles report emission.
// Returns (ok, hasError):
//
//	ok == false           → report I/O failed; caller must exit 1 immediately
//	                        (snapshot write should be skipped).
//	ok == true, err true  → at least one error-severity violation was found;
//	                        caller writes the snapshot and then exits 1.
//	ok == true, err false → no error-severity violations; caller exits 0.
//
// Unknown rule IDs are warned to stderr; the engine itself skips them
// silently so typos don't change severity resolution unpredictably.
func runValidation(
	snapshot *model.MetricSnapshot,
	stderr io.Writer,
	skipRules, warnRules, errorRules, enableRules repeatable,
	reportPath string,
	strict bool,
	minDescriptionLength int,
	ruleMinLength repeatable,
) (ok bool, hasError bool) {
	// Warn on unknown rule IDs — typos here would silently do nothing, so
	// surface them early. When the registry is empty (pre-step-9), every ID
	// passed is "unknown" by definition.
	knownIDs := map[string]bool{}
	for _, r := range allValidationRules {
		knownIDs[r.ID()] = true
	}
	for _, id := range concatAll(skipRules, warnRules, errorRules, enableRules) {
		if !knownIDs[id] {
			fmt.Fprintf(stderr, "warn: unknown rule id: %s\n", id)
		}
	}

	// Severity overrides — error-rule wins over warn-rule when both list
	// the same ID; the engine surfaces these conflicts for us to print.
	overrides, conflicts := validation.BuildOverrides(
		allValidationRules,
		strict,
		[]string(warnRules),
		[]string(errorRules),
	)
	for _, c := range conflicts {
		fmt.Fprintf(stderr, "warn: rule %s listed in both --warn-rule and --error-rule; --error-rule wins\n", c)
	}

	// Parse --rule-min-length entries. Malformed entries are warned and
	// skipped; they don't abort the run because the rule itself can fall
	// back to the global default.
	ruleMin := map[string]int{}
	for _, rml := range ruleMinLength {
		parts := strings.SplitN(rml, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			fmt.Fprintf(stderr, "warn: invalid --rule-min-length value %q: expected 'RULE-ID:N'\n", rml)
			continue
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Fprintf(stderr, "warn: invalid --rule-min-length %q: %s\n", rml, err)
			continue
		}
		if n < 0 {
			// Negative minimums are trivially satisfied and almost always a
			// typo; clamp to 0 and surface it so the user can see what the
			// engine actually applied.
			fmt.Fprintf(stderr, "warn: --rule-min-length %s: negative value %d treated as 0\n", rml, n)
			n = 0
		}
		ruleMin[parts[0]] = n
	}

	valRes := validation.Run(snapshot, validation.Options{
		Rules:            allValidationRules,
		Skip:             toSet(skipRules),
		Enable:           toSet(enableRules),
		DefaultOff:       defaultOffRuleIDs,
		SeverityOverride: overrides,
		Strict:           strict,
		Context: validation.Context{
			MinDescriptionLength: minDescriptionLength,
			RuleMinLength:        ruleMin,
		},
	})

	if reportPath != "" {
		f, err := os.Create(reportPath)
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to create validation report %s: %s\n", reportPath, err)
			return false, false
		}
		writeErr := validation.WriteReport(f, valRes)
		closeErr := f.Close()
		if writeErr != nil {
			fmt.Fprintf(stderr, "error: failed to write validation report %s: %s\n", reportPath, writeErr)
			return false, false
		}
		if closeErr != nil {
			fmt.Fprintf(stderr, "error: failed to close validation report %s: %s\n", reportPath, closeErr)
			return false, false
		}
	} else {
		if summary := validation.FormatStderrSummary(valRes); summary != "" {
			fmt.Fprint(stderr, summary)
		}
	}

	// Effective severity reflects overrides and --strict applied by the
	// engine; any error-severity hit fails the run.
	for _, v := range valRes.Violations {
		if v.Severity == validation.SeverityError {
			return true, true
		}
	}
	return true, false
}

// concatAll flattens multiple repeatable lists into one slice. Order is
// preserved within each input; groups are appended in argument order. Used
// only for the unknown-ID warning pass so allocation is negligible.
func concatAll(lists ...repeatable) []string {
	var out []string
	for _, l := range lists {
		out = append(out, []string(l)...)
	}
	return out
}

// toSet converts a repeatable into a membership map. Empty inputs produce
// an empty (non-nil) map so downstream map-indexing never panics.
func toSet(r repeatable) map[string]bool {
	out := make(map[string]bool, len(r))
	for _, id := range r {
		out[id] = true
	}
	return out
}
