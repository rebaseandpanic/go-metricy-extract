package rules

import (
	"reflect"
	"sort"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

// expectedRuleIDs is the canonical v0.1 built-in rule set. Kept in a
// sorted slice so the assertion below is order-insensitive against the
// All() ordering (which is a separate concern: display order).
var expectedRuleIDs = []string{
	"metric.calculation-required",
	"metric.description-required",
	"metric.duplicate-name",
	"metric.help-required",
	"metric.label-description-required",
	"metric.name-required",
	"metric.type-consistency",
}

func TestAll_Returns7Rules(t *testing.T) {
	if n := len(All()); n != 7 {
		t.Fatalf("All(): got %d rules, want 7", n)
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

func TestAll_AllAreError(t *testing.T) {
	for _, r := range All() {
		if r.DefaultSeverity() != validation.SeverityError {
			t.Errorf("rule %s: got severity %v, want SeverityError", r.ID(), r.DefaultSeverity())
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
		"metric.name-required",
		"metric.help-required",
		"metric.description-required",
		"metric.calculation-required",
		"metric.label-description-required",
		"metric.duplicate-name",
		"metric.type-consistency",
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

func TestDefaultOffIDs_EmptyInV01(t *testing.T) {
	off := DefaultOffIDs()
	if len(off) != 0 {
		t.Errorf("DefaultOffIDs: got %d entries, want 0 for v0.1 (entries=%v)", len(off), off)
	}
}
