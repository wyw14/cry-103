package verifycase

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/landing"
	"github.com/wyw14/cry-103/internal/otdr"
	"github.com/wyw14/cry-103/internal/platform"
	"github.com/wyw14/cry-103/internal/topology"
)

func TestOTDRTraceUsesSingleTopologyGeneration(t *testing.T) {
	registry := topology.NewRegistry()
	if _, err := registry.Register("S3", topology.PathPrimary, time.Now()); err != nil {
		t.Fatal(err)
	}
	service := otdr.NewSessionService(registry, landing.NewScanner(registry, 10*time.Millisecond), platform.UUIDGenerator{}, time.Now)
	type result struct {
		trace otdr.Trace
		err   error
	}
	done := make(chan result, 1)
	go func() {
		trace, err := service.Run(context.Background(), "S3", "east", 126.4, 5)
		done <- result{trace: trace, err: err}
	}()
	time.Sleep(16 * time.Millisecond)
	if _, err := registry.Switch("S3", topology.PathProtection, time.Now()); err != nil {
		t.Fatal(err)
	}
	resultValue := <-done
	if resultValue.err != nil {
		return
	}
	if generations := resultValue.trace.Generations(); len(generations) != 1 {
		t.Fatalf("published OTDR trace crossed topology generations: %v", generations)
	}
}
