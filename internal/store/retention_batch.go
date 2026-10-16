package store

import (
	"fmt"
	"time"
)

// RetainBatch checks current persistent references inside every write batch.
// No pins computed in an earlier snapshot are used for deletion. The row bound
// applies to each history family; dependent evidence/attempts cascade with it.
// The result counts examined candidates plus direct deletions as progress.
// Use the same nonempty pass identity until progress is zero.
func (t *Tx) RetainBatch(cutoff, now time.Time, limit int, pass string) (int64, error) {
	if limit < 1 || limit > 256 || pass == "" || len(pass) > 128 {
		return 0, fmt.Errorf("invalid retention batch limit")
	}
	var changed int64
	for _, item := range []struct {
		query statement
		at    time.Time
	}{
		{qRetainBatchInputReceipts, now}, {qRetainBatchPushReceipts, now}, {qRetainBatchBackoff, now},
		{qRetainBatchOutbox, cutoff},
	} {
		result, err := t.exec(item.query, timestamp(item.at), limit)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		changed += n
	}
	for _, family := range retentionFamilies {
		n, err := t.scanRetentionFamily(family, pass, timestamp(cutoff), limit)
		if err != nil {
			return 0, err
		}
		changed += n
	}
	result, err := t.exec(qRetainBatchStreams, limit)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	return changed + n, err
}

var qRetainBatchStreams = statement{sqlite: `DELETE FROM delivery_streams WHERE (watch_id,destination_id) IN (SELECT watch_id,destination_id FROM delivery_streams s WHERE NOT EXISTS(SELECT 1 FROM outbox o WHERE o.watch_id=s.watch_id AND o.destination_id=s.destination_id) LIMIT ?)`, postgres: `DELETE FROM delivery_streams WHERE workspace_id=current_setting('ding.workspace') AND (watch_id,destination_id) IN (SELECT watch_id,destination_id FROM delivery_streams s WHERE s.workspace_id=current_setting('ding.workspace') AND NOT EXISTS(SELECT 1 FROM outbox o WHERE o.workspace_id=s.workspace_id AND o.watch_id=s.watch_id AND o.destination_id=s.destination_id) LIMIT $1)`}

// RetainLogSummariesBatch scans a bounded candidate page before checking
// source type or live references, using the same durable sweep protocol.
func (t *Tx) RetainLogSummariesBatch(id string, cutoff time.Time, limit int, pass string) (int64, error) {
	if id == "" || limit < 1 || limit > 256 || pass == "" || len(pass) > 128 {
		return 0, fmt.Errorf("invalid log retention batch")
	}
	family := retentionFamily{name: "logs:" + id, numeric: true, prefixArgs: []any{id}, selectQuery: qSelectLogSweep, deletePrefix: qDeleteLogSweep}
	return t.scanRetentionFamily(family, pass, timestamp(cutoff), limit)
}

var qSelectLogSweep = statement{sqlite: `SELECT accepted_at,CAST(sequence AS TEXT),'' FROM observations WHERE watch_id=? AND accepted_at<? AND (accepted_at,sequence)>(?,?) ORDER BY observations.accepted_at,observations.sequence LIMIT ?`, postgres: `SELECT accepted_at,CAST(sequence AS TEXT),'' FROM observations WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND accepted_at<$2 AND (accepted_at,sequence)>($3,$4) ORDER BY observations.accepted_at,observations.sequence LIMIT $5`}
var qDeleteLogSweep = statement{sqlite: `DELETE FROM observations WHERE revision IN(SELECT revision FROM watch_revisions WHERE watch_id=observations.watch_id AND json_extract(definition,'$.spec.source.type')='logs') AND NOT EXISTS(SELECT 1 FROM event_evidence WHERE observation_sequence=observations.sequence) AND NOT EXISTS(SELECT 1 FROM entity_pins WHERE kind='input' AND value=CAST(observations.sequence AS TEXT)) AND sequence IN (`, postgres: `DELETE FROM observations WHERE workspace_id=current_setting('ding.workspace') AND revision IN(SELECT revision FROM watch_revisions WHERE workspace_id=current_setting('ding.workspace') AND watch_id=observations.watch_id AND (convert_from(definition,'UTF8')::jsonb #>> '{spec,source,type}')='logs') AND NOT EXISTS(SELECT 1 FROM event_evidence WHERE workspace_id=current_setting('ding.workspace') AND observation_sequence=observations.sequence) AND NOT EXISTS(SELECT 1 FROM entity_pins WHERE workspace_id=current_setting('ding.workspace') AND kind='input' AND value=CAST(observations.sequence AS TEXT)) AND sequence IN (`}
