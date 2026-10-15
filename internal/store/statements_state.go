package store

var qAppendObservation = statement{sqlite: `INSERT INTO observations(watch_id,revision,generation,input_id,accepted_at,body) VALUES(?,?,?,?,?,?)`, postgres: `INSERT INTO observations(watch_id,revision,generation,input_id,accepted_at,body) VALUES($1,$2,$3,$4,$5,$6) RETURNING sequence`}

var qEntity = statement{sqlite: `SELECT state FROM entities WHERE watch_id=? AND entity_key=?`, postgres: `SELECT state FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND entity_key=$2`}

var qSaveEntity = statement{sqlite: `INSERT INTO entities(watch_id,entity_key,revision,state,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(watch_id,entity_key) DO UPDATE SET revision=excluded.revision,state=excluded.state,updated_at=excluded.updated_at`, postgres: `INSERT INTO entities(watch_id,entity_key,revision,state,updated_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,watch_id,entity_key) DO UPDATE SET revision=excluded.revision,state=excluded.state,updated_at=excluded.updated_at`}

var qReceipt = statement{sqlite: `SELECT first_sequence,last_sequence FROM input_receipts WHERE watch_id=? AND generation=? AND input_id=? AND expires_at>?`, postgres: `SELECT first_sequence,last_sequence FROM input_receipts WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND generation=$2 AND input_id=$3 AND expires_at>$4`}

var qSaveReceipt = statement{sqlite: `INSERT INTO input_receipts(watch_id,generation,input_id,first_sequence,last_sequence,expires_at) VALUES(?,?,?,?,?,?) ON CONFLICT(watch_id,generation,input_id) DO UPDATE SET first_sequence=excluded.first_sequence,last_sequence=excluded.last_sequence,expires_at=excluded.expires_at`, postgres: `INSERT INTO input_receipts(watch_id,generation,input_id,first_sequence,last_sequence,expires_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace_id,watch_id,generation,input_id) DO UPDATE SET first_sequence=excluded.first_sequence,last_sequence=excluded.last_sequence,expires_at=excluded.expires_at`}

var qSaveTimer = statement{sqlite: `INSERT INTO timers(watch_id,entity_key,kind,generation,due_at,body) VALUES(?,?,?,?,?,?) ON CONFLICT(watch_id,entity_key,kind) DO UPDATE SET generation=excluded.generation,due_at=excluded.due_at,body=excluded.body`, postgres: `INSERT INTO timers(watch_id,entity_key,kind,generation,due_at,body) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace_id,watch_id,entity_key,kind) DO UPDATE SET generation=excluded.generation,due_at=excluded.due_at,body=excluded.body`}

var qEntityCount = statement{sqlite: `SELECT count(*) FROM entities WHERE watch_id=?`, postgres: `SELECT count(*) FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1`}

var qEntities = statement{sqlite: `SELECT entity_key,state FROM entities WHERE watch_id=? ORDER BY entity_key`, postgres: `SELECT entity_key,state FROM entities WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 ORDER BY entity_key`}
