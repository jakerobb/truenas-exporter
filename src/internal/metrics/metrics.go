// Package metrics collects from TrueNAS and renders the Prometheus text
// exposition format directly (no client_golang dependency; the format is
// simple and this exporter only emits gauges).
package metrics

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jakerobb/truenas-exporter/internal/truenas"
	"github.com/jakerobb/truenas-exporter/internal/util"
)

// Snapshot is the result of one collection.
type Snapshot struct {
	Up       bool
	Duration time.Duration
	Pools    []truenas.Pool
	Usages   []truenas.PoolUsage
}

// Collector queries TrueNAS on demand, once per scrape.
type Collector struct {
	opts    truenas.Options
	timeout time.Duration
}

func NewCollector(opts truenas.Options, timeout time.Duration) *Collector {
	return &Collector{opts: opts, timeout: timeout}
}

// Collect opens a session, queries pools, and closes it. Errors are logged
// and reported as truenas_up 0 rather than failing the scrape.
func (c *Collector) Collect(ctx context.Context) Snapshot {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	snap, err := c.collect(ctx)
	snap.Duration = time.Since(start)
	if err != nil {
		slog.Error("collection failed", "err", err, "duration", snap.Duration)
		return Snapshot{Up: false, Duration: snap.Duration}
	}
	slog.Debug("collection succeeded", "pools", len(snap.Pools), "duration", snap.Duration)
	snap.Up = true
	return snap
}

func (c *Collector) collect(ctx context.Context) (Snapshot, error) {
	client, err := truenas.Dial(ctx, c.opts)
	if err != nil {
		return Snapshot{}, err
	}
	defer util.CloseCleanly(client)

	pools, err := client.Pools(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	names := make([]string, len(pools))
	for i, p := range pools {
		names[i] = p.Name
	}
	usages, err := client.PoolUsages(ctx, names)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Pools: pools, Usages: usages}, nil
}

// Write renders the snapshot in Prometheus text format, with pools sorted by
// name so output is stable.
func Write(w io.Writer, s Snapshot) error {
	pools := append([]truenas.Pool(nil), s.Pools...)
	sort.Slice(pools, func(i, j int) bool { return pools[i].Name < pools[j].Name })
	usages := append([]truenas.PoolUsage(nil), s.Usages...)
	sort.Slice(usages, func(i, j int) bool { return usages[i].Name < usages[j].Name })

	ew := &errWriter{w: w}

	ew.header("truenas_up", "Whether the last query of the TrueNAS API succeeded.")
	ew.sample("truenas_up", nil, boolFloat(s.Up))
	ew.header("truenas_scrape_duration_seconds", "Time taken to query the TrueNAS API.")
	ew.sample("truenas_scrape_duration_seconds", nil, s.Duration.Seconds())

	if len(pools) > 0 {
		ew.header("truenas_pool_status", "Pool status as reported by ZFS (ONLINE, DEGRADED, ...); always 1.")
		for _, p := range pools {
			ew.sample("truenas_pool_status", []label{{"pool", p.Name}, {"status", p.Status}}, 1)
		}
		ew.header("truenas_pool_healthy", "Whether TrueNAS considers the pool healthy.")
		for _, p := range pools {
			ew.sample("truenas_pool_healthy", []label{{"pool", p.Name}}, boolFloat(p.Healthy))
		}
		rawGauges := []struct {
			name, help string
			value      func(truenas.Pool) *uint64
		}{
			{"truenas_pool_raw_size_bytes", "Raw pool capacity, including parity.", func(p truenas.Pool) *uint64 { return p.Size }},
			{"truenas_pool_raw_allocated_bytes", "Raw space allocated in the pool, including parity.", func(p truenas.Pool) *uint64 { return p.Allocated }},
			{"truenas_pool_raw_free_bytes", "Raw space free in the pool, including parity.", func(p truenas.Pool) *uint64 { return p.Free }},
		}
		for _, g := range rawGauges {
			ew.header(g.name, g.help)
			for _, p := range pools {
				if v := g.value(p); v != nil {
					ew.sample(g.name, []label{{"pool", p.Name}}, float64(*v))
				}
			}
		}
	}

	if len(usages) > 0 {
		ew.header("truenas_pool_used_bytes", "Usable space used in the pool (root dataset), as shown in the TrueNAS UI.")
		for _, u := range usages {
			ew.sample("truenas_pool_used_bytes", []label{{"pool", u.Name}}, float64(u.Used))
		}
		ew.header("truenas_pool_available_bytes", "Usable space available in the pool (root dataset), as shown in the TrueNAS UI.")
		for _, u := range usages {
			ew.sample("truenas_pool_available_bytes", []label{{"pool", u.Name}}, float64(u.Available))
		}
	}

	return ew.err
}

type label struct{ name, value string }

// errWriter keeps the first write error so Write needn't check every line.
type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) printf(format string, args ...any) {
	if ew.err == nil {
		_, ew.err = fmt.Fprintf(ew.w, format, args...)
	}
}

func (ew *errWriter) header(name, help string) {
	ew.printf("# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
}

func (ew *errWriter) sample(name string, labels []label, value float64) {
	if len(labels) == 0 {
		ew.printf("%s %g\n", name, value)
		return
	}
	parts := make([]string, len(labels))
	for i, l := range labels {
		parts[i] = fmt.Sprintf(`%s="%s"`, l.name, escapeLabelValue(l.value))
	}
	ew.printf("%s{%s} %g\n", name, strings.Join(parts, ","), value)
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escapeLabelValue(s string) string {
	return labelEscaper.Replace(s)
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
