package verifycase

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/otdr"
	"github.com/wyw14/cry-103/internal/platform"
	"github.com/wyw14/cry-103/internal/repair"
	"github.com/wyw14/cry-103/internal/route"
	"github.com/wyw14/cry-103/internal/span"
	"github.com/wyw14/cry-103/internal/switchover"
	"github.com/wyw14/cry-103/internal/topology"
)

func TestSpliceAcceptanceRequiresBothDirections(t *testing.T) {
	now := time.Now
	registry := topology.NewRegistry()
	if _, err := registry.Register("S3", topology.PathPrimary, now()); err != nil {
		t.Fatal(err)
	}
	path, err := route.New("R-S3", "S3", now())
	if err != nil {
		t.Fatal(err)
	}
	activator := route.NewActivator(registry)
	if _, err := activator.Protection(context.Background(), path, now()); err != nil {
		t.Fatal(err)
	}
	cable, err := span.New("S3", "S3", 126.4, now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cable.MarkCut(now()); err != nil {
		t.Fatal(err)
	}
	if _, err := cable.StartRepair(now()); err != nil {
		t.Fatal(err)
	}
	if _, err := cable.BeginProof(now()); err != nil {
		t.Fatal(err)
	}
	restorer := switchover.NewRestorer(route.NewDrainer(0), activator, registry, platform.UUIDGenerator{}, now)
	service := repair.NewAcceptanceService(restorer, now)
	service.Register("S3", cable, path)
	result, err := service.AddProof(context.Background(), "acceptance-1", "S3", otdr.Proof{
		ID: "east-proof", Direction: otdr.DirectionEastToWest, Passed: true, MeasuredAt: now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Restored || result.Proofs.Complete || cable.Snapshot().State == span.StateRecovered {
		t.Fatalf("one-way proof finalized splice acceptance: result=%+v span=%+v", result, cable.Snapshot())
	}
}
