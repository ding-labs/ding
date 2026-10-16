package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ding-labs/ding/internal/watch"
)

// AppendObservations reserves exactly the appended range in the workspace's
// transaction. SQLite keeps its inexpensive embedded inserts; PostgreSQL uses
// one statement for the range and rows. Evaluation order remains with the caller.
func (t *Tx) AppendObservations(id, revision string, generation int64, observations []watch.Observation) ([]int64, error) {
	if len(observations) == 0 {
		return nil, nil
	}
	if !t.postgres || len(observations) == 1 {
		sequences := make([]int64, len(observations))
		for i, o := range observations {
			var err error
			sequences[i], err = t.AppendObservation(id, revision, generation, o)
			if err != nil {
				return nil, err
			}
		}
		return sequences, nil
	}
	// Watch plans allow 100 outputs plus one internally generated gap.
	if len(observations) > 101 {
		return nil, fmt.Errorf("observation batch exceeds bound")
	}
	args := []any{id, revision, generation, len(observations)}
	values := make([]string, len(observations))
	for i, o := range observations {
		body, err := json.Marshal(o)
		if err != nil {
			return nil, err
		}
		n := len(args) + 1
		values[i] = fmt.Sprintf("(%d,$%d::text,$%d::bigint,$%d::bytea)", i+1, n, n+1, n+2)
		args = append(args, o.InputID, timestamp(o.AcceptedAt), body)
	}
	query := `WITH allocated AS (
 UPDATE workspaces SET observation_sequence=observation_sequence+$4
 WHERE workspace_id=current_setting('ding.workspace') RETURNING observation_sequence)
 INSERT INTO observations(sequence,watch_id,revision,generation,input_id,accepted_at,body)
 SELECT allocated.observation_sequence-$4+v.ordinal,$1,$2,$3,v.input_id,v.accepted_at,v.body
 FROM allocated CROSS JOIN (VALUES ` + strings.Join(values, ",") + `) v(ordinal,input_id,accepted_at,body)
 ORDER BY v.ordinal RETURNING sequence`
	rows, err := t.sql.QueryContext(t.ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sequences := make([]int64, 0, len(observations))
	for rows.Next() {
		var sequence int64
		if err := rows.Scan(&sequence); err != nil {
			return nil, err
		}
		sequences = append(sequences, sequence)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(sequences) != len(observations) {
		return nil, fmt.Errorf("observation range unavailable")
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	t.changed = true
	return sequences, nil
}
