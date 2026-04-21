package annotations

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func strPtr(s string) *string { return &s }

func TestSwagStyleParser_Parse(t *testing.T) {
	tests := []struct {
		name         string
		doc          string
		wantAnn      Annotations
		wantWarnings int // exact count; -1 = >=1
	}{
		{
			name:         "empty doc",
			doc:          "",
			wantAnn:      Annotations{},
			wantWarnings: 0,
		},
		{
			name:         "plain text only, no directives",
			doc:          "HttpRequests counts incoming HTTP requests.\nAcross all endpoints.",
			wantAnn:      Annotations{},
			wantWarnings: 0,
		},
		{
			name: "only @metric description",
			doc:  "@metric description Total incoming HTTP requests.",
			wantAnn: Annotations{
				Description: strPtr("Total incoming HTTP requests."),
			},
			wantWarnings: 0,
		},
		{
			name: "full set: description + calculation + two labels",
			doc: `HttpRequests counts HTTP requests.

@metric description Total incoming HTTP requests across all endpoints.
@metric calculation Incremented in RequestLoggingMiddleware.ServeHTTP on each request.
@label method HTTP method: GET, POST, PUT, DELETE
@label status_code HTTP response status code`,
			wantAnn: Annotations{
				Description: strPtr("Total incoming HTTP requests across all endpoints."),
				Calculation: strPtr("Incremented in RequestLoggingMiddleware.ServeHTTP on each request."),
				Labels: map[string]string{
					"method":      "HTTP method: GET, POST, PUT, DELETE",
					"status_code": "HTTP response status code",
				},
			},
			wantWarnings: 0,
		},
		{
			name: "duplicate @metric description overwrites with warning",
			doc: `@metric description first text
@metric description second text`,
			wantAnn: Annotations{
				Description: strPtr("second text"),
			},
			wantWarnings: 1,
		},
		{
			name: "duplicate @label overwrites with warning",
			doc: `@label method first desc
@label method second desc`,
			wantAnn: Annotations{
				Labels: map[string]string{"method": "second desc"},
			},
			wantWarnings: 1,
		},
		{
			name:         "unknown @metric key warns",
			doc:          "@metric foo bar baz",
			wantAnn:      Annotations{},
			wantWarnings: 1,
		},
		{
			name:         "empty @metric description value warns",
			doc:          "@metric description   ",
			wantAnn:      Annotations{},
			wantWarnings: 1,
		},
		{
			name:         "empty @metric calculation value warns",
			doc:          "@metric calculation",
			wantAnn:      Annotations{},
			wantWarnings: 1,
		},
		{
			name:         "@label with only name and no description warns",
			doc:          "@label method",
			wantAnn:      Annotations{},
			wantWarnings: 1,
		},
		{
			name:         "@label with no name at all is rejected",
			doc:          "@label",
			wantAnn:      Annotations{},
			wantWarnings: 1,
		},
		{
			name:         "other @-directives silently skipped",
			doc:          "@api something\n@param x int\n@summary hello",
			wantAnn:      Annotations{},
			wantWarnings: 0,
		},
		{
			name: "mixed: plain text + @metric description + unknown @api",
			doc: `This is a metric doc.
@api some external directive
@metric description valid text here`,
			wantAnn: Annotations{
				Description: strPtr("valid text here"),
			},
			wantWarnings: 0,
		},
		{
			name: "leading and trailing whitespace trimmed in description",
			doc:  "   @metric description    hello world   ",
			wantAnn: Annotations{
				Description: strPtr("hello world"),
			},
			wantWarnings: 0,
		},
		{
			name: "tab separators accepted",
			doc:  "@metric\tdescription\ttab-separated text",
			wantAnn: Annotations{
				Description: strPtr("tab-separated text"),
			},
			wantWarnings: 0,
		},
		{
			name: "multi-line description NOT supported: continuation line warns",
			doc: `@metric description foo
bar baz`,
			wantAnn: Annotations{
				Description: strPtr("foo"),
			},
			wantWarnings: 1,
		},
		{
			name: "continuation after @metric calculation warns",
			doc: `@metric calculation Incremented
in RequestLoggingMiddleware.`,
			wantAnn: Annotations{
				Calculation: strPtr("Incremented"),
			},
			wantWarnings: 1,
		},
		{
			name: "continuation after @label warns",
			doc: `@label method HTTP method
used by the handler.`,
			wantAnn: Annotations{
				Labels: map[string]string{"method": "HTTP method"},
			},
			wantWarnings: 1,
		},
		{
			name: "blank line separates paragraphs — no continuation warning",
			doc: `@metric description Total requests

Plain godoc prose after blank line.`,
			wantAnn: Annotations{
				Description: strPtr("Total requests"),
			},
			wantWarnings: 0,
		},
		{
			name: "plain text before directive — no continuation warning",
			doc: `Package-level prose.
@metric description Total requests`,
			wantAnn: Annotations{
				Description: strPtr("Total requests"),
			},
			wantWarnings: 0,
		},
		{
			name: "multi-line paragraph after directive — single warning, not cascade",
			doc: `@metric description foo
bar
baz`,
			wantAnn: Annotations{
				Description: strPtr("foo"),
			},
			wantWarnings: 1,
		},
		{
			name: "continuation then new directive — warning emitted, new directive processed",
			doc: `@metric description foo
wrong continuation
@metric calculation bar`,
			wantAnn: Annotations{
				Description: strPtr("foo"),
				Calculation: strPtr("bar"),
			},
			wantWarnings: 1,
		},
		{
			name: "no warning when directive is last non-blank line",
			doc: `@metric description foo
`,
			wantAnn: Annotations{
				Description: strPtr("foo"),
			},
			wantWarnings: 0,
		},
		{
			name: "non-our @-directive does not trigger continuation",
			doc: `@metric description foo
@api something
continuation of @api should not warn`,
			wantAnn: Annotations{
				Description: strPtr("foo"),
			},
			wantWarnings: 0,
		},
		{
			name: "@label multiple spaces between name and description",
			doc:  "@label method    HTTP method used for the request",
			wantAnn: Annotations{
				Labels: map[string]string{"method": "HTTP method used for the request"},
			},
			wantWarnings: 0,
		},
		{
			name: "lines with leading // stripped",
			doc: `// @metric description via slash-prefix
// @label method the verb`,
			wantAnn: Annotations{
				Description: strPtr("via slash-prefix"),
				Labels:      map[string]string{"method": "the verb"},
			},
			wantWarnings: 0,
		},
		{
			name: "empty @metric directive (no keyword)",
			doc:  "@metric",
			wantAnn: Annotations{},
			wantWarnings: 1,
		},
		{
			name: "description with colon and punctuation preserved",
			doc:  "@metric description Connections: active vs. idle (rolling 1m)",
			wantAnn: Annotations{
				Description: strPtr("Connections: active vs. idle (rolling 1m)"),
			},
			wantWarnings: 0,
		},
		{
			name:         "case-sensitive directive names silently skipped",
			doc:          "@Metric description foo\n@LABEL method desc",
			wantAnn:      Annotations{},
			wantWarnings: 0,
		},
		{
			name: "internal whitespace preserved in description",
			doc:  "@metric description  a   b    c  ",
			wantAnn: Annotations{
				Description: strPtr("a   b    c"),
			},
			wantWarnings: 0,
		},
		{
			name: "unicode in description and label desc",
			doc:  "@metric description Запросы в минуту 📊\n@label method HTTP-метод: GET / POST",
			wantAnn: Annotations{
				Description: strPtr("Запросы в минуту 📊"),
				Labels:      map[string]string{"method": "HTTP-метод: GET / POST"},
			},
			wantWarnings: 0,
		},
		{
			name: "description with quotes, slashes, braces",
			doc:  `@metric description Use {"x": 42} or \"quoted\" form`,
			wantAnn: Annotations{
				Description: strPtr(`Use {"x": 42} or \"quoted\" form`),
			},
			wantWarnings: 0,
		},
		{
			name:         "line with only slashes yields empty",
			doc:          "//\n// \n///\n",
			wantAnn:      Annotations{},
			wantWarnings: 0,
		},
		{
			name: "two paragraphs after two directives — one warning per paragraph",
			doc: `@metric description first para
continuation of first
@metric calculation second para
continuation of second`,
			wantAnn: Annotations{
				Description: strPtr("first para"),
				Calculation: strPtr("second para"),
			},
			wantWarnings: 2, // one per continuation
		},
		{
			name: "duplicate directive keeps tracker — continuation after duplicate fires",
			doc: `@metric description first
@metric description second
continuation after dup`,
			wantAnn: Annotations{
				Description: strPtr("second"),
			},
			wantWarnings: 2, // one "duplicate @metric description", one "continuation"
		},
		{
			name: "triple-slash line after directive — treated as blank, no continuation warning",
			doc: `@metric description foo
///
@metric calculation bar`,
			wantAnn: Annotations{
				Description: strPtr("foo"),
				Calculation: strPtr("bar"),
			},
			wantWarnings: 0, // /// normalizes to empty, blank-line reset
		},
		{
			name: "directives separated by plain text and blank lines",
			doc: `@metric description foo

Plain prose.
Another line.

@metric calculation bar`,
			wantAnn: Annotations{
				Description: strPtr("foo"),
				Calculation: strPtr("bar"),
			},
			wantWarnings: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := SwagStyleParser{}
			ann, warns := p.Parse(tc.doc)

			if diff := cmp.Diff(tc.wantAnn, ann); diff != "" {
				t.Errorf("annotations mismatch (-want +got):\n%s", diff)
			}

			switch {
			case tc.wantWarnings == -1:
				if len(warns) < 1 {
					t.Errorf("want >=1 warnings, got 0")
				}
			default:
				if got := len(warns); got != tc.wantWarnings {
					t.Errorf("warning count: want %d, got %d (%v)", tc.wantWarnings, got, warns)
				}
			}
		})
	}
}

// TestSwagStyleParser_Parse_DuplicateDescriptionWarningContent asserts the
// warning text surfaces the relevant keyword so operators can grep for it.
func TestSwagStyleParser_Parse_DuplicateDescriptionWarningContent(t *testing.T) {
	p := SwagStyleParser{}
	doc := "@metric description a\n@metric description b"
	_, warns := p.Parse(doc)
	if len(warns) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "duplicate") || !strings.Contains(warns[0], "description") {
		t.Errorf("warning should mention 'duplicate' and 'description'; got %q", warns[0])
	}
}

// TestSwagStyleParser_Parse_InterfaceSatisfied compile-time asserts that
// SwagStyleParser satisfies the AnnotationParser interface.
func TestSwagStyleParser_Parse_InterfaceSatisfied(t *testing.T) {
	var _ AnnotationParser = SwagStyleParser{}
}

// TestSwagStyleParser_Parse_ContinuationWarningContent asserts the continuation
// warning text mentions the likely cause so authors can diagnose why only the
// first line of their description was captured.
func TestSwagStyleParser_Parse_ContinuationWarningContent(t *testing.T) {
	p := SwagStyleParser{}
	doc := `@metric description foo
bar baz`
	_, warns := p.Parse(doc)
	if len(warns) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "possible multi-line continuation") {
		t.Errorf("warning should mention 'possible multi-line continuation'; got %q", warns[0])
	}
	if !strings.Contains(warns[0], "@metric description") {
		t.Errorf("warning should mention the offending directive '@metric description'; got %q", warns[0])
	}
}

// TestSwagStyleParser_Parse_ContinuationWarningMentionsLabel asserts the
// warning names the specific @label directive (including label name) so an
// author can find which label's description was truncated.
func TestSwagStyleParser_Parse_ContinuationWarningMentionsLabel(t *testing.T) {
	p := SwagStyleParser{}
	doc := `@label method HTTP verb
wrapped to second line`
	_, warns := p.Parse(doc)
	if len(warns) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "@label method") {
		t.Errorf("warning should mention the offending directive '@label method'; got %q", warns[0])
	}
}
