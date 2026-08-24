package repeater

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/feed"
	"github.com/wyw14/cry-103/internal/store"
	"github.com/wyw14/cry-103/internal/telemetry"
)

// stableWindow requires at least two current samples within a tight spread so a
// settling ramp is distinguishable from a settled one.
func stableWindow() telemetry.StabilityWindow {
	return telemetry.StabilityWindow{MinimumSamples: 2, MaximumSpread: 0.5, MinimumPeriod: 0}
}

func newTestRampController(t *testing.T, repository *telemetry.Repository, now time.Time) (*RampController, *feed.Actuator, *store.Checkpoints) {
	t.Helper()
	actuator := feed.NewActuator()
	snapshots := store.NewSnapshotStore(filepath.Join(t.TempDir(), "snapshots"))
	checkpoints := store.NewCheckpoints(snapshots)
	ramps := NewRampController(actuator, repository, stableWindow(), checkpoints, func() time.Time { return now })
	return ramps, actuator, checkpoints
}

// currentSample seeds the telemetry repository with a current reading for the
// given repeater at the supplied sequence and value.
func currentSample(t *testing.T, repository *telemetry.Repository, repeaterID string, sequence uint64, value float64, at time.Time) {
	t.Helper()
	if err := repository.Add(telemetry.Sample{
		ID: "sample", AssetID: repeaterID, Kind: telemetry.KindCurrent, Value: value, Unit: "A",
		Sequence: sequence, ObservedAt: at.UTC(),
	}); err != nil {
		t.Fatalf("seed current sample: %v", err)
	}
}

func TestApplyStepCommitsCheckpointOnlyAfterCurrentStable(t *testing.T) {
	now := time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)
	repository := telemetry.NewRepository()
	ramps, _, checkpoints := newTestRampController(t, repository, now)
	repeaterR07, err := New("R07", now)
	if err != nil {
		t.Fatal(err)
	}

	// No current samples yet: stability cannot be confirmed, so the step must be
	// rejected and, critically, no checkpoint may be persisted for it.
	snapshot, applyErr := ramps.ApplyStep(context.Background(), repeaterR07, RampStep{Number: 2, Voltage: 48}, 0)
	if applyErr == nil {
		t.Fatalf("expected unstable step to fail, got snapshot=%+v", snapshot)
	}
	if got := stableStepFromCheckpoint(t, checkpoints, "R07"); got != nil {
		t.Fatalf("checkpoint persisted for unstable step: %+v", got)
	}

	// Two settled samples make the window stable; the checkpoint is now earned.
	currentSample(t, repository, "R07", 1, 12.0, now.Add(time.Second))
	currentSample(t, repository, "R07", 2, 12.1, now.Add(2*time.Second))
	if _, err := ramps.ApplyStep(context.Background(), repeaterR07, RampStep{Number: 2, Voltage: 48}, 0); err != nil {
		t.Fatalf("expected stable step to succeed: %v", err)
	}
	got := stableStepFromCheckpoint(t, checkpoints, "R07")
	if got == nil || got.StableStep != 2 {
		t.Fatalf("stable step not checkpointed: %+v", got)
	}
}

// TestRecoverDoesNotResumeFromUnstableStep reproduces the RAMP-OVERCURRENT
// restart-recovery defect: when the service is restarted mid-ramp, recovery
// must only resume from a step whose current genuinely settled. A checkpoint
// written before stability was confirmed would let recovery jump past a
// settling step and drive the next command over the protection threshold.
func TestRecoverDoesNotResumeFromUnstableStep(t *testing.T) {
	now := time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)
	repository := telemetry.NewRepository()
	ramps, _, checkpoints := newTestRampController(t, repository, now)
	repeaterR07, err := New("R07", now)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the second ramp step being commanded but the service restarting
	// before its current ever stabilized. The fix means ApplyStep must not have
	// persisted a checkpoint for step 2; recovery therefore has no stable origin
	// and must keep StableStep at 0 rather than resuming from step 2.
	currentSample(t, repository, "R07", 1, 12.0, now.Add(time.Second))
	if _, err := ramps.ApplyStep(context.Background(), repeaterR07, RampStep{Number: 2, Voltage: 48}, 0); err == nil {
		t.Fatal("expected unstable step 2 to fail before recovery")
	}

	// The unstable step must leave no checkpoint behind, so a restart cannot
	// adopt it as a recovery origin.
	if got := stableStepFromCheckpoint(t, checkpoints, "R07"); got != nil {
		t.Fatalf("checkpoint persisted for unstable step 2: %+v", got)
	}

	// Fresh repeater process simulating a restart: only a confirmed-stable step
	// is an acceptable recovery origin.
	restarted, err := New("R07", now)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := ramps.Recover(restarted)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovered.StableStep != 0 {
		t.Fatalf("recover resumed from unstable step StableStep=%d, want 0", recovered.StableStep)
	}
}

func TestRecoverResumesFromConfirmedStableStep(t *testing.T) {
	now := time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC)
	repository := telemetry.NewRepository()
	ramps, _, _ := newTestRampController(t, repository, now)
	repeaterR07, err := New("R07", now)
	if err != nil {
		t.Fatal(err)
	}

	// Step 1 settles first; its checkpoint is committed only after stability.
	currentSample(t, repository, "R07", 1, 6.0, now.Add(time.Second))
	currentSample(t, repository, "R07", 2, 6.1, now.Add(2*time.Second))
	if _, err := ramps.ApplyStep(context.Background(), repeaterR07, RampStep{Number: 1, Voltage: 24}, 0); err != nil {
		t.Fatalf("stable step 1: %v", err)
	}

	// A restart should resume from the confirmed-stable step 1, not jump ahead.
	restarted, err := New("R07", now)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := ramps.Recover(restarted)
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovered.StableStep != 1 {
		t.Fatalf("recover StableStep=%d, want 1", recovered.StableStep)
	}
}

func stableStepFromCheckpoint(t *testing.T, checkpoints *store.Checkpoints, repeaterID string) *store.RampCheckpoint {
	t.Helper()
	cp, found, err := checkpoints.LoadRamp(repeaterID)
	if err != nil {
		t.Fatalf("load ramp checkpoint: %v", err)
	}
	if !found {
		return nil
	}
	return &cp
}
