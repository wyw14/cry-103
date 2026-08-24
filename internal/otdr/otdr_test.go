package otdr

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/landing"
	"github.com/wyw14/cry-103/internal/platform"
	"github.com/wyw14/cry-103/internal/topology"
)

func TestProofSetWaitsForConfiguredDirections(t *testing.T) {
	set := NewProofSet()
	result, err := set.Add(Proof{ID: "east", Direction: DirectionEastToWest, Passed: true, MeasuredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatal("one proof must leave the set incomplete")
	}
	result, err = set.Add(Proof{ID: "west", Direction: DirectionWestToEast, Passed: true, MeasuredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || !result.Passed {
		t.Fatalf("unexpected proof result: %+v", result)
	}
}

// TestRunNeverPublishesSwitchoverStraddledTrace is the regression test for the
// South China Sea S3 incident. A protection switchover lands while an OTDR scan
// is in flight. Before the fix the first chunks came from the primary path and
// the trailing chunks from the protection path, but the service still published
// them as one trace and reported a fault 40+km off. After the fix Run must
// either publish a trace whose chunks all share one path and one generation,
// or refuse to publish at all.
func TestRunNeverPublishesSwitchoverStraddledTrace(t *testing.T) {
	registry := topology.NewRegistry()
	if _, err := registry.Register("S3", topology.PathPrimary, time.Now()); err != nil {
		t.Fatal(err)
	}
	ids := platform.UUIDGenerator{}
	clock := fixedScanClock{now: time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)}
	// A per-chunk delay gives the concurrent failover room to land mid-scan.
	scanner := landing.NewScanner(registry, 2*time.Millisecond)
	service := NewSessionService(registry, scanner, ids, clock.Now)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := registry.Open("S3", topology.PathPrimary, time.Now()); err != nil {
			t.Errorf("open primary: %v", err)
			return
		}
		if _, err := registry.Connect("S3", topology.PathProtection, time.Now()); err != nil {
			t.Errorf("connect protection: %v", err)
			return
		}
		if _, err := registry.Switch("S3", topology.PathProtection, time.Now()); err != nil {
			t.Errorf("switch protection: %v", err)
		}
	}()

	trace, err := service.Run(context.Background(), "S3", "east", 126, 6)
	wg.Wait()
	if err != nil {
		// An abort is an acceptable outcome; the important guarantee is that no
		// straddled trace is ever published. Nothing to assert further.
		return
	}
	// A published trace must correspond to exactly one physical path.
	generations := trace.Generations()
	if len(generations) != 1 {
		t.Fatalf("published trace spans multiple generations: %v", generations)
	}
	for _, chunk := range trace.Chunks {
		if chunk.Path != trace.Path {
			t.Fatalf("chunk path %s disagrees with trace path %s", chunk.Path, trace.Path)
		}
		if chunk.Generation != trace.Generation {
			t.Fatalf("chunk generation %d disagrees with trace generation %d", chunk.Generation, trace.Generation)
		}
	}
	// No pair of reflections may straddle the 42km gap between the primary and
	// protection routes. If it did, the scan merged two physical paths.
	var min, max float64
	for i, chunk := range trace.Chunks {
		if i == 0 || chunk.DistanceKM < min {
			min = chunk.DistanceKM
		}
		if chunk.DistanceKM > max {
			max = chunk.DistanceKM
		}
	}
	if max-min > 40 {
		t.Fatalf("published trace straddles primary/protection distance gap: min=%f max=%f", min, max)
	}
}

// TestRunPublishesConsistentProtectionTrace confirms that once a switchover has
// settled, Run re-pins against the protection snapshot and publishes a trace
// whose chunks all belong to the protection path.
func TestRunPublishesConsistentProtectionTrace(t *testing.T) {
	registry := topology.NewRegistry()
	if _, err := registry.Register("S3", topology.PathPrimary, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Open("S3", topology.PathPrimary, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Connect("S3", topology.PathProtection, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Switch("S3", topology.PathProtection, time.Now()); err != nil {
		t.Fatal(err)
	}
	ids := platform.UUIDGenerator{}
	clock := fixedScanClock{now: time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)}
	scanner := landing.NewScanner(registry, 0)
	service := NewSessionService(registry, scanner, ids, clock.Now)

	trace, err := service.Run(context.Background(), "S3", "east", 126, 3)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if trace.Path != topology.PathProtection {
		t.Fatalf("expected protection trace, got path %s", trace.Path)
	}
	if len(trace.Generations()) != 1 {
		t.Fatalf("protection trace spans multiple generations: %v", trace.Generations())
	}
	for _, chunk := range trace.Chunks {
		if chunk.Path != topology.PathProtection {
			t.Fatalf("chunk on wrong path: %s", chunk.Path)
		}
	}
}

type fixedScanClock struct {
	now time.Time
}

func (c fixedScanClock) Now() time.Time { return c.now }
