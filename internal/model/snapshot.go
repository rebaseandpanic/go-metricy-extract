// Package model defines the JSON snapshot shape produced by go-metricy-extract.
package model

import (
	"encoding/json"
	"time"
)

// SchemaVersion identifies the output JSON contract version.
// Bump the major component on breaking changes to consumers;
// the minor component for additive, backward-compatible fields.
const SchemaVersion = "1.0"

// ExtractorName identifies this tool in the snapshot's extractor block.
const ExtractorName = "go-metricy-extract"

// MetricSnapshot is the root document emitted by the extractor.
type MetricSnapshot struct {
	SchemaVersion string             `json:"schema_version"`
	Project       string             `json:"project"`
	ExtractedAt   time.Time          `json:"extracted_at"`
	Extractor     ExtractorInfo      `json:"extractor"`
	Metrics       []MetricDescriptor `json:"metrics"`
}

// extractedAtLayout is the wire format for MetricSnapshot.ExtractedAt.
// Second-precision ISO-8601 UTC; golden-file tests and consumer diffs
// depend on it staying stable.
const extractedAtLayout = "2006-01-02T15:04:05Z"

// MarshalJSON normalizes nil Metrics to an empty array and pins
// ExtractedAt to second-precision ISO-8601 UTC (default time.Time
// marshaling uses RFC3339Nano, which breaks byte-for-byte determinism
// across runs).
func (s MetricSnapshot) MarshalJSON() ([]byte, error) {
	metrics := s.Metrics
	if metrics == nil {
		metrics = []MetricDescriptor{}
	}
	return json.Marshal(struct {
		SchemaVersion string             `json:"schema_version"`
		Project       string             `json:"project"`
		ExtractedAt   string             `json:"extracted_at"`
		Extractor     ExtractorInfo      `json:"extractor"`
		Metrics       []MetricDescriptor `json:"metrics"`
	}{
		SchemaVersion: s.SchemaVersion,
		Project:       s.Project,
		ExtractedAt:   s.ExtractedAt.UTC().Format(extractedAtLayout),
		Extractor:     s.Extractor,
		Metrics:       metrics,
	})
}

// ExtractorInfo identifies the tool that produced the snapshot.
type ExtractorInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MetricDescriptor describes a single Prometheus metric.
type MetricDescriptor struct {
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	Help           string            `json:"help"`
	Description    *string           `json:"description"`
	Calculation    *string           `json:"calculation"`
	Labels         []LabelDescriptor `json:"labels"`
	SourceLocation *SourceLocation   `json:"source_location,omitempty"`
}

// MarshalJSON normalizes nil Labels to an empty slice so consumers always
// see `"labels":[]` instead of `"labels":null`.
func (m MetricDescriptor) MarshalJSON() ([]byte, error) {
	type alias MetricDescriptor
	if m.Labels == nil {
		m.Labels = []LabelDescriptor{}
	}
	return json.Marshal(alias(m))
}

// LabelDescriptor describes a single Prometheus label on a metric.
type LabelDescriptor struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// SourceLocation points back to the declaration site of a metric in Go source.
type SourceLocation struct {
	File   string  `json:"file"`
	Line   *int    `json:"line"`
	Class  *string `json:"class"`
	Member *string `json:"member"`
}
