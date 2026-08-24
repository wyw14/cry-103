package repair

import (
	"testing"
	"time"

	"github.com/wyw14/cry-103/internal/feed"
	"github.com/wyw14/cry-103/internal/landing"
)

// stubIDGen yields deterministic, unique operation IDs so that two circuits
// going through isolation at the same station receive distinct operations.
type stubIDGen struct {
	counter int
}

func (g *stubIDGen) New(prefix string) string {
	g.counter++
	return prefix + "-" + itoa(g.counter)
}

// itoa is a tiny strconv-free int formatter (keeps the test dependency-free).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func fixedNow() time.Time { return time.Date(2026, 8, 24, 3, 0, 0, 0, time.UTC) }

// TestEvaluateRejectsCrossCircuitReceipt reproduces the reported incident:
// the east landing station hosts both the A and B feed circuits. A's
// isolation operation just finished and recorded its receipt. A permit
// requested for B must NOT become groundable against A's receipt — B is still
// energized, so confirming against the wrong receipt would let a live circuit
// be grounded.
func TestEvaluateRejectsCrossCircuitReceipt(t *testing.T) {
	now := fixedNow
	ids := &stubIDGen{}
	receipts := landing.NewReceiptRegistry()
	permits := NewPermitService(feed.NewIsolationService(), receipts, ids, now)

	circuitA, err := feed.NewCircuit("A", "east", now())
	if err != nil {
		t.Fatalf("new circuit A: %v", err)
	}
	circuitB, err := feed.NewCircuit("B", "east", now())
	if err != nil {
		t.Fatalf("new circuit B: %v", err)
	}
	permits.RegisterCircuit(circuitA)
	permits.RegisterCircuit(circuitB)

	// A's isolation completes first: it is fully isolated and a receipt is
	// recorded against A's operation.
	permitA, err := permits.Request("east", "A")
	if err != nil {
		t.Fatalf("request A: %v", err)
	}
	receipts.Record(landing.IsolationReceipt{
		StationID: "east", CircuitID: "A", OperationID: permitA.OperationID,
		Isolated: true, ReceivedAt: now(),
	})
	if _, err := permits.Evaluate(permitA.ID); err != nil {
		t.Fatalf("evaluate A: %v", err)
	}

	// Now B requests isolation. The field is still energized — no receipt
	// exists for B's operation. Evaluate must refuse to confirm B.
	permitB, err := permits.Request("east", "B")
	if err != nil {
		t.Fatalf("request B: %v", err)
	}
	if _, err := permits.Evaluate(permitB.ID); err == nil {
		t.Fatalf("B permit must not become groundable against A's receipt")
	}

	// Sanity: B's circuit is still energized/isolating, not isolated.
	if snap := circuitB.Snapshot(); snap.State == feed.StateIsolated {
		t.Fatalf("B was wrongly marked isolated by A's receipt: %+v", snap)
	}

	// And grounding B must fail, because B was never confirmed by its own
	// receipt.
	if _, err := permits.Ground(permitB.ID); err == nil {
		t.Fatalf("B permit must not be groundable without its own receipt")
	}
}

// TestEvaluateRejectsStaleReceiptFromPriorOperation ensures a receipt from a
// PRIOR operation on the SAME circuit cannot confirm a NEW operation. This is
// the "past operation" half of the mis-confirmation: even within one circuit,
// only the current operation's receipt counts.
func TestEvaluateRejectsStaleReceiptFromPriorOperation(t *testing.T) {
	now := fixedNow
	ids := &stubIDGen{}
	receipts := landing.NewReceiptRegistry()
	permits := NewPermitService(feed.NewIsolationService(), receipts, ids, now)

	circuit, err := feed.NewCircuit("B", "east", now())
	if err != nil {
		t.Fatalf("new circuit: %v", err)
	}
	permits.RegisterCircuit(circuit)

	// First operation completes and is grounded + released, leaving behind a
	// recorded receipt for operation #1.
	permit1, err := permits.Request("east", "B")
	if err != nil {
		t.Fatalf("request op1: %v", err)
	}
	receipts.Record(landing.IsolationReceipt{
		StationID: "east", CircuitID: "B", OperationID: permit1.OperationID,
		Isolated: true, ReceivedAt: now(),
	})
	if _, err := permits.Evaluate(permit1.ID); err != nil {
		t.Fatalf("evaluate op1: %v", err)
	}
	if _, err := permits.Ground(permit1.ID); err != nil {
		t.Fatalf("ground op1: %v", err)
	}
	if _, err := feed.NewIsolationService().Release(circuit, permit1.OperationID, now()); err != nil {
		t.Fatalf("release op1: %v", err)
	}

	// A second isolation operation begins. The stale receipt from op1 must
	// NOT confirm op2.
	permit2, err := permits.Request("east", "B")
	if err != nil {
		t.Fatalf("request op2: %v", err)
	}
	if permit2.OperationID == permit1.OperationID {
		t.Fatalf("op2 must have a distinct operation ID")
	}
	if _, err := permits.Evaluate(permit2.ID); err == nil {
		t.Fatalf("op2 permit must not be confirmed by op1's stale receipt")
	}
	if snap := circuit.Snapshot(); snap.State == feed.StateIsolated {
		t.Fatalf("circuit was wrongly marked isolated by a stale receipt: %+v", snap)
	}

	// Confirming op2 against its OWN receipt succeeds.
	receipts.Record(landing.IsolationReceipt{
		StationID: "east", CircuitID: "B", OperationID: permit2.OperationID,
		Isolated: true, ReceivedAt: now(),
	})
	if _, err := permits.Evaluate(permit2.ID); err != nil {
		t.Fatalf("evaluate op2 with its own receipt: %v", err)
	}
}
