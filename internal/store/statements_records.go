package store

// Named watch/revision SQL. PostgreSQL joins and conflict keys include workspace
// identity; RLS additionally enforces the transaction-local workspace.

var qWatch = statement{
	sqlite:   `SELECT r.definition,r.revision,r.fingerprint,w.generation,w.status,w.next_at,w.cursor,w.last_input_at,w.last_error,w.lifetime FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision WHERE w.id=?`,
	postgres: `SELECT r.definition,r.revision,r.fingerprint,w.generation,w.status,w.next_at,w.cursor,w.last_input_at,w.last_error,w.lifetime FROM watches w JOIN watch_revisions r ON r.workspace_id=w.workspace_id AND r.watch_id=w.id AND r.revision=w.revision WHERE w.workspace_id=current_setting('ding.workspace') AND w.id=$1`,
}

var qWatches = statement{
	sqlite:   `SELECT r.definition,r.revision,r.fingerprint,w.generation,w.status,w.next_at,w.cursor,w.last_input_at,w.last_error,w.lifetime FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision ORDER BY w.id`,
	postgres: `SELECT r.definition,r.revision,r.fingerprint,w.generation,w.status,w.next_at,w.cursor,w.last_input_at,w.last_error,w.lifetime FROM watches w JOIN watch_revisions r ON r.workspace_id=w.workspace_id AND r.watch_id=w.id AND r.revision=w.revision WHERE w.workspace_id=current_setting('ding.workspace') ORDER BY w.id`,
}

var qInsertWatchRevision = statement{
	sqlite:   `INSERT INTO watch_revisions(watch_id,revision,fingerprint,definition,created_at) VALUES(?,?,?,?,?) ON CONFLICT(watch_id,revision) DO NOTHING`,
	postgres: `INSERT INTO watch_revisions(watch_id,revision,fingerprint,definition,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,watch_id,revision) DO NOTHING`,
}

var qSaveWatch = statement{
	sqlite:   `INSERT INTO watches(id,revision,generation,status,next_at,cursor,last_input_at,last_error,lifetime) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,generation=excluded.generation,status=excluded.status,next_at=excluded.next_at,cursor=excluded.cursor,last_input_at=excluded.last_input_at,last_error=excluded.last_error,lifetime=CASE WHEN watches.status='deleted' AND excluded.status!='deleted' THEN excluded.lifetime ELSE watches.lifetime END`,
	postgres: `INSERT INTO watches(id,revision,generation,status,next_at,cursor,last_input_at,last_error,lifetime) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(workspace_id,id) DO UPDATE SET revision=excluded.revision,generation=excluded.generation,status=excluded.status,next_at=excluded.next_at,cursor=excluded.cursor,last_input_at=excluded.last_input_at,last_error=excluded.last_error,lifetime=CASE WHEN watches.status='deleted' AND excluded.status!='deleted' THEN excluded.lifetime ELSE watches.lifetime END`,
}

var qCheckpoint = statement{
	sqlite:   `UPDATE watches SET next_at=?,last_input_at=?,cursor=?,last_error='' WHERE id=? AND generation=? AND status='running'`,
	postgres: `UPDATE watches SET next_at=$1,last_input_at=$2,cursor=$3,last_error='' WHERE workspace_id=current_setting('ding.workspace') AND id=$4 AND generation=$5 AND status='running'`,
}

var qInsertDestinationRevision = statement{
	sqlite:   `INSERT INTO destination_revisions(destination_id,revision,definition,created_at) VALUES(?,?,?,?) ON CONFLICT(destination_id,revision) DO NOTHING`,
	postgres: `INSERT INTO destination_revisions(destination_id,revision,definition,created_at) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,destination_id,revision) DO NOTHING`,
}

var qSaveDestination = statement{
	sqlite:   `INSERT INTO destinations(id,revision) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision`,
	postgres: `INSERT INTO destinations(id,revision) VALUES($1,$2) ON CONFLICT(workspace_id,id) DO UPDATE SET revision=excluded.revision`,
}

var qDestinationRevision = statement{
	sqlite:   `SELECT revision,definition FROM destination_revisions WHERE destination_id=? AND revision=?`,
	postgres: `SELECT revision,definition FROM destination_revisions WHERE workspace_id=current_setting('ding.workspace') AND destination_id=$1 AND revision=$2`,
}

var qCurrentDestination = statement{
	sqlite:   `SELECT r.revision,r.definition FROM destinations d JOIN destination_revisions r ON r.destination_id=d.id AND r.revision=d.revision WHERE d.id=?`,
	postgres: `SELECT r.revision,r.definition FROM destinations d JOIN destination_revisions r ON r.workspace_id=d.workspace_id AND r.destination_id=d.id AND r.revision=d.revision WHERE d.workspace_id=current_setting('ding.workspace') AND d.id=$1`,
}
