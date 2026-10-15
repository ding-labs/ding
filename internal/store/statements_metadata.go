package store

var qLastError1 = statement{sqlite: `UPDATE watches SET last_error=? WHERE id=? AND generation=?`, postgres: `UPDATE watches SET last_error=$1 WHERE workspace_id=current_setting('ding.workspace') AND id=$2 AND generation=$3`}

var qLastError2 = statement{sqlite: `UPDATE watches SET last_error=?,next_at=? WHERE id=? AND generation=?`, postgres: `UPDATE watches SET last_error=$1,next_at=$2 WHERE workspace_id=current_setting('ding.workspace') AND id=$3 AND generation=$4`}

var qPending1 = statement{sqlite: `SELECT count(*) FROM outbox WHERE status IN ('pending','leased')`, postgres: `SELECT count(*) FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND status IN ('pending','leased')`}

var qMonitor1 = statement{sqlite: `SELECT count(*),COALESCE(MIN(next_at),0) FROM watches WHERE status='running'`, postgres: `SELECT count(*),COALESCE(MIN(next_at),0) FROM watches WHERE workspace_id=current_setting('ding.workspace') AND status='running'`}

var qMonitor2 = statement{sqlite: `SELECT count(*),COALESCE(MIN(created_at),0) FROM outbox WHERE status IN ('pending','leased')`, postgres: `SELECT count(*),COALESCE(MIN(created_at),0) FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND status IN ('pending','leased')`}

var qMonitor3 = statement{sqlite: `SELECT count(*) FROM entities WHERE json_extract(state,'$.sourceUnhealthy')=1`, postgres: `SELECT count(*) FROM entities WHERE workspace_id=current_setting('ding.workspace') AND (convert_from(state,'UTF8')::jsonb->>'sourceUnhealthy')::boolean=true`}

var qauditMutation1 = statement{sqlite: `INSERT INTO audit(actor,workspace,grant_id,action,target,at) VALUES(?,?,?,?,?,?)`, postgres: `INSERT INTO audit(actor,workspace,grant_id,action,target,at) VALUES($1,$2,$3,$4,$5,$6)`}

var qauditMutation2 = statement{sqlite: `DELETE FROM audit WHERE sequence <= (SELECT COALESCE(MAX(sequence),0)-10000 FROM audit)`, postgres: `DELETE FROM audit WHERE workspace_id=current_setting('ding.workspace') AND sequence <= (SELECT COALESCE(MAX(sequence),0)-10000 FROM audit)`}

var qAudit1 = statement{sqlite: `SELECT sequence,actor,workspace,grant_id,action,target,at FROM audit WHERE (?=0 OR sequence<?) ORDER BY sequence DESC LIMIT 100`, postgres: `SELECT sequence,actor,workspace,grant_id,action,target,at FROM audit WHERE workspace_id=current_setting('ding.workspace') AND ($1=0 OR sequence<$2) ORDER BY sequence DESC LIMIT 100`}

var qAllOutbox = statement{sqlite: `SELECT id FROM outbox ORDER BY id DESC LIMIT 1000`, postgres: `SELECT id FROM outbox WHERE workspace_id=current_setting('ding.workspace') ORDER BY id DESC LIMIT 1000`}

var qWatchOutbox = statement{sqlite: `SELECT id FROM outbox WHERE watch_id=? ORDER BY id DESC LIMIT 1000`, postgres: `SELECT id FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 ORDER BY id DESC LIMIT 1000`}
