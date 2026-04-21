// Package extractor walks Go source files and extracts Prometheus metric
// metadata from static factory-call patterns.
//
// Extractor policy for malformed declarations (three-tier):
//
//  1. Wrong opts type or absent Name/Help → silent skip. The user may be
//     building a different metric shape; no evidence of extraction intent.
//  2. Recognized factory + opts-shaped literal with non-literal Name/Help
//     → warn + skip metric. The user clearly intended a metric but used
//     runtime-resolved metadata we can't read statically.
//  3. Vec constructor with malformed labels slice → warn + emit metric
//     with best-effort labels (subset / empty). Non-fatal: the metric
//     exists, labels are partial. Missing labels argument entirely
//     remains warn + skip.
//
// Receiver recognition is syntactic: only bare `prometheus.*` and
// `promauto.*` are matched. Aliased imports (e.g. `import promauto2 "..."`)
// are not resolved; chain forms like `promauto.With(reg).NewX(...)` are
// silently skipped and may be supported in a later release.
//
// Source location: emitted [model.MetricDescriptor.SourceLocation] always
// has Class == nil — Go has no C#-style classes, and the extractor only
// looks at package-level var declarations. File / Line / Member are
// populated from the declaration's position in the parsed FileSet.
package extractor

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"

	"github.com/rebaseandpanic/go-metricy-extract/internal/annotations"
	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/sourceloc"
)

// Result is the outcome of extracting metrics from a single Go file.
type Result struct {
	Metrics  []model.MetricDescriptor
	Warnings []string
}

// declContext carries the declaration's identifiers down the call chain
// so warnings can point at the offending variable and emitted metrics can
// be annotated with a source location.
type declContext struct {
	varName string
	// ann carries the annotations already parsed from the declaration's doc
	// comment (empty Annotations{} if no doc or no directives). Parser
	// warnings are pushed to res.Warnings at the var-block level, so callers
	// at the leaf (extractFromCall) just consume fields without re-warning.
	ann annotations.Annotations
	// namePos is the position of the declaring identifier (vs.Names[i]).
	// token.NoPos when unavailable — in that case SourceLocation is omitted.
	namePos token.Pos
}

// metricKind captures the metadata associated with a recognized prometheus
// factory function.
type metricKind struct {
	metricType string // stored in MetricDescriptor.Type
	optsType   string // expected type name of the CompositeLit argument
	isVec      bool   // if true, second argument is []string of label names
}

// metricKinds maps prometheus/promauto factory function names to the
// extracted metric type and the expected options struct type.
var metricKinds = map[string]metricKind{
	"NewCounter":      {metricType: "counter", optsType: "CounterOpts", isVec: false},
	"NewGauge":        {metricType: "gauge", optsType: "GaugeOpts", isVec: false},
	"NewHistogram":    {metricType: "histogram", optsType: "HistogramOpts", isVec: false},
	"NewSummary":      {metricType: "summary", optsType: "SummaryOpts", isVec: false},
	"NewCounterVec":   {metricType: "counter", optsType: "CounterOpts", isVec: true},
	"NewGaugeVec":     {metricType: "gauge", optsType: "GaugeOpts", isVec: true},
	"NewHistogramVec": {metricType: "histogram", optsType: "HistogramOpts", isVec: true},
	"NewSummaryVec":   {metricType: "summary", optsType: "SummaryOpts", isVec: true},
}

// recognizedReceivers is the set of package qualifiers we accept as the
// receiver of a metric-factory call. `promauto.With(reg).NewX(...)` is a
// different shape (method on a call result) and remains unsupported here.
var recognizedReceivers = map[string]struct{}{
	"prometheus": {},
	"promauto":   {},
}

// defaultParser is the annotation parser used when callers do not supply one.
var defaultParser annotations.AnnotationParser = annotations.SwagStyleParser{}

// ExtractOptions configures extraction. A zero value is valid and behaves
// identically to [ExtractSource]: the default SwagStyleParser is used and
// source-location filenames are left as whatever string was handed to the
// parser (typically an absolute path).
type ExtractOptions struct {
	// Parser overrides the annotation parser. Nil falls back to the default
	// SwagStyleParser.
	Parser annotations.AnnotationParser
	// RepoRoot, when non-empty, causes [model.SourceLocation.File] to be
	// rewritten to a repo-relative forward-slash path via
	// [sourceloc.MakeRelative]. Files outside RepoRoot are passed through
	// unchanged. Empty keeps the raw filename from the FileSet.
	RepoRoot string
}

// ExtractFile parses filename and returns all Prometheus metrics found in
// top-level var declarations, using the default SwagStyleParser for doc
// comments. Warnings are collected for recognized but malformed patterns
// (e.g. non-literal Name / Help) and for malformed annotation directives.
func ExtractFile(fset *token.FileSet, filename string) (*Result, error) {
	return ExtractSourceWithOptions(fset, filename, nil, ExtractOptions{})
}

// ExtractFileWithParser is like ExtractFile but uses the supplied annotation
// parser. Passing nil falls back to the default SwagStyleParser.
func ExtractFileWithParser(fset *token.FileSet, filename string, parser annotations.AnnotationParser) (*Result, error) {
	return ExtractSourceWithOptions(fset, filename, nil, ExtractOptions{Parser: parser})
}

// ExtractSource parses Go source from src (any type accepted by parser.ParseFile)
// — primarily for tests. filename is used for diagnostics. If src is nil, the
// file at filename is read from disk. Uses the default SwagStyleParser.
func ExtractSource(fset *token.FileSet, filename string, src any) (*Result, error) {
	return ExtractSourceWithOptions(fset, filename, src, ExtractOptions{})
}

// ExtractSourceWithParser is like ExtractSource but accepts a custom
// AnnotationParser (nil falls back to the default SwagStyleParser).
func ExtractSourceWithParser(fset *token.FileSet, filename string, src any, parser annotations.AnnotationParser) (*Result, error) {
	return ExtractSourceWithOptions(fset, filename, src, ExtractOptions{Parser: parser})
}

// ExtractSourceWithOptions is the primary entry-point. It parses Go source
// (either the src argument, or the file at filename if src is nil) and
// returns every recognized metric along with any warnings. The other
// Extract* functions are thin wrappers that build an [ExtractOptions].
func ExtractSourceWithOptions(fset *token.FileSet, filename string, src any, opts ExtractOptions) (*Result, error) {
	file, err := goParseFile(fset, filename, src)
	if err != nil {
		return nil, err
	}
	parser := opts.Parser
	if parser == nil {
		parser = defaultParser
	}

	res := &Result{}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			extractFromValueSpec(vs, gen, parser, fset, opts.RepoRoot, res)
		}
	}

	return res, nil
}

// goParseFile isolates the call to go/parser so the error wrapping stays in
// one place and ExtractSourceWithParser stays legible.
func goParseFile(fset *token.FileSet, filename string, src any) (*ast.File, error) {
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	return f, nil
}

// extractFromValueSpec walks a single `var` specification (which may declare
// multiple names) and appends any recognized metric to res.
//
// Doc-comment resolution: ValueSpec.Doc wins over GenDecl.Doc when both are
// present. This matches Go's convention for `var (...)` blocks where each
// inner spec can carry its own doc, and falls back to the block-level doc
// for single-line `var X = ...` declarations (where ValueSpec.Doc is nil
// and the comment sits on the GenDecl).
//
// fset + repoRoot are threaded through so each extracted metric can be
// annotated with its source location.
func extractFromValueSpec(vs *ast.ValueSpec, gen *ast.GenDecl, parser annotations.AnnotationParser, fset *token.FileSet, repoRoot string, res *Result) {
	if len(vs.Values) == 0 {
		return
	}

	docText := resolveDocText(vs, gen)
	ann, annWarnings := parser.Parse(docText)

	// Pair Names[i] with Values[i] when the shape is strictly pairwise.
	// Other shapes (e.g. multi-return `var a, b = f()` where len(Values)==1
	// and len(Names)>1) are not recognized metric patterns — fall back to an
	// empty varName rather than indexing out of range.
	pairwise := len(vs.Names) == len(vs.Values)

	// Prefix parser warnings with a varName once we can pick one. For a
	// multi-name pairwise spec we attribute annotation warnings to the first
	// name — the doc comment applies to the whole spec, not to any single
	// binding, and choosing the first gives a deterministic, stable prefix.
	var warnVarName string
	for _, n := range vs.Names {
		if n != nil && n.Name != "" {
			warnVarName = n.Name
			break
		}
	}
	for _, w := range annWarnings {
		res.Warnings = append(res.Warnings, formatWarning(declContext{varName: warnVarName}, w))
	}

	for i, value := range vs.Values {
		call, ok := value.(*ast.CallExpr)
		if !ok {
			continue
		}
		ctx := declContext{ann: ann, namePos: token.NoPos}
		if pairwise && vs.Names[i] != nil {
			ctx.varName = vs.Names[i].Name
			ctx.namePos = vs.Names[i].NamePos
		}
		extractFromCall(call, ctx, fset, repoRoot, res)
	}
}

// resolveDocText returns the text of the doc comment that applies to vs.
// ValueSpec.Doc takes precedence; fall back to GenDecl.Doc for single-line
// var declarations where the comment attaches to the GenDecl.
func resolveDocText(vs *ast.ValueSpec, gen *ast.GenDecl) string {
	if vs.Doc != nil {
		return vs.Doc.Text()
	}
	if gen != nil && gen.Doc != nil {
		return gen.Doc.Text()
	}
	return ""
}

// extractFromCall inspects a CallExpr to see if it is a recognized
// prometheus/promauto metric factory call (e.g. prometheus.NewCounter,
// promauto.NewGaugeVec). The set of recognized factories is driven by the
// package-level metricKinds table.
//
// fset + repoRoot are consulted when building the emitted metric's
// SourceLocation. If fset is nil or ctx.namePos is invalid the location
// is omitted.
func extractFromCall(call *ast.CallExpr, ctx declContext, fset *token.FileSet, repoRoot string, res *Result) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		// Non-receiver call (e.g. `NewCounter(...)` via dot-import) — not a
		// prometheus call in any pattern we recognize. Skip silently.
		return
	}

	recvIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		// Chained selector like `a.b.NewCounter(...)` or
		// `promauto.With(reg).NewCounter(...)` — sel.X is not a bare ident, so
		// it falls outside the shapes we recognize. Skip silently.
		return
	}

	if _, ok := recognizedReceivers[recvIdent.Name]; !ok {
		return
	}

	if sel.Sel == nil {
		return
	}

	kind, known := metricKinds[sel.Sel.Name]
	if !known {
		// Unknown factory function on the receiver — skip silently.
		return
	}

	if len(call.Args) < 1 {
		return
	}

	opts, ok := call.Args[0].(*ast.CompositeLit)
	if !ok {
		// Opts was passed as a variable/ident/other expression — we cannot
		// statically resolve its fields.
		res.Warnings = append(res.Warnings,
			formatWarning(ctx, "non-literal options argument; skipping metric"))
		return
	}

	// If a type is present on the composite literal, require it to match the
	// expected opts type (either `prometheus.OptsType` or bare `OptsType`).
	// If Type is nil, the literal relies on the target type — accept it.
	if opts.Type != nil && !isOptsType(opts.Type, kind.optsType) {
		return
	}

	name, nameOK, nameWarn := stringFieldFromOpts(opts, "Name", ctx)
	help, helpOK, helpWarn := stringFieldFromOpts(opts, "Help", ctx)

	if nameWarn != "" {
		res.Warnings = append(res.Warnings, nameWarn)
	}
	if helpWarn != "" {
		res.Warnings = append(res.Warnings, helpWarn)
	}

	if !nameOK || !helpOK {
		return
	}

	var labels []model.LabelDescriptor
	extractedLabelSet := map[string]struct{}{}
	if kind.isVec {
		labelNames, vecWarns, skip := extractVecLabels(call, ctx)
		res.Warnings = append(res.Warnings, vecWarns...)
		if skip {
			return
		}
		if len(labelNames) > 0 {
			labels = make([]model.LabelDescriptor, 0, len(labelNames))
			for _, ln := range labelNames {
				desc := labelDescriptionFromAnn(ctx.ann, ln)
				labels = append(labels, model.LabelDescriptor{Name: ln, Description: desc})
				extractedLabelSet[ln] = struct{}{}
			}
		}
	}

	// Surface @label annotations that don't correspond to any extracted
	// label. Sort for deterministic output across map iterations.
	if len(ctx.ann.Labels) > 0 {
		var orphans []string
		for ln := range ctx.ann.Labels {
			if _, ok := extractedLabelSet[ln]; !ok {
				orphans = append(orphans, ln)
			}
		}
		sort.Strings(orphans)
		for _, ln := range orphans {
			var msg string
			if kind.isVec {
				msg = fmt.Sprintf("@label %q not declared in labels slice; ignored", ln)
			} else {
				msg = fmt.Sprintf("@label %q on scalar metric (use Vec constructor to declare labels); ignored", ln)
			}
			res.Warnings = append(res.Warnings, formatWarning(ctx, msg))
		}
	}

	res.Metrics = append(res.Metrics, model.MetricDescriptor{
		Name:           name,
		Type:           kind.metricType,
		Help:           help,
		Description:    ctx.ann.Description,
		Calculation:    ctx.ann.Calculation,
		Labels:         labels,
		SourceLocation: buildSourceLocation(fset, ctx, repoRoot),
	})
}

// buildSourceLocation turns the declaration context into a
// [model.SourceLocation] pointer. Returns nil when the position or filename
// is unusable — callers then emit the metric with the zero-value (omitted)
// source_location field. Class is always nil: Go's package-level vars have
// no enclosing C#-style class, so the field is structurally unused here.
func buildSourceLocation(fset *token.FileSet, ctx declContext, repoRoot string) *model.SourceLocation {
	if fset == nil || !ctx.namePos.IsValid() {
		return nil
	}
	pos := fset.Position(ctx.namePos)
	if pos.Filename == "" {
		return nil
	}
	file := sourceloc.MakeRelative(pos.Filename, repoRoot)
	line := pos.Line
	sl := &model.SourceLocation{
		File:  file,
		Line:  &line,
		Class: nil,
	}
	if ctx.varName != "" {
		member := ctx.varName
		sl.Member = &member
	}
	return sl
}

// labelDescriptionFromAnn returns a pointer to the description for label
// name if present in ann.Labels, otherwise nil. A value of "" is never
// expected here because the parser rejects empty label descriptions, but we
// still guard against it to keep the empty-string / nil distinction clean.
func labelDescriptionFromAnn(ann annotations.Annotations, name string) *string {
	if ann.Labels == nil {
		return nil
	}
	desc, ok := ann.Labels[name]
	if !ok || desc == "" {
		return nil
	}
	return &desc
}

// extractVecLabels extracts label names from the second argument of a Vec
// constructor call. Returns (labels, warnings, skip):
//
//	skip=true  — missing or structurally unusable labels argument. Caller
//	             should discard the entire metric.
//	skip=false — metric emits, labels may be a subset (partially non-literal
//	             elements), nil (all non-literal / empty / wrong type), or
//	             full (all literal). Warnings describe any degradation.
//
// Duplicate label names are preserved in source order (not deduplicated);
// a warning is emitted once per duplicated name. Validation of the resulting
// label set against Prometheus registration rules is handled by later stages.
func extractVecLabels(call *ast.CallExpr, ctx declContext) (labels []string, warnings []string, skip bool) {
	if len(call.Args) < 2 {
		return nil, []string{formatWarning(ctx, "Vec constructor requires labels argument; skipping metric")}, true
	}

	lit, ok := call.Args[1].(*ast.CompositeLit)
	if !ok {
		// labels passed as variable, function call, or other non-literal expression.
		return nil, []string{formatWarning(ctx, "labels argument is not a []string{...} literal; emitting metric without labels")}, false
	}

	// Accept a composite literal with explicit []string type, or with nil Type
	// (the very uncommon top-level case where the type is inferred from context).
	if lit.Type != nil && !isStringSlice(lit.Type) {
		return nil, []string{formatWarning(ctx, "labels argument is not a []string{...} literal; emitting metric without labels")}, false
	}

	// Empty slice literal: `[]string{}`. The element loop below would never
	// run, so we surface this shape as its own warning (distinct from the
	// "zero literal names" case which implies non-literal elements were
	// present but unreadable).
	if len(lit.Elts) == 0 {
		return nil, []string{formatWarning(ctx, "Vec constructor has empty labels slice; emitting metric with zero labels")}, false
	}

	var nonLiteral bool
	seen := map[string]struct{}{}
	warnedDup := map[string]struct{}{}
	for _, elt := range lit.Elts {
		bl, ok := elt.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			nonLiteral = true
			continue
		}
		unquoted, err := strconv.Unquote(bl.Value)
		if err != nil {
			nonLiteral = true
			continue
		}
		// Preserve duplicates in output — validation is someone else's job.
		// Emit one warning per duplicated name (not per extra occurrence).
		if _, dup := seen[unquoted]; dup {
			if _, already := warnedDup[unquoted]; !already {
				warnings = append(warnings,
					formatWarning(ctx, fmt.Sprintf("duplicate label name %q in labels slice; consumer will fail registration", unquoted)))
				warnedDup[unquoted] = struct{}{}
			}
		} else {
			seen[unquoted] = struct{}{}
		}
		labels = append(labels, unquoted)
	}

	if nonLiteral {
		if len(labels) == 0 {
			warnings = append(warnings, formatWarning(ctx, "Vec labels contain zero literal names; emitting metric without labels"))
		} else {
			warnings = append(warnings, formatWarning(ctx, "non-literal label name in labels slice; continuing with extracted labels"))
		}
	}

	return labels, warnings, false
}

// isStringSlice reports whether expr is the type expression `[]string`.
func isStringSlice(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	if !ok {
		return false
	}
	// []string — no length. `[N]string` would have arr.Len != nil.
	if arr.Len != nil {
		return false
	}
	ident, ok := arr.Elt.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "string"
}

// isOptsType reports whether the given composite-literal type expression
// refers to prometheus.<typeName> (either qualified or bare).
//
// Opts types live only in the prometheus package, even for promauto
// factories. Accept both the qualified form (prometheus.CounterOpts) and
// the bare form (CounterOpts).
func isOptsType(expr ast.Expr, typeName string) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == typeName
	case *ast.SelectorExpr:
		recv, ok := t.X.(*ast.Ident)
		if !ok {
			return false
		}
		return recv.Name == "prometheus" && t.Sel != nil && t.Sel.Name == typeName
	}
	return false
}

// stringFieldFromOpts looks up a named field in a CounterOpts-like composite
// literal. Returns one of three outcomes:
//
//	value, true,  ""   — field present as a valid string literal
//	"",    false, msg  — field present but not a string literal (warn & skip)
//	"",    false, ""   — field absent (silent skip; caller's callsite decides)
func stringFieldFromOpts(opts *ast.CompositeLit, fieldName string, ctx declContext) (value string, found bool, warnMsg string) {
	for _, elt := range opts.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyIdent, ok := kv.Key.(*ast.Ident)
		if !ok || keyIdent.Name != fieldName {
			continue
		}

		lit, ok := kv.Value.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false, formatWarning(ctx, fmt.Sprintf("non-literal %s; skipping metric", fieldName))
		}
		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", false, formatWarning(ctx, fmt.Sprintf("invalid %s string literal; skipping metric", fieldName))
		}
		return unquoted, true, ""
	}
	// Field absent — caller decides whether this is malformed. For the step-3
	// contract an absent Name/Help is a silent skip.
	return "", false, ""
}

// formatWarning renders a warning message using the declContext. When a
// varName is available the message is prefixed "<varName>: <issue>";
// otherwise the raw issue is returned.
func formatWarning(ctx declContext, issue string) string {
	if ctx.varName == "" {
		return issue
	}
	return ctx.varName + ": " + issue
}
