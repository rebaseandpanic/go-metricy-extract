package model

import "sort"

// SortMetrics sorts metrics in place by Name using byte-wise comparison
// for deterministic output across locales.
func SortMetrics(metrics []MetricDescriptor) {
	sort.SliceStable(metrics, func(i, j int) bool {
		return metrics[i].Name < metrics[j].Name
	})
}

// SortLabels sorts labels in place by Name using byte-wise comparison.
func SortLabels(labels []LabelDescriptor) {
	sort.SliceStable(labels, func(i, j int) bool {
		return labels[i].Name < labels[j].Name
	})
}
