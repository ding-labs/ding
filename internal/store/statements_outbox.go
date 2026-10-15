package store

var qAppendEvent = statement{sqlite: `INSERT INTO events(id,watch_id,revision,at,type,body) VALUES(?,?,?,?,?,?)`, postgres: `INSERT INTO events(id,watch_id,revision,at,type,body) VALUES($1,$2,$3,$4,$5,$6) RETURNING sequence`}

var qEvidence = statement{sqlite: `INSERT INTO event_evidence(event_id,observation_sequence) VALUES(?,?)`, postgres: `INSERT INTO event_evidence(event_id,observation_sequence) VALUES($1,$2)`}

var qEnqueue = statement{sqlite: `INSERT INTO outbox(event_id,watch_id,destination_id,destination_revision,payload,status,next_at,created_at) VALUES(?,?,?,?,?,'pending',?,?)`, postgres: `INSERT INTO outbox(event_id,watch_id,destination_id,destination_revision,payload,status,next_at,created_at) VALUES($1,$2,$3,$4,$5,'pending',$6,$7) RETURNING id`}

var qIntent = statement{sqlite: `SELECT id,event_id,watch_id,destination_id,destination_revision,payload,status,attempts,next_at,created_at,lease_token,retry_started_at FROM outbox WHERE id=?`, postgres: `SELECT id,event_id,watch_id,destination_id,destination_revision,payload,status,attempts,next_at,created_at,lease_token,retry_started_at FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND id=$1`}

var qClaim = statement{sqlite: claimQuery, postgres: `SELECT o.id FROM delivery_streams s CROSS JOIN LATERAL
 (SELECT * FROM outbox p WHERE p.workspace_id=s.workspace_id AND p.watch_id=s.watch_id AND p.destination_id=s.destination_id AND p.status IN ('pending','leased') ORDER BY p.id LIMIT 1) o
 WHERE s.workspace_id=current_setting('ding.workspace') AND s.active
 AND ((o.status='pending' AND o.next_at<=$1) OR (o.status='leased' AND o.lease_until<=$2))
 AND NOT EXISTS (SELECT 1 FROM metadata m WHERE m.workspace_id=o.workspace_id AND m.key='delivery_backoff:'||o.destination_id||':'||o.destination_revision AND CASE WHEN m.key='delivery_backoff:'||o.destination_id||':'||o.destination_revision THEN CAST(m.value AS BIGINT) ELSE 0 END>$3)
 AND NOT EXISTS (SELECT 1 FROM outbox p WHERE p.workspace_id=o.workspace_id AND p.watch_id=o.watch_id AND p.destination_id=o.destination_id AND p.destination_revision=o.destination_revision AND p.status='leased' AND p.lease_until>$4)
 ORDER BY s.last_turn,s.watch_id,s.destination_id LIMIT 1`}

var qDeliveryTurn = statement{sqlite: `UPDATE delivery_streams SET last_turn=(SELECT coalesce(max(last_turn),0)+1 FROM delivery_streams) WHERE watch_id=? AND destination_id=?`, postgres: `UPDATE delivery_streams SET last_turn=(SELECT coalesce(max(last_turn),0)+1 FROM delivery_streams WHERE workspace_id=current_setting('ding.workspace')) WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND destination_id=$2`}

var qLeaseIntent = statement{sqlite: `UPDATE outbox SET status='leased',lease_token=?,lease_until=?,attempts=attempts+1 WHERE id=?`, postgres: `UPDATE outbox SET status='leased',lease_token=$1,lease_until=$2,attempts=attempts+1 WHERE workspace_id=current_setting('ding.workspace') AND id=$3`}

var qFinishIntent = statement{sqlite: `UPDATE outbox SET status=?,next_at=?,lease_token='',lease_until=0,last_error=? WHERE id=? AND status='leased' AND lease_token=?`, postgres: `UPDATE outbox SET status=$1,next_at=$2,lease_token='',lease_until=0,last_error=$3 WHERE workspace_id=current_setting('ding.workspace') AND id=$4 AND status='leased' AND lease_token=$5`}

var qDeliveryBackoff = statement{sqlite: `INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=CAST(MAX(CAST(metadata.value AS INTEGER),CAST(excluded.value AS INTEGER)) AS TEXT)`, postgres: `INSERT INTO metadata(key,value) VALUES($1,$2) ON CONFLICT(workspace_id,key) DO UPDATE SET value=CAST(GREATEST(CAST(metadata.value AS BIGINT),CAST(excluded.value AS BIGINT)) AS TEXT)`}

var qDeliveryAttempt = statement{sqlite: `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) VALUES($1,$2,$3,$4,$5)`}

var qEvents = statement{sqlite: `SELECT sequence,body FROM events WHERE sequence>? ORDER BY sequence LIMIT ?`, postgres: `SELECT sequence,body FROM events WHERE workspace_id=current_setting('ding.workspace') AND sequence>$1 ORDER BY sequence LIMIT $2`}
var qWatchEvents = statement{sqlite: `SELECT sequence,body FROM events WHERE sequence>? AND watch_id=? ORDER BY sequence LIMIT ?`, postgres: `SELECT sequence,body FROM events WHERE workspace_id=current_setting('ding.workspace') AND sequence>$1 AND watch_id=$2 ORDER BY sequence LIMIT $3`}
