package verifycase

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/feed"
	"github.com/wyw14/cry-103/internal/repeater"
	"github.com/wyw14/cry-103/internal/store"
	"github.com/wyw14/cry-103/internal/telemetry"
)

func TestRampRecoveryStartsAfterLastStableStep(t *testing.T) {
	now := time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC)
	repository := telemetry.NewRepository()
	for sequence, value := range []float64{10.1, 10.3} {
		if err := repository.Add(telemetry.Sample{
			ID: "current", AssetID: "R18", Kind: telemetry.KindCurrent, Value: value,
			Sequence: uint64(sequence + 1), ObservedAt: now.Add(time.Duration(sequence) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}
	checkpoints := store.NewCheckpoints(store.NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots")))
	controller := repeater.NewRampController(
		feed.NewActuator(), repository,
		telemetry.StabilityWindow{MinimumSamples: 2, MaximumSpread: 1, MinimumPeriod: time.Second},
		checkpoints, func() time.Time { return now },
	)
	target, err := repeater.New("R18", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ApplyStep(context.Background(), target, repeater.RampStep{Number: 1, Voltage: 700}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.ApplyStep(context.Background(), target, repeater.RampStep{Number: 2, Voltage: 900}, 2); err == nil {
		t.Fatal("unstable second ramp step unexpectedly completed")
	}
	checkpoint, found, err := checkpoints.LoadRamp("R18")
	if err != nil || !found {
		t.Fatalf("load ramp checkpoint: found=%v err=%v", found, err)
	}
	if checkpoint.StableStep != 1 {
		t.Fatalf("recovery cursor advanced before current stability: %+v", checkpoint)
	}
}
