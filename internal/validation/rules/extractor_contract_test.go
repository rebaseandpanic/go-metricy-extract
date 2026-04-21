package rules_test

import (
	"go/token"
	"testing"

	"github.com/rebaseandpanic/go-metricy-extract/internal/extractor"
	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation"
	"github.com/rebaseandpanic/go-metricy-extract/internal/validation/rules"
)

// TestExtractorContract_NonLiteralNameHelpFlagged verifies that the substring
// pattern used by metric.non-literal-metadata stays synchronised with the
// warning text produced by the extractor. If extractor changes "non-literal
// Name" → e.g. "Name is runtime-computed", this test fails loudly instead
// of the rule silently emitting zero violations on a real silent-drop.
func TestExtractorContract_NonLiteralNameHelpFlagged(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "non-literal Name",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var name = "runtime_computed"
var M = prometheus.NewCounter(prometheus.CounterOpts{Name: name, Help: "h"})`,
		},
		{
			name: "non-literal Help",
			src: `package p
import "github.com/prometheus/client_golang/prometheus"
var help = "runtime_computed_help"
var M = prometheus.NewCounter(prometheus.CounterOpts{Name: "n", Help: help})`,
		},
	}

	rule := &rules.MetricNonLiteralMetadataRule{}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			res, err := extractor.ExtractSourceWithOptions(fset, "contract.go", tc.src, extractor.ExtractOptions{})
			if err != nil {
				t.Fatalf("extract: %v", err)
			}
			if len(res.Warnings) == 0 {
				t.Fatalf("expected extractor warnings; got none (extractor wording may have drifted)")
			}
			snap := &model.MetricSnapshot{
				Metrics:            res.Metrics,
				ExtractionWarnings: res.Warnings,
			}
			vios := rule.Validate(snap, validation.Context{})
			if len(vios) != 1 {
				t.Errorf("expected exactly 1 violation from rule; got %d (warnings: %v)",
					len(vios), res.Warnings)
			}
		})
	}
}
