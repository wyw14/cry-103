package landing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wyw14/cry-103/internal/topology"
)

// ErrTopologyChanged is returned when the span's active path is modified by a
// concurrent protection switchover while an OTDR scan is in flight. A scan that
// straddled a switchover would mix chunks from two physical paths into a single
// published trace, so the scan is aborted rather than emitting an inconsistent
// trajectory. Callers may retry against a fresh snapshot.
var ErrTopologyChanged = errors.New("otdr scan aborted: topology changed mid-scan")

type TraceChunk struct {
	StationID  string        `json:"station_id"`
	Index      int           `json:"index"`
	Path       topology.Path `json:"path"`
	Generation uint64        `json:"generation"`
	DistanceKM float64       `json:"distance_km"`
	LossDB     float64       `json:"loss_db"`
}

type Scanner struct {
	topology *topology.Registry
	delay    time.Duration
}

func NewScanner(registry *topology.Registry, delay time.Duration) *Scanner {
	return &Scanner{topology: registry, delay: delay}
}

// ScanPinned collects OTDR chunks that all belong to the same physical path
// captured by snapshot. It re-reads the live topology before each chunk only to
// detect a concurrent protection switchover; the path and generation stamped on
// every chunk are taken from the pinned snapshot so a single trace can never
// span two physical paths. If the span's generation changes mid-scan the scan
// is aborted with ErrTopologyChanged and no chunks are returned.
func (s *Scanner) ScanPinned(ctx context.Context, snapshot topology.Snapshot, stationID string, chunks int) ([]TraceChunk, error) {
	if chunks < 1 {
		return nil, fmt.Errorf("chunk count must be positive")
	}
	result := make([]TraceChunk, 0, chunks)
	for index := 0; index < chunks; index++ {
		if err := wait(ctx, s.delay); err != nil {
			return nil, err
		}
		live, err := s.topology.Snapshot(snapshot.SpanID)
		if err != nil {
			return nil, err
		}
		if live.Generation != snapshot.Generation {
			return nil, fmt.Errorf("%w: span %s moved from generation %d to %d",
				ErrTopologyChanged, snapshot.SpanID, snapshot.Generation, live.Generation)
		}
		result = append(result, synthesizeChunk(snapshot, stationID, index))
	}
	return result, nil
}

// synthesizeChunk stamps the pinned path and generation onto the chunk. The
// distance offset mirrors the physical difference between primary and
// protection routes so a switchover is observable in the trace when it happens
// between two scans rather than being silently merged into one.
func synthesizeChunk(snapshot topology.Snapshot, stationID string, index int) TraceChunk {
	distance := 18.0 + float64(index)*3.5
	if snapshot.ActivePath == topology.PathProtection {
		distance += 42
	}
	return TraceChunk{
		StationID: stationID, Index: index, Path: snapshot.ActivePath,
		Generation: snapshot.Generation, DistanceKM: distance, LossDB: 2.5 + float64(index)/2,
	}
}

func wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
