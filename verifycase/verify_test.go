package verifycase

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/platform"
	"github.com/wyw14/cry-103/internal/route"
	"github.com/wyw14/cry-103/internal/switchover"
	"github.com/wyw14/cry-103/internal/topology"
)

func TestProtectionActivationSurvivesDrainDeadline(t *testing.T) {
	registry := topology.NewRegistry()
	if _, err := registry.Register("S3", topology.PathPrimary, time.Now()); err != nil {
		t.Fatal(err)
	}
	target, err := route.New("R-S3", "S3", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	service := switchover.NewSessionService(
		route.NewDrainer(30*time.Millisecond), route.NewActivator(registry),
		platform.UUIDGenerator{}, time.Now, 2*time.Millisecond,
	)
	result, err := service.Failover(context.Background(), target)
	if err != nil {
		t.Fatalf("protection activation was canceled by drain deadline: %v", err)
	}
	if result.State != switchover.StateProtected || target.Snapshot().State != route.StateProtectionActive {
		t.Fatalf("protection route did not recover traffic: operation=%+v route=%+v", result, target.Snapshot())
	}
}
