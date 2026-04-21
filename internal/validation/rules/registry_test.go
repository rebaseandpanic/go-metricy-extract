package rules

import (
	"reflect"
	"sort"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// expectedRuleIDs is the canonical built-in rule set. Kept sorted so
// the assertion below is order-insensitive against the All() ordering
// (which is a separate concern: display order).
var expectedRuleIDs = []string{
	"metric.calculation-min-length",
	"metric.calculation-required",
	"metric.counter-total-suffix",
	"metric.description-min-length",
	"metric.description-required",
	"metric.duplicate-name",
	"metric.help-required",
	"metric.histogram-unit-suffix",
	"metric.label-description-min-length",
	"metric.label-description-required",
	"metric.label-high-cardinality-hint",
	"metric.name-required",
	"metric.name-snake-case",
	"metric.non-literal-metadata",
	"metric.type-consistency",
}

// errorRuleIDs / warningRuleIDs partition expectedRuleIDs by default
// severity. Split assertions key off this partition so adding a rule
// in either class only requires touching one slice.
var errorRuleIDs = map[string]bool{
	"metric.name-required":              true,
	"metric.help-required":              true,
	"metric.description-required":       true,
	"metric.calculation-required":       true,
	"metric.label-description-required": true,
	"metric.duplicate-name":             true,
	"metric.type-consistency":           true,
}

var warningRuleIDs = map[string]bool{
	"metric.counter-total-suffix":         true,
	"metric.histogram-unit-suffix":        true,
	"metric.name-snake-case":              true,
	"metric.non-literal-metadata":         true,
	"metric.description-min-length":       true,
	"metric.calculation-min-length":       true,
	"metric.label-description-min-length": true,
	"metric.label-high-cardinality-hint":  true,
}

func TestAll_ReturnsExpectedRuleCount(t *testing.T) {
	if n, want := len(All()), len(expectedRuleIDs); n != want {
		t.Fatalf("All(): got %d rules, want %d", n, want)
	}
}

func TestAll_AllIDsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, r := range All() {
		if seen[r.ID()] {
			t.Errorf("duplicate rule ID in registry: %s", r.ID())
		}
		seen[r.ID()] = true
	}
}

func TestAll_IDsMatchExpectedList(t *testing.T) {
	got := make([]string, 0, len(All()))
	for _, r := range All() {
		got = append(got, r.ID())
	}
	sort.Strings(got)

	if len(got) != len(expectedRuleIDs) {
		t.Fatalf("id count: got %d, want %d (got=%v, want=%v)", len(got), len(expectedRuleIDs), got, expectedRuleIDs)
	}
	for i := range got {
		if got[i] != expectedRuleIDs[i] {
			t.Errorf("id[%d]: got %q, want %q (full got=%v)", i, got[i], expectedRuleIDs[i], got)
		}
	}
}

// TestAll_ErrorRulesSet pins the set of rules that default to
// error-severity. If a rule's severity flips, this assertion breaks
// loudly — severity changes must be deliberate.
func TestAll_ErrorRulesSet(t *testing.T) {
	for _, r := range All() {
		if !errorRuleIDs[r.ID()] {
			continue
		}
		if got := r.DefaultSeverity(); got != validation.SeverityError {
			t.Errorf("rule %s: got severity %v, want SeverityError", r.ID(), got)
		}
	}
}

// TestAll_WarningRulesSet pins the set of rules that default to
// warning-severity. Same rationale as TestAll_ErrorRulesSet.
func TestAll_WarningRulesSet(t *testing.T) {
	for _, r := range All() {
		if !warningRuleIDs[r.ID()] {
			continue
		}
		if got := r.DefaultSeverity(); got != validation.SeverityWarning {
			t.Errorf("rule %s: got severity %v, want SeverityWarning", r.ID(), got)
		}
	}
}

// TestAll_EveryRuleClassified ensures every rule returned by All()
// is accounted for in either errorRuleIDs or warningRuleIDs. Catches
// the case where a new rule is added to All() but the partition
// slices are not updated.
func TestAll_EveryRuleClassified(t *testing.T) {
	for _, r := range All() {
		id := r.ID()
		in := errorRuleIDs[id] || warningRuleIDs[id]
		if !in {
			t.Errorf("rule %s is neither in errorRuleIDs nor warningRuleIDs (update the test partition)", id)
		}
		if errorRuleIDs[id] && warningRuleIDs[id] {
			t.Errorf("rule %s appears in both errorRuleIDs and warningRuleIDs (ambiguous)", id)
		}
	}
}

func TestAll_AllHaveNonEmptyDescription(t *testing.T) {
	for _, r := range All() {
		if r.Description() == "" {
			t.Errorf("rule %s: Description() returned empty string", r.ID())
		}
	}
}

// TestAll_DisplayOrderDeterministic pins the exact order in which rules
// are returned by All(). Breaking changes to the order require an explicit
// test update — prevents silent reordering via rule registry refactors.
// The order must also be stable across repeated calls.
func TestAll_DisplayOrderDeterministic(t *testing.T) {
	want := []string{
		// v0.1 — error severity
		"metric.name-required",
		"metric.help-required",
		"metric.description-required",
		"metric.calculation-required",
		"metric.label-description-required",
		"metric.duplicate-name",
		"metric.type-consistency",
		// v0.2 stage 1 — warning severity
		"metric.counter-total-suffix",
		"metric.histogram-unit-suffix",
		"metric.name-snake-case",
		"metric.non-literal-metadata",
		// v0.2 stage 2 — warning severity, min-length
		"metric.description-min-length",
		"metric.calculation-min-length",
		"metric.label-description-min-length",
		// v0.2 stage 3 — warning severity, off-by-default
		"metric.label-high-cardinality-hint",
	}
	got1 := All()
	got2 := All()

	ids1 := make([]string, 0, len(got1))
	ids2 := make([]string, 0, len(got2))
	for _, r := range got1 {
		ids1 = append(ids1, r.ID())
	}
	for _, r := range got2 {
		ids2 = append(ids2, r.ID())
	}

	if !reflect.DeepEqual(ids1, want) {
		t.Errorf("All() order:\n got: %v\nwant: %v", ids1, want)
	}
	if !reflect.DeepEqual(ids1, ids2) {
		t.Errorf("All() not deterministic across calls: %v vs %v", ids1, ids2)
	}
}

// TestDefaultOffIDs_OnlyHighCardinalityInV02Stage3 pins the contents of
// the default-off set. Stage 3 introduces exactly one off-by-default
// rule — metric.label-high-cardinality-hint. Any new entry or removal
// must break this test deliberately.
func TestDefaultOffIDs_OnlyHighCardinalityInV02Stage3(t *testing.T) {
	ids := DefaultOffIDs()
	if len(ids) != 1 {
		t.Fatalf("want 1 off-by-default rule, got %d: %v", len(ids), ids)
	}
	if !ids["metric.label-high-cardinality-hint"] {
		t.Errorf("expected metric.label-high-cardinality-hint in DefaultOffIDs, got: %v", ids)
	}
}
