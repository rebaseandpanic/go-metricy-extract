package rules

import (
	"fmt"

	"github.com/rebaseandpanic/go-metricy-extract/internal/model"
)

// metricLabel returns a human-readable description of a metric for
// inclusion in violation messages. Returns `metric "foo"` when the metric
// has a name, and the bare word `metric` when the name is empty so the
// message does not render as `metric ""` with stray quotes around an
// empty string.
func metricLabel(m model.MetricDescriptor) string {
	if m.Name == "" {
		return "metric"
	}
	return fmt.Sprintf("metric %q", m.Name)
}
