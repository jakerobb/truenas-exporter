package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/jakerobb/truenas-exporter/internal/truenas"
)

func u64(v uint64) *uint64 { return &v }

func TestWrite(t *testing.T) {
	var b strings.Builder
	err := Write(&b, Snapshot{
		Up:       true,
		Duration: 250 * time.Millisecond,
		Pools: []truenas.Pool{
			{Name: "tank", Status: "ONLINE", Healthy: true, Size: u64(3985729650688), Allocated: u64(1486747058176), Free: u64(2498982592512)},
			{Name: "cold", Status: "OFFLINE"},
		},
		Usages: []truenas.PoolUsage{{Name: "tank", Used: 1488812220416, Available: 2372900249600}},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := `# HELP truenas_up Whether the last query of the TrueNAS API succeeded.
# TYPE truenas_up gauge
truenas_up 1
# HELP truenas_scrape_duration_seconds Time taken to query the TrueNAS API.
# TYPE truenas_scrape_duration_seconds gauge
truenas_scrape_duration_seconds 0.25
# HELP truenas_pool_status Pool status as reported by ZFS (ONLINE, DEGRADED, ...); always 1.
# TYPE truenas_pool_status gauge
truenas_pool_status{pool="cold",status="OFFLINE"} 1
truenas_pool_status{pool="tank",status="ONLINE"} 1
# HELP truenas_pool_healthy Whether TrueNAS considers the pool healthy.
# TYPE truenas_pool_healthy gauge
truenas_pool_healthy{pool="cold"} 0
truenas_pool_healthy{pool="tank"} 1
# HELP truenas_pool_raw_size_bytes Raw pool capacity, including parity.
# TYPE truenas_pool_raw_size_bytes gauge
truenas_pool_raw_size_bytes{pool="tank"} 3.985729650688e+12
# HELP truenas_pool_raw_allocated_bytes Raw space allocated in the pool, including parity.
# TYPE truenas_pool_raw_allocated_bytes gauge
truenas_pool_raw_allocated_bytes{pool="tank"} 1.486747058176e+12
# HELP truenas_pool_raw_free_bytes Raw space free in the pool, including parity.
# TYPE truenas_pool_raw_free_bytes gauge
truenas_pool_raw_free_bytes{pool="tank"} 2.498982592512e+12
# HELP truenas_pool_used_bytes Usable space used in the pool (root dataset), as shown in the TrueNAS UI.
# TYPE truenas_pool_used_bytes gauge
truenas_pool_used_bytes{pool="tank"} 1.488812220416e+12
# HELP truenas_pool_available_bytes Usable space available in the pool (root dataset), as shown in the TrueNAS UI.
# TYPE truenas_pool_available_bytes gauge
truenas_pool_available_bytes{pool="tank"} 2.3729002496e+12
`
	if got := b.String(); got != want {
		t.Errorf("unexpected output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestWriteDown(t *testing.T) {
	var b strings.Builder
	if err := Write(&b, Snapshot{Up: false, Duration: time.Second}); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if !strings.Contains(got, "truenas_up 0\n") || strings.Contains(got, "truenas_pool") {
		t.Errorf("unexpected output for down snapshot:\n%s", got)
	}
}

func TestEscapeLabelValue(t *testing.T) {
	if got, want := escapeLabelValue("a\"b\\c\nd"), `a\"b\\c\nd`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
