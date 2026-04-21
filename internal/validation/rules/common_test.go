package rules

import (
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
)

func TestResolveMinLength(t *testing.T) {
	const ruleID = "metric.description-min-length"
	const hardcoded = 20

	cases := []struct {
		name string
		ctx  validation.Context
		want int
	}{
		{
			name: "empty context -- hardcoded default",
			ctx:  validation.Context{},
			want: hardcoded,
		},
		{
			name: "global MinDescriptionLength set -- overrides hardcoded",
			ctx:  validation.Context{MinDescriptionLength: 50},
			want: 50,
		},
		{
			name: "per-rule override -- wins over hardcoded",
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: 5},
			},
			want: 5,
		},
		{
			name: "per-rule override -- wins over global",
			ctx: validation.Context{
				MinDescriptionLength: 50,
				RuleMinLength:        map[string]int{ruleID: 7},
			},
			want: 7,
		},
		{
			name: "per-rule override for DIFFERENT rule -- falls back to global",
			ctx: validation.Context{
				MinDescriptionLength: 30,
				RuleMinLength:        map[string]int{"some.other-rule": 99},
			},
			want: 30,
		},
		{
			name: "per-rule override for DIFFERENT rule -- falls back to hardcoded",
			ctx: validation.Context{
				RuleMinLength: map[string]int{"some.other-rule": 99},
			},
			want: hardcoded,
		},
		{
			// Zero global is "unset", NOT "require 0 chars". CLI reserves
			// MinDescriptionLength=0 as the sentinel for "user never
			// provided --min-description-length", so rules must fall
			// through to their hardcoded default.
			name: "zero global -- treated as unset",
			ctx:  validation.Context{MinDescriptionLength: 0},
			want: hardcoded,
		},
		{
			// Per-rule zero IS a valid signal (user explicitly wrote
			// --rule-min-length foo:0). A minimum of 0 means every
			// non-empty value passes, which is what the user asked for.
			name: "per-rule zero -- honoured (user disabled the floor)",
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: 0},
			},
			want: 0,
		},
		{
			// Defence-in-depth: CLI already clamps negatives, but a
			// direct caller of the validation package could still
			// inject one. resolveMinLength clamps to 0 rather than
			// doing a nonsensical `len(s) < -3` comparison.
			name: "negative per-rule value -- clamped to 0",
			ctx: validation.Context{
				RuleMinLength: map[string]int{ruleID: -5},
			},
			want: 0,
		},
		{
			// Global negative falls through to hardcoded default.
			// The `> 0` guard in resolveMinLength means a negative
			// global is equivalent to "unset" — it cannot silently
			// flip the comparison into "every string passes".
			name: "negative global -- falls through to hardcoded default",
			ctx:  validation.Context{MinDescriptionLength: -5},
			want: hardcoded,
		},
		{
			name: "nil RuleMinLength map -- safe, falls back",
			ctx: validation.Context{
				MinDescriptionLength: 15,
				RuleMinLength:        nil,
			},
			want: 15,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveMinLength(tc.ctx, ruleID, hardcoded)
			if got != tc.want {
				t.Errorf("resolveMinLength: got %d, want %d", got, tc.want)
			}
		})
	}
}
