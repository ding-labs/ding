package store

var qEventPage1 = statement{sqlite: `SELECT value FROM metadata WHERE key='store_id'`, postgres: `SELECT value FROM metadata WHERE workspace_id=current_setting('ding.workspace') AND key='store_id'`}

var qEventPage2 = statement{sqlite: `SELECT COALESCE((SELECT seq FROM sqlite_sequence WHERE name='events'),0)`, postgres: `SELECT event_sequence FROM workspaces WHERE workspace_id=current_setting('ding.workspace')`}

var qEventPage3 = statement{sqlite: `SELECT COALESCE((SELECT CAST(value AS INTEGER) FROM metadata WHERE key=?),0)`, postgres: `SELECT COALESCE((SELECT CAST(value AS BIGINT) FROM metadata WHERE workspace_id=current_setting('ding.workspace') AND key=$1),0)`}

var qEvent1 = statement{sqlite: `SELECT sequence,body FROM events WHERE id=?`, postgres: `SELECT sequence,body FROM events WHERE workspace_id=current_setting('ding.workspace') AND id=$1`}

var qDefinition1 = statement{sqlite: `SELECT definition FROM watch_revisions WHERE watch_id=? AND revision=?`, postgres: `SELECT definition FROM watch_revisions WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND revision=$2`}

var qSaveReplay1 = statement{sqlite: `INSERT INTO event_replays(event_id,body) VALUES(?,?)`, postgres: `INSERT INTO event_replays(event_id,body) VALUES($1,$2)`}

var qReplay1 = statement{sqlite: `SELECT body FROM event_replays WHERE event_id=?`, postgres: `SELECT body FROM event_replays WHERE workspace_id=current_setting('ding.workspace') AND event_id=$1`}

var qDelivery1 = statement{sqlite: `SELECT id,attempt,at,outcome,detail FROM delivery_attempts WHERE outbox_id=? AND id<? ORDER BY id DESC LIMIT 100`, postgres: `SELECT id,attempt,at,outcome,detail FROM delivery_attempts WHERE workspace_id=current_setting('ding.workspace') AND outbox_id=$1 AND id<$2 ORDER BY id DESC LIMIT 100`}

var qRetry1 = statement{sqlite: `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) VALUES(?,?,?,'manual_retry','new delivery policy cycle')`, postgres: `INSERT INTO delivery_attempts(outbox_id,attempt,at,outcome,detail) VALUES($1,$2,$3,'manual_retry','new delivery policy cycle')`}

var qRetry2 = statement{sqlite: `UPDATE outbox SET status='pending',attempts=0,next_at=?,retry_started_at=?,last_error='' WHERE id=?`, postgres: `UPDATE outbox SET status='pending',attempts=0,next_at=$1,retry_started_at=$2,last_error='' WHERE workspace_id=current_setting('ding.workspace') AND id=$3`}

var qDeliveryCounts1 = statement{sqlite: `SELECT status,count(*) FROM outbox GROUP BY status`, postgres: `SELECT status,count(*) FROM outbox WHERE workspace_id=current_setting('ding.workspace') GROUP BY status`}

var qDestinations1 = statement{sqlite: `SELECT DISTINCT r.definition FROM destination_revisions r WHERE EXISTS(SELECT 1 FROM destinations d WHERE d.id=r.destination_id AND d.revision=r.revision) OR EXISTS(SELECT 1 FROM outbox o WHERE o.destination_id=r.destination_id AND o.destination_revision=r.revision AND o.status IN ('pending','leased'))`, postgres: `SELECT DISTINCT r.definition FROM destination_revisions r WHERE r.workspace_id=current_setting('ding.workspace') AND EXISTS(SELECT 1 FROM destinations d WHERE d.workspace_id=r.workspace_id AND d.id=r.destination_id AND d.revision=r.revision) OR EXISTS(SELECT 1 FROM outbox o WHERE o.workspace_id=r.workspace_id AND o.destination_id=r.destination_id AND o.destination_revision=r.revision AND o.status IN ('pending','leased'))`}

var qEvidenceInputs1 = statement{sqlite: `SELECT o.sequence,o.body FROM observations o JOIN event_evidence e ON e.observation_sequence=o.sequence WHERE e.event_id=? AND o.sequence>? ORDER BY o.sequence LIMIT 100`, postgres: `SELECT o.sequence,o.body FROM observations o JOIN event_evidence e ON e.workspace_id=o.workspace_id AND e.observation_sequence=o.sequence WHERE o.workspace_id=current_setting('ding.workspace') AND e.event_id=$1 AND o.sequence>$2 ORDER BY o.sequence LIMIT 100`}

var qInspectSummary1 = statement{sqlite: `SELECT sequence,body FROM observations WHERE watch_id=? AND generation=? ORDER BY sequence DESC LIMIT 1`, postgres: `SELECT sequence,body FROM observations WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND generation=$2 ORDER BY sequence DESC LIMIT 1`}

var qInspectSummary2 = statement{sqlite: `SELECT entity_key,COALESCE(json_extract(state,'$.open'),0),COALESCE(json_extract(state,'$.sourceUnhealthy'),0),COALESCE(json_extract(state,'$.matches'),0),COALESCE(json_extract(state,'$.recoveries'),0),COALESCE(json_extract(state,'$.lastSequence'),0),COALESCE(json_array_length(state,'$.samples'),0),COALESCE(json_extract(state,'$.waitingReason'),'') FROM entities WHERE watch_id=? AND entity_key>? ORDER BY entity_key LIMIT 101`, postgres: `SELECT entity_key,
coalesce((convert_from(state,'UTF8')::jsonb->>'open')::boolean,false),
coalesce((convert_from(state,'UTF8')::jsonb->>'sourceUnhealthy')::boolean,false),
coalesce((convert_from(state,'UTF8')::jsonb->>'matches')::bigint,0),
coalesce((convert_from(state,'UTF8')::jsonb->>'recoveries')::bigint,0),
coalesce((convert_from(state,'UTF8')::jsonb->>'lastSequence')::bigint,0),
CASE WHEN jsonb_typeof(convert_from(state,'UTF8')::jsonb->'samples')='array' THEN jsonb_array_length(convert_from(state,'UTF8')::jsonb->'samples') ELSE 0 END,
coalesce(convert_from(state,'UTF8')::jsonb->>'waitingReason','')
FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND entity_key>$2 ORDER BY entity_key LIMIT 101`}

var qInspectSummary3 = statement{sqlite: `SELECT id,event_id,destination_id,destination_revision,status,attempts,next_at,last_error FROM outbox WHERE watch_id=? AND id<? ORDER BY id DESC LIMIT 101`, postgres: `SELECT id,event_id,destination_id,destination_revision,status,attempts,next_at,last_error FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND id<$2 ORDER BY id DESC LIMIT 101`}

var qEntityHealth1 = statement{sqlite: `SELECT COALESCE(SUM(CASE WHEN json_extract(state,'$.sourceUnhealthy') THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN json_extract(state,'$.open') THEN 1 ELSE 0 END),0) FROM entities WHERE watch_id=?`, postgres: `SELECT coalesce(sum(CASE WHEN (convert_from(state,'UTF8')::jsonb->>'sourceUnhealthy')::boolean THEN 1 ELSE 0 END),0),coalesce(sum(CASE WHEN (convert_from(state,'UTF8')::jsonb->>'open')::boolean THEN 1 ELSE 0 END),0) FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`}
