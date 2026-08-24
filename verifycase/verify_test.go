package verifycase

import (
	"context"
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/platform"
	"github.com/wyw14/cry-103/internal/repair"
	"github.com/wyw14/cry-103/internal/telemetry"
)

func TestCanceledWetPlantTestReleasesAlarmMute(t *testing.T) {
	mutes := telemetry.NewMuteManager(time.Now)
	service := repair.NewTestService(mutes, platform.UUIDGenerator{}, time.Now, 30*time.Millisecond)
	session := service.Start(context.Background(), "S3")
	if !mutes.Muted("S3") {
		t.Fatal("wet-plant test did not acquire its alarm mute")
	}
	result := service.Cancel(session)
	if result.State != repair.TestCanceled {
		t.Fatalf("test session did not cancel: %+v", result)
	}
	time.Sleep(70 * time.Millisecond)
	if mutes.Muted("S3") {
		t.Fatal("canceled wet-plant test continued renewing its alarm mute")
	}
}
