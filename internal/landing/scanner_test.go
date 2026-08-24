package landing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/topology"
)

// TestScanPinnedStampsSinglePath confirms a scan that completes without a
// concurrent switchover produces chunks that all share the pinned path and
// generation, even when the scan straddles multiple chunk reads.
func TestScanPinnedStampsSinglePath(t *testing.T) {
	registry := topology.NewRegistry()
	snapshot, err := registry.Register("S3", topology.PathPrimary, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(registry, 0)
	chunks, err := scanner.ScanPinned(context.Background(), snapshot, "east", 4)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(chunks) != 4 {
		t.Fatalf("expected 4 chunks, got %d", len(chunks))
	}
	for _, chunk := range chunks {
		if chunk.Path != topology.PathPrimary {
			t.Fatalf("chunk pinned to wrong path: %s", chunk.Path)
		}
		if chunk.Generation != snapshot.Generation {
			t.Fatalf("chunk pinned to wrong generation: %d", chunk.Generation)
		}
	}
}

// TestScanPinnedAbortsOnConcurrentSwitchover reproduces the South China Sea S3
// scenario: the first half of a scan runs on the primary path, a protection
// switchover lands mid-scan, and the scan must refuse to publish a trajectory
// that straddles two physical paths. Instead of mixing chunks 40+km apart, the
// scan aborts with ErrTopologyChanged so the caller can re-pin and retry.
func TestScanPinnedAbortsOnConcurrentSwitchover(t *testing.T) {
	registry := topology.NewRegistry()
	snapshot, err := registry.Register("S3", topology.PathPrimary, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// A small per-chunk delay gives the switchover room to land mid-scan.
	scanner := NewScanner(registry, 2*time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Open primary, then connect+switch to protection, mirroring the real
		// break-before-make cross-connect that a protection failover performs.
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

	chunks, err := scanner.ScanPinned(context.Background(), snapshot, "east", 6)
	wg.Wait()
	if err == nil {
		t.Fatalf("expected scan to abort mid-switchover, got chunks: %+v", chunks)
	}
	if !errors.Is(err, ErrTopologyChanged) {
		t.Fatalf("expected ErrTopologyChanged, got %v", err)
	}
	if chunks != nil {
		t.Fatalf("aborted scan must not return partial chunks: %+v", chunks)
	}
}

// TestScanPinnedReplaysProtectionDistanceOnce confirms that after a switchover
// settles, a fresh pinned scan stamps the protection path consistently: every
// chunk carries the protection generation and the 42km offset is applied. This
// is the "rescan leaves only one peak" half of the report.
func TestScanPinnedReplaysProtectionDistanceOnce(t *testing.T) {
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
	protection, err := registry.Switch("S3", topology.PathProtection, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	scanner := NewScanner(registry, 0)
	chunks, err := scanner.ScanPinned(context.Background(), protection, "east", 3)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	for _, chunk := range chunks {
		if chunk.Path != topology.PathProtection {
			t.Fatalf("chunk on wrong path: %s", chunk.Path)
		}
		if chunk.Generation != protection.Generation {
			t.Fatalf("chunk on wrong generation: %d", chunk.Generation)
		}
		if chunk.DistanceKM < 42 {
			t.Fatalf("protection chunk missing 42km offset: %f", chunk.DistanceKM)
		}
	}
}
