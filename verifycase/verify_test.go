package verifycase

import (
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/feed"
	"github.com/wyw14/cry-103/internal/landing"
	"github.com/wyw14/cry-103/internal/repair"
)

type fixedIDs struct{ next int }

func (g *fixedIDs) New(prefix string) string {
	g.next++
	return prefix + "-fixed"
}

func TestFeedPermitUsesCurrentCircuitReceipt(t *testing.T) {
	now := time.Now
	receipts := landing.NewReceiptRegistry()
	service := repair.NewPermitService(feed.NewIsolationService(), receipts, &fixedIDs{}, now)
	circuit, err := feed.NewCircuit("B", "east", now())
	if err != nil {
		t.Fatal(err)
	}
	service.RegisterCircuit(circuit)
	permit, err := service.Request("east", "B")
	if err != nil {
		t.Fatal(err)
	}
	receipts.Record(landing.IsolationReceipt{
		StationID: "east", CircuitID: "A", OperationID: "isolation-old",
		Isolated: true, ReceivedAt: now(),
	})
	if _, err := service.Evaluate(permit.ID); err == nil {
		t.Fatal("receipt from circuit A satisfied the active circuit B isolation")
	}
	current, _ := service.Get(permit.ID)
	if current.State != repair.PermitIsolating {
		t.Fatalf("permit advanced without a scoped receipt: %+v", current)
	}
}
