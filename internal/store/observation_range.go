package store

import (
	"encoding/json"
	"fmt"
	"github.com/ding-labs/ding/internal/watch"
)

// ObservationWatermark is the highest committed allocation, including history
// already removed by retention. It belongs to this workspace on both backends.
func (t *Tx) ObservationWatermark() (int64, error) {
	var sequence int64
	err := t.sql.QueryRowContext(t.ctx, t.query(qObservationWatermark)).Scan(&sequence)
	return sequence, err
}

// ObservationRange is a bounded retained-history read. Callers checking a
// receipt must additionally verify that every sequence in its range is present.
func (t *Tx) ObservationRange(id string, first, last int64) ([]watch.Observation, error) {
	if first < 1 || last < first || last-first >= 1000 {
		return nil, fmt.Errorf("observation range must contain 1–1000 sequences")
	}
	rows, err := t.sql.QueryContext(t.ctx, t.query(qObservationRange), id, first, last)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []watch.Observation{}
	bytes := 0
	for rows.Next() {
		var sequence int64
		var raw []byte
		if err = rows.Scan(&sequence, &raw); err != nil {
			return nil, err
		}
		bytes += len(raw)
		if bytes > 8<<20 {
			return nil, fmt.Errorf("observation range exceeds 8 MiB read budget")
		}
		var o watch.Observation
		if err = json.Unmarshal(raw, &o); err != nil {
			return nil, err
		}
		o.Sequence = sequence
		out = append(out, o)
	}
	return out, rows.Err()
}

var qObservationWatermark = statement{sqlite: `SELECT coalesce((SELECT seq FROM sqlite_sequence WHERE name='observations'),0)`, postgres: `SELECT observation_sequence FROM workspaces WHERE workspace_id=current_setting('ding.workspace')`}
var qObservationRange = statement{sqlite: `SELECT sequence,body FROM observations WHERE watch_id=? AND sequence>=? AND sequence<=? ORDER BY sequence`, postgres: `SELECT sequence,body FROM observations WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND sequence>=$2 AND sequence<=$3 ORDER BY sequence`}
