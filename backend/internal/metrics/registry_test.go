package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestPrometheusUsesStableLabels(t *testing.T) {
	r := New()
	r.Inc("requests_total", map[string]string{"route": "/cases/:id"})
	r.Observe("latency", map[string]string{"route": "/cases/:id"}, 2*time.Millisecond)
	out := r.Prometheus()
	for _, wanted := range []string{`requests_total{route="/cases/:id"} 1`, `latency_milliseconds_count{route="/cases/:id"} 1`} {
		if !strings.Contains(out, wanted) {
			t.Fatalf("missing %q in %s", wanted, out)
		}
	}
}
