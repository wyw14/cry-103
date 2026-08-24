package otdr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wyw14/cry-103/internal/landing"
	"github.com/wyw14/cry-103/internal/platform"
	"github.com/wyw14/cry-103/internal/topology"
)

// maxScanAttempts bounds how many times Run will re-pin and retry a scan that a
// concurrent protection switchover keeps interrupting. Each attempt reacquires a
// fresh topology snapshot so a scan that starts on the primary path and ends on
// the protection path is retried against a consistent path instead of being
// published as a single trajectory that spans both.
const maxScanAttempts = 3

type SessionService struct {
	mu       sync.RWMutex
	topology *topology.Registry
	scanner  *landing.Scanner
	ids      platform.IDGenerator
	now      func() time.Time
	traces   map[string]Trace
}

func NewSessionService(registry *topology.Registry, scanner *landing.Scanner, ids platform.IDGenerator, now func() time.Time) *SessionService {
	return &SessionService{
		topology: registry, scanner: scanner, ids: ids, now: now, traces: make(map[string]Trace),
	}
}

func (s *SessionService) Run(ctx context.Context, spanID, stationID string, lengthKM float64, chunks int) (Trace, error) {
	// A protection switchover can complete in the middle of a scan. To guarantee
	// every published trace corresponds to exactly one physical path, Run reacquires
	// the topology snapshot at the start of each attempt and aborts the attempt the
	// moment the live generation moves. It only publishes a trace whose chunks all
	// share one generation and one path, retrying until a switchover stops racing.
	var lastErr error
	for attempt := 0; attempt < maxScanAttempts; attempt++ {
		trace, retry, err := s.attemptScan(ctx, spanID, stationID, lengthKM, chunks)
		if err != nil {
			return Trace{}, err
		}
		if retry {
			lastErr = err
			continue
		}
		s.mu.Lock()
		s.traces[trace.ID] = trace
		s.mu.Unlock()
		return trace, nil
	}
	if lastErr == nil {
		lastErr = errors.New("otdr scan could not pin a consistent topology snapshot")
	}
	return Trace{}, fmt.Errorf("collect OTDR chunks: %w", lastErr)
}

// attemptScan performs a single pinned scan. When the topology generation moves
// during the scan it returns retry=true so the caller can re-pin against the new
// snapshot instead of publishing a trace that straddles a switchover.
func (s *SessionService) attemptScan(ctx context.Context, spanID, stationID string, lengthKM float64, chunks int) (Trace, bool, error) {
	snapshot, err := s.topology.Snapshot(spanID)
	if err != nil {
		return Trace{}, false, err
	}
	startedAt := s.now()
	trace := Trace{
		ID: s.ids.New("scan"), SpanID: spanID, Generation: snapshot.Generation,
		Path: snapshot.ActivePath, StartedAt: startedAt.UTC(),
	}
	parts, err := s.scanner.ScanPinned(ctx, snapshot, stationID, chunks)
	if err != nil {
		if errors.Is(err, landing.ErrTopologyChanged) {
			return Trace{}, true, err
		}
		return Trace{}, false, fmt.Errorf("collect OTDR chunks: %w", err)
	}
	trace, err = assemble(trace, parts, lengthKM, s.now())
	if err != nil {
		return Trace{}, false, err
	}
	return trace, false, nil
}

func (s *SessionService) Get(id string) (Trace, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	trace, exists := s.traces[id]
	return trace, exists
}

func (s *SessionService) List() []Trace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Trace, 0, len(s.traces))
	for _, trace := range s.traces {
		result = append(result, trace)
	}
	return result
}
