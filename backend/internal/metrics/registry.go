package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type Registry struct {
	mu        sync.RWMutex
	counters  map[string]float64
	durations map[string][]float64
}

var Default = New()

func New() *Registry {
	return &Registry{counters: map[string]float64{}, durations: map[string][]float64{}}
}
func key(name string, labels map[string]string) string {
	parts := []string{}
	for k, v := range labels {
		parts = append(parts, k+`="`+strings.NewReplacer(`\`, `\\`, `"`, `\"`, `\n`, ``).Replace(v)+`"`)
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return name
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}
func (r *Registry) Inc(name string, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[key(name, labels)]++
}
func (r *Registry) Observe(name string, labels map[string]string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(name+"_milliseconds", labels)
	r.durations[k] = append(r.durations[k], float64(d.Microseconds())/1000)
}
func (r *Registry) Prometheus() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(r.counters)+len(r.durations))
	for k := range r.counters {
		keys = append(keys, k)
	}
	for k := range r.durations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		if v, ok := r.counters[k]; ok {
			fmt.Fprintf(&b, "%s %g\n", k, v)
			continue
		}
		values := r.durations[k]
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		fmt.Fprintf(&b, "%s %d\n%s %g\n", suffix(k, "_count"), len(values), suffix(k, "_sum"), sum)
	}
	return b.String()
}

func suffix(metric, value string) string {
	if at := strings.IndexByte(metric, '{'); at >= 0 {
		return metric[:at] + value + metric[at:]
	}
	return metric + value
}
