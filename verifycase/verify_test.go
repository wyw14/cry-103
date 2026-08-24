package verifycase

import (
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/incident"
)

func TestCableIncidentAcceptsPeerEvidenceWithinWindow(t *testing.T) {
	now := time.Date(2026, 8, 24, 5, 0, 0, 0, time.UTC)
	current := incident.New("INC-1", "S3", now.Add(10*time.Second), now)
	classifier := incident.NewClassifier()
	first := classifier.LocalComplete(current, true, now.Add(time.Second))
	if first.State != incident.StateWaitingPeer {
		t.Fatalf("incident did not preserve its peer evidence window: %+v", first)
	}
	final := classifier.PeerEvidence(current, now.Add(4*time.Second))
	if final.State != incident.StateCableCut || !final.PeerComplete {
		t.Fatalf("valid peer evidence did not refine the incident: %+v", final)
	}
}
