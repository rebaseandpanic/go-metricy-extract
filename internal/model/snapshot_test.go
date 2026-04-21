package model

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func fullSnapshot() MetricSnapshot {
	return MetricSnapshot{
		SchemaVersion: SchemaVersion,
		Project:       "MyService",
		ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		Extractor: ExtractorInfo{
			Name:    ExtractorName,
			Version: "0.1.0-dev",
		},
		Metrics: []MetricDescriptor{
			{
				Name:        "http_requests_total",
				Type:        "counter",
				Help:        "Total HTTP requests processed",
				Description: strPtr("Total incoming HTTP requests across all endpoints."),
				Calculation: strPtr("Incremented in LoggingMiddleware on each completed request."),
				Labels: []LabelDescriptor{
					{Name: "method", Description: strPtr("HTTP method: GET, POST, PUT, DELETE")},
					{Name: "status_code", Description: strPtr("HTTP response status code")},
				},
				SourceLocation: &SourceLocation{
					File:   "internal/metrics/metrics.go",
					Line:   intPtr(17),
					Class:  strPtr(""),
					Member: strPtr("HttpRequests"),
				},
			},
		},
	}
}

func TestMetricSnapshot_JSONRoundtrip(t *testing.T) {
	original := fullSnapshot()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var round MetricSnapshot
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if diff := cmp.Diff(original, round); diff != "" {
		t.Errorf("roundtrip mismatch (-want +got):\n%s", diff)
	}
}

func TestMetricSnapshot_JSON_EmptyDescriptionIsNull(t *testing.T) {
	snap := MetricSnapshot{
		SchemaVersion: SchemaVersion,
		Project:       "svc",
		ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		Extractor:     ExtractorInfo{Name: ExtractorName, Version: "0.1.0-dev"},
		Metrics: []MetricDescriptor{
			{
				Name:        "foo_total",
				Type:        "counter",
				Help:        "h",
				Description: nil,
				Calculation: nil,
				Labels: []LabelDescriptor{
					{Name: "l1", Description: nil},
				},
				SourceLocation: &SourceLocation{
					File:   "a.go",
					Line:   intPtr(1),
					Class:  nil,
					Member: nil,
				},
			},
		},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)

	mustContain := []string{
		`"description":null`,
		`"calculation":null`,
		`"class":null`,
		`"member":null`,
	}
	for _, s := range mustContain {
		if !strings.Contains(got, s) {
			t.Errorf("expected JSON to contain %q, got:\n%s", s, got)
		}
	}

	// labels[].description should also be explicit null — check structurally
	// rather than via substring match, since the metric-level "description":null
	// already satisfies Contains and hides whether the label-level field is set.
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal for structural check: %v", err)
	}
	metrics, ok := decoded["metrics"].([]any)
	if !ok || len(metrics) == 0 {
		t.Fatalf("expected metrics array, got %T: %v", decoded["metrics"], decoded["metrics"])
	}
	metric, ok := metrics[0].(map[string]any)
	if !ok {
		t.Fatalf("expected metric object, got %T", metrics[0])
	}
	labels, ok := metric["labels"].([]any)
	if !ok || len(labels) == 0 {
		t.Fatalf("expected labels array, got %T: %v", metric["labels"], metric["labels"])
	}
	label, ok := labels[0].(map[string]any)
	if !ok {
		t.Fatalf("expected label object, got %T", labels[0])
	}
	desc, present := label["description"]
	if !present {
		t.Errorf("expected labels[0].description key to be present, got %v", label)
	}
	if desc != nil {
		t.Errorf("expected labels[0].description to be JSON null (nil after unmarshal), got %T: %v", desc, desc)
	}
}

func TestMetricSnapshot_JSON_NilMetrics_SerializedAsEmptyArray(t *testing.T) {
	snap := MetricSnapshot{
		SchemaVersion: SchemaVersion,
		Project:       "svc",
		ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		Extractor:     ExtractorInfo{Name: ExtractorName, Version: "0.1.0-dev"},
		Metrics:       nil,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)

	if !strings.Contains(got, `"metrics":[]`) {
		t.Errorf("expected metrics to serialize as [], got:\n%s", got)
	}
	if strings.Contains(got, `"metrics":null`) {
		t.Errorf("expected metrics not to be null, got:\n%s", got)
	}

	// MarshalJSON must not mutate the receiver.
	if snap.Metrics != nil {
		t.Errorf("MarshalJSON mutated the receiver: Metrics = %v (want nil)", snap.Metrics)
	}
}

func TestMetricDescriptor_JSON_NilLabels_SerializedAsEmptyArray(t *testing.T) {
	m := MetricDescriptor{
		Name:        "foo_total",
		Type:        "counter",
		Help:        "h",
		Description: nil,
		Calculation: nil,
		Labels:      nil,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)

	if !strings.Contains(got, `"labels":[]`) {
		t.Errorf("expected labels to serialize as [], got:\n%s", got)
	}
	if strings.Contains(got, `"labels":null`) {
		t.Errorf("expected labels not to be null, got:\n%s", got)
	}

	// MarshalJSON must not mutate the receiver.
	if m.Labels != nil {
		t.Errorf("MarshalJSON mutated the receiver: Labels = %v (want nil)", m.Labels)
	}
}

func TestMetricSnapshot_JSON_MissingSourceLocationOmitted(t *testing.T) {
	snap := MetricSnapshot{
		SchemaVersion: SchemaVersion,
		Project:       "svc",
		ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		Extractor:     ExtractorInfo{Name: ExtractorName, Version: "0.1.0-dev"},
		Metrics: []MetricDescriptor{
			{
				Name:   "foo_total",
				Type:   "counter",
				Help:   "h",
				Labels: []LabelDescriptor{},
				// SourceLocation intentionally nil
			},
		},
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)

	if strings.Contains(got, `"source_location"`) {
		t.Errorf("expected source_location to be omitted when nil, got:\n%s", got)
	}
}

func TestMetricSnapshot_JSON_MissingLineOmittedAsNull(t *testing.T) {
	// Line is *int without omitempty — nil must serialize as null (not dropped).
	sl := SourceLocation{
		File:   "a.go",
		Line:   nil,
		Class:  nil,
		Member: strPtr("X"),
	}
	data, err := json.Marshal(sl)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, `"line":null`) {
		t.Errorf("expected line:null, got %s", got)
	}
}

func TestSortMetrics_Deterministic(t *testing.T) {
	unsorted := []MetricDescriptor{
		{Name: "z_metric"},
		{Name: "a_metric"},
		{Name: "m_metric"},
		{Name: "b_metric"},
	}
	want := []MetricDescriptor{
		{Name: "a_metric"},
		{Name: "b_metric"},
		{Name: "m_metric"},
		{Name: "z_metric"},
	}

	SortMetrics(unsorted)

	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("SortMetrics result mismatch (-want +got):\n%s", diff)
	}

	// Sorting twice is a no-op.
	SortMetrics(unsorted)
	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("SortMetrics not idempotent (-want +got):\n%s", diff)
	}
}

func TestSortLabels_Deterministic(t *testing.T) {
	unsorted := []LabelDescriptor{
		{Name: "status_code"},
		{Name: "method"},
		{Name: "endpoint"},
	}
	want := []LabelDescriptor{
		{Name: "endpoint"},
		{Name: "method"},
		{Name: "status_code"},
	}

	SortLabels(unsorted)

	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("SortLabels result mismatch (-want +got):\n%s", diff)
	}

	SortLabels(unsorted)
	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("SortLabels not idempotent (-want +got):\n%s", diff)
	}
}

func TestSortMetrics_ByteCompareNotLocale(t *testing.T) {
	// Ensure uppercase sorts before lowercase (byte/ordinal), not Unicode collation.
	unsorted := []MetricDescriptor{
		{Name: "bravo"},
		{Name: "Alpha"},
	}
	want := []MetricDescriptor{
		{Name: "Alpha"}, // 'A' (0x41) < 'b' (0x62)
		{Name: "bravo"},
	}

	SortMetrics(unsorted)

	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("byte-compare ordering mismatch (-want +got):\n%s", diff)
	}
}

func TestMetricSnapshot_JSON_EscapesSpecialCharacters(t *testing.T) {
	// Arbitrary Go source comments may contain quotes, backslashes, CR/LF/tab —
	// Marshal must escape them, and roundtrip must return the exact originals.
	original := MetricSnapshot{
		SchemaVersion: SchemaVersion,
		Project:       "svc",
		ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		Extractor:     ExtractorInfo{Name: ExtractorName, Version: "0.1.0-dev"},
		Metrics: []MetricDescriptor{
			{
				Name:        "weird\"name\\with\nspecials",
				Type:        "counter",
				Help:        "has \"quotes\" and\nnewlines\\and\\backslashes\r\tand\ttabs",
				Description: strPtr("desc with \"escapes\"\nand\\slashes"),
				Calculation: strPtr("calc\twith\r\nCRLF and \"q\""),
				Labels: []LabelDescriptor{
					{
						Name:        "lbl\"odd",
						Description: strPtr("label desc \"quoted\"\nmulti\\line"),
					},
				},
				SourceLocation: &SourceLocation{
					File:   "path\\with\\back\"slashes\n.go",
					Line:   intPtr(42),
					Class:  strPtr(""),
					Member: strPtr("Mem\"ber\tname"),
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var round MetricSnapshot
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if diff := cmp.Diff(original, round); diff != "" {
		t.Errorf("escape roundtrip mismatch (-want +got):\n%s", diff)
	}
}

func TestSortMetrics_StableForDuplicateNames(t *testing.T) {
	// Several entries share Name — their Help order must be preserved (stable sort).
	unsorted := []MetricDescriptor{
		{Name: "a", Help: "first"},
		{Name: "a", Help: "second"},
		{Name: "b", Help: "only"},
		{Name: "a", Help: "third"},
	}
	want := []MetricDescriptor{
		{Name: "a", Help: "first"},
		{Name: "a", Help: "second"},
		{Name: "a", Help: "third"},
		{Name: "b", Help: "only"},
	}

	SortMetrics(unsorted)

	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("SortMetrics not stable for duplicate Names (-want +got):\n%s", diff)
	}
}

func TestSortLabels_StableForDuplicateNames(t *testing.T) {
	unsorted := []LabelDescriptor{
		{Name: "a", Description: strPtr("first")},
		{Name: "a", Description: strPtr("second")},
		{Name: "b", Description: strPtr("only")},
		{Name: "a", Description: strPtr("third")},
	}
	want := []LabelDescriptor{
		{Name: "a", Description: strPtr("first")},
		{Name: "a", Description: strPtr("second")},
		{Name: "a", Description: strPtr("third")},
		{Name: "b", Description: strPtr("only")},
	}

	SortLabels(unsorted)

	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("SortLabels not stable for duplicate Names (-want +got):\n%s", diff)
	}
}

func TestSortMetrics_EmptyAndSingle(t *testing.T) {
	// nil must not panic and must stay nil.
	SortMetrics(nil)

	// Empty slice must not panic and stay empty.
	empty := []MetricDescriptor{}
	SortMetrics(empty)
	if len(empty) != 0 {
		t.Errorf("expected empty slice to stay empty, got len=%d", len(empty))
	}

	// Single-element slice must not panic and must equal the input.
	single := []MetricDescriptor{{Name: "x"}}
	want := []MetricDescriptor{{Name: "x"}}
	SortMetrics(single)
	if diff := cmp.Diff(want, single); diff != "" {
		t.Errorf("SortMetrics single element mismatch (-want +got):\n%s", diff)
	}
}

func TestSortLabels_EmptyAndSingle(t *testing.T) {
	SortLabels(nil)

	empty := []LabelDescriptor{}
	SortLabels(empty)
	if len(empty) != 0 {
		t.Errorf("expected empty slice to stay empty, got len=%d", len(empty))
	}

	single := []LabelDescriptor{{Name: "x"}}
	want := []LabelDescriptor{{Name: "x"}}
	SortLabels(single)
	if diff := cmp.Diff(want, single); diff != "" {
		t.Errorf("SortLabels single element mismatch (-want +got):\n%s", diff)
	}
}

func TestMetricSnapshot_JSON_DeterministicBytes(t *testing.T) {
	// Same snapshot — same JSON bytes on every call. Fixes key-order drift.
	snap := MetricSnapshot{
		SchemaVersion: SchemaVersion,
		Project:       "svc",
		ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		Extractor:     ExtractorInfo{Name: ExtractorName, Version: "0.1.0-dev"},
		Metrics: []MetricDescriptor{
			{
				Name: "a_total",
				Type: "counter",
				Help: "a help",
				Labels: []LabelDescriptor{
					{Name: "x", Description: strPtr("xd")},
					{Name: "y", Description: nil},
				},
			},
			{
				Name:        "b_total",
				Type:        "counter",
				Help:        "b help",
				Description: strPtr("b desc"),
				Labels:      []LabelDescriptor{{Name: "z"}},
			},
		},
	}

	first, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	second, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("second marshal: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("non-deterministic JSON output:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestMetricSnapshot_JSON_UnicodeStrings(t *testing.T) {
	// Cyrillic + emoji + CJK roundtrip untouched.
	original := MetricSnapshot{
		SchemaVersion: SchemaVersion,
		Project:       "svc",
		ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
		Extractor:     ExtractorInfo{Name: ExtractorName, Version: "0.1.0-dev"},
		Metrics: []MetricDescriptor{
			{
				Name:        "requests_total",
				Type:        "counter",
				Help:        "Счётчик запросов 📊 обработаных",
				Description: strPtr("处理请求的计数器"),
				Labels: []LabelDescriptor{
					{Name: "метод", Description: strPtr("HTTP メソッド")},
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var round MetricSnapshot
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if diff := cmp.Diff(original, round); diff != "" {
		t.Errorf("unicode roundtrip mismatch (-want +got):\n%s", diff)
	}
}

func TestSortMetrics_NonASCIIByteOrder(t *testing.T) {
	// 'a' (0x61), 'z' (0x7A), 'б' (0xD0 0xB1 in UTF-8).
	// Byte-order sort puts all ASCII before any multi-byte UTF-8 rune,
	// so the expected order is: a, z, б. If someone swaps `<` for
	// collate.String (which puts Cyrillic between Latin letters), this fails.
	unsorted := []MetricDescriptor{
		{Name: "б"},
		{Name: "a"},
		{Name: "z"},
	}
	want := []MetricDescriptor{
		{Name: "a"},
		{Name: "z"},
		{Name: "б"},
	}

	SortMetrics(unsorted)

	if diff := cmp.Diff(want, unsorted); diff != "" {
		t.Errorf("non-ASCII byte-order sort mismatch (-want +got):\n%s", diff)
	}
}

func TestMetricSnapshot_JSON_ExplicitEmptyMetrics(t *testing.T) {
	// Explicit empty and nil must produce byte-identical JSON, both with "metrics":[].
	base := func() MetricSnapshot {
		return MetricSnapshot{
			SchemaVersion: SchemaVersion,
			Project:       "svc",
			ExtractedAt:   time.Date(2026, 4, 20, 10, 0, 0, 0, time.UTC),
			Extractor:     ExtractorInfo{Name: ExtractorName, Version: "0.1.0-dev"},
		}
	}

	empty := base()
	empty.Metrics = []MetricDescriptor{}

	nilSnap := base()
	nilSnap.Metrics = nil

	emptyBytes, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	nilBytes, err := json.Marshal(nilSnap)
	if err != nil {
		t.Fatalf("marshal nil: %v", err)
	}

	if !strings.Contains(string(emptyBytes), `"metrics":[]`) {
		t.Errorf("empty-slice path: expected metrics:[], got:\n%s", emptyBytes)
	}
	if !bytes.Equal(emptyBytes, nilBytes) {
		t.Errorf("nil and empty Metrics produced different JSON:\nempty: %s\nnil:   %s", emptyBytes, nilBytes)
	}
}
