package store

var qResetEntities = statement{sqlite: `DELETE FROM entities WHERE watch_id=?`, postgres: `DELETE FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`}

var qResetTimers = statement{sqlite: `DELETE FROM timers WHERE watch_id=?`, postgres: `DELETE FROM timers WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`}

var qResetReceipts = statement{sqlite: `DELETE FROM input_receipts WHERE watch_id=?`, postgres: `DELETE FROM input_receipts WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`}

var qDeleteEntity1 = statement{sqlite: `DELETE FROM entities WHERE watch_id=? AND entity_key=?`, postgres: `DELETE FROM entities WHERE watch_id=$1 AND entity_key=$2`}

var qDeleteTimers1 = statement{sqlite: `DELETE FROM timers WHERE watch_id=?`, postgres: `DELETE FROM timers WHERE watch_id=$1`}

var qTimer1 = statement{sqlite: `SELECT generation,due_at,body FROM timers WHERE watch_id=? AND entity_key=? AND kind=?`, postgres: `SELECT generation,due_at,body FROM timers WHERE watch_id=$1 AND entity_key=$2 AND kind=$3`}

var qDeleteTimer1 = statement{sqlite: `DELETE FROM timers WHERE watch_id=? AND entity_key=? AND kind=? AND generation=? AND due_at=?`, postgres: `DELETE FROM timers WHERE watch_id=$1 AND entity_key=$2 AND kind=$3 AND generation=$4 AND due_at=$5`}

var qDueTimers1 = statement{sqlite: `SELECT t.watch_id,t.entity_key,t.kind,t.generation,t.due_at,t.body FROM timers t JOIN watches w ON w.id=t.watch_id WHERE w.status='running' AND t.generation=w.generation AND t.due_at<=? ORDER BY t.due_at,t.watch_id,t.entity_key LIMIT ?`, postgres: `SELECT t.watch_id,t.entity_key,t.kind,t.generation,t.due_at,t.body FROM timers t JOIN watches w ON w.workspace_id=t.workspace_id AND w.id=t.watch_id WHERE w.status='running' AND t.generation=w.generation AND t.due_at<=$1 ORDER BY t.due_at,t.watch_id,t.entity_key LIMIT $2`}

var qCancelDeliveries1 = statement{sqlite: `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) SELECT id,attempts,?,'canceled','explicit cancellation' FROM outbox WHERE watch_id=? AND status IN ('pending','leased')`, postgres: `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) SELECT id,attempts,$1,'canceled','explicit cancellation' FROM outbox WHERE watch_id=$2 AND status IN ('pending','leased')`}

var qCancelDeliveries2 = statement{sqlite: `UPDATE outbox SET status='canceled',lease_token='',lease_until=0,last_error='explicit cancellation' WHERE watch_id=? AND status IN ('pending','leased')`, postgres: `UPDATE outbox SET status='canceled',lease_token='',lease_until=0,last_error='explicit cancellation' WHERE watch_id=$1 AND status IN ('pending','leased')`}

var qRetain1 = statement{sqlite: `CREATE TEMP TABLE IF NOT EXISTS retained_inputs(sequence INTEGER PRIMARY KEY)`, postgres: `CREATE TEMP TABLE IF NOT EXISTS retained_inputs(sequence BIGINT PRIMARY KEY) ON COMMIT DROP`}

var qRetain2 = statement{sqlite: `DELETE FROM retained_inputs`, postgres: `DELETE FROM retained_inputs`}

var qRetain3 = statement{sqlite: `CREATE TEMP TABLE IF NOT EXISTS retained_events(id TEXT PRIMARY KEY)`, postgres: `CREATE TEMP TABLE IF NOT EXISTS retained_events(id TEXT PRIMARY KEY) ON COMMIT DROP`}

var qRetain4 = statement{sqlite: `DELETE FROM retained_events`, postgres: `DELETE FROM retained_events`}

var qRetain5 = statement{sqlite: `INSERT OR IGNORE INTO retained_events(id) VALUES(?)`, postgres: `INSERT INTO retained_events(id) VALUES($1) ON CONFLICT DO NOTHING`}

var qRetain6 = statement{sqlite: `INSERT OR IGNORE INTO retained_inputs(sequence) VALUES(?)`, postgres: `INSERT INTO retained_inputs(sequence) VALUES($1) ON CONFLICT DO NOTHING`}

var qRetain7 = statement{sqlite: `DELETE FROM input_receipts WHERE expires_at<=?`, postgres: `DELETE FROM input_receipts WHERE expires_at<=$1`}

var qRetain8 = statement{sqlite: `DELETE FROM push_receipts WHERE expires_at<=?`, postgres: `DELETE FROM push_receipts WHERE expires_at<=$1`}

var qRetain9 = statement{sqlite: `DELETE FROM metadata WHERE key LIKE 'delivery_backoff:%' AND CAST(value AS INTEGER)<=?`, postgres: `DELETE FROM metadata WHERE key LIKE 'delivery_backoff:%' AND CAST(value AS BIGINT)<=$1`}

var qRetain10 = statement{sqlite: `DELETE FROM outbox WHERE created_at<? AND status NOT IN ('pending','leased')`, postgres: `DELETE FROM outbox WHERE created_at<$1 AND status NOT IN ('pending','leased')`}

var qRetain11 = statement{sqlite: `DELETE FROM events WHERE at<? AND NOT EXISTS (SELECT 1 FROM outbox WHERE event_id=events.id) AND NOT EXISTS (SELECT 1 FROM retained_events WHERE retained_events.id=events.id)`, postgres: `DELETE FROM events WHERE at<$1 AND NOT EXISTS (SELECT 1 FROM outbox WHERE event_id=events.id) AND NOT EXISTS (SELECT 1 FROM retained_events WHERE retained_events.id=events.id)`}

var qRetain12 = statement{sqlite: `DELETE FROM observations WHERE accepted_at<? AND NOT EXISTS (SELECT 1 FROM event_evidence WHERE observation_sequence=observations.sequence) AND NOT EXISTS (SELECT 1 FROM retained_inputs WHERE sequence=observations.sequence)`, postgres: `DELETE FROM observations WHERE accepted_at<$1 AND NOT EXISTS (SELECT 1 FROM event_evidence WHERE observation_sequence=observations.sequence) AND NOT EXISTS (SELECT 1 FROM retained_inputs WHERE sequence=observations.sequence)`}

var qRetain13 = statement{sqlite: `DELETE FROM watches WHERE status='deleted' AND next_at<? AND NOT EXISTS (SELECT 1 FROM entities WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM observations WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM events WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM outbox WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM timers WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM input_receipts WHERE watch_id=watches.id)`, postgres: `DELETE FROM watches WHERE status='deleted' AND next_at<$1 AND NOT EXISTS (SELECT 1 FROM entities WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM observations WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM events WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM outbox WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM timers WHERE watch_id=watches.id) AND NOT EXISTS (SELECT 1 FROM input_receipts WHERE watch_id=watches.id)`}

var qRetain14 = statement{sqlite: `DELETE FROM watch_revisions WHERE created_at<? AND NOT EXISTS (SELECT 1 FROM watches WHERE id=watch_id AND watches.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM entities WHERE entities.watch_id=watch_revisions.watch_id AND entities.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM observations WHERE observations.watch_id=watch_revisions.watch_id AND observations.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM events WHERE events.watch_id=watch_revisions.watch_id AND events.revision=watch_revisions.revision)`, postgres: `DELETE FROM watch_revisions WHERE created_at<$1 AND NOT EXISTS (SELECT 1 FROM watches WHERE id=watch_id AND watches.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM entities WHERE entities.watch_id=watch_revisions.watch_id AND entities.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM observations WHERE observations.watch_id=watch_revisions.watch_id AND observations.revision=watch_revisions.revision) AND NOT EXISTS (SELECT 1 FROM events WHERE events.watch_id=watch_revisions.watch_id AND events.revision=watch_revisions.revision)`}

var qRetain15 = statement{sqlite: `DELETE FROM destination_revisions WHERE created_at<? AND NOT EXISTS (SELECT 1 FROM destinations WHERE destinations.id=destination_id AND destinations.revision=destination_revisions.revision) AND NOT EXISTS (SELECT 1 FROM outbox WHERE outbox.destination_id=destination_revisions.destination_id AND outbox.destination_revision=destination_revisions.revision)`, postgres: `DELETE FROM destination_revisions WHERE created_at<$1 AND NOT EXISTS (SELECT 1 FROM destinations WHERE destinations.id=destination_id AND destinations.revision=destination_revisions.revision) AND NOT EXISTS (SELECT 1 FROM outbox WHERE outbox.destination_id=destination_revisions.destination_id AND outbox.destination_revision=destination_revisions.revision)`}

var qRetainLogSummaries1 = statement{sqlite: `DELETE FROM observations WHERE watch_id=? AND accepted_at<?
 AND revision IN (SELECT revision FROM watch_revisions WHERE watch_id=? AND json_extract(definition,'$.spec.source.type')='logs')
 AND NOT EXISTS (SELECT 1 FROM event_evidence WHERE observation_sequence=observations.sequence)
 AND NOT EXISTS (SELECT 1 FROM retained_inputs WHERE sequence=observations.sequence)`, postgres: `DELETE FROM observations WHERE watch_id=$1 AND accepted_at<$2
 AND revision IN (SELECT revision FROM watch_revisions WHERE watch_id=$3 AND (convert_from(definition,'UTF8')::jsonb #>> '{spec,source,type}')='logs')
 AND NOT EXISTS (SELECT 1 FROM event_evidence WHERE observation_sequence=observations.sequence)
 AND NOT EXISTS (SELECT 1 FROM retained_inputs WHERE sequence=observations.sequence)`}

var qRevisionState1 = statement{sqlite: `UPDATE entities SET revision=? WHERE watch_id=?`, postgres: `UPDATE entities SET revision=$1 WHERE watch_id=$2`}

var qRevisionState2 = statement{sqlite: `UPDATE timers SET generation=? WHERE watch_id=?`, postgres: `UPDATE timers SET generation=$1 WHERE watch_id=$2`}

var qObservations1 = statement{sqlite: `SELECT body FROM observations WHERE watch_id=? AND sequence=?`, postgres: `SELECT body FROM observations WHERE watch_id=$1 AND sequence=$2`}
