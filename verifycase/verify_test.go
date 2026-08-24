package verifycase

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/incident"
	"github.com/wyw14/cry-103/internal/repeater"
	"github.com/wyw14/cry-103/internal/telemetry"
)

func TestLateTelemetryCannotClearRepeaterTrip(t *testing.T) {
	now := time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC)
	lifecycle := incident.NewRepeaterLifecycle()
	projector := repeater.NewHealthProjector(lifecycle, 85)
	target, err := repeater.New("R07", now)
	if err != nil {
		t.Fatal(err)
	}
	projector.Register(target)
	ingestor := telemetry.NewIngestor(telemetry.NewRepository(), func() time.Time { return now }, projector)
	trip := telemetry.Sample{
		ID: "trip", AssetID: "R07", Kind: telemetry.KindTemperature, Value: 96,
		Sequence: 10, Generation: 5, ObservedAt: now,
	}
	if err := ingestor.Apply(context.Background(), trip); err != nil {
		t.Fatal(err)
	}
	late := telemetry.Sample{
		ID: "late", AssetID: "R07", Kind: telemetry.KindTemperature, Value: 42,
		Sequence: 9, Generation: 4, ObservedAt: now.Add(-time.Minute),
	}
	if err := ingestor.Apply(context.Background(), late); err != nil {
		t.Fatal(err)
	}
	if target.Snapshot().Health != repeater.HealthTripped {
		t.Fatalf("late telemetry cleared the newer trip: %+v", target.Snapshot())
	}
	if _, active := lifecycle.Active("R07"); !active {
		t.Fatal("late telemetry resolved the active overheat incident")
	}
}
