package state

var qCreateWebhook1 = statement{sqlite: `SELECT count(*) FROM webhook_connections WHERE workspace=?`, postgres: `SELECT count(*) FROM ding_control.webhook_connections WHERE workspace=$1`}

var qCreateWebhook2 = statement{sqlite: `INSERT INTO webhook_connections(id,workspace,lifetime,body,secret,previous,previous_until) VALUES(?,?,?,?,?,NULL,0)`, postgres: `INSERT INTO ding_control.webhook_connections(id,workspace,lifetime,body,secret,previous,previous_until) VALUES($1,$2,$3,$4,$5,NULL,0)`}

var qWebhook1 = statement{sqlite: `SELECT h.body,h.lifetime,h.secret,h.previous,h.previous_until FROM webhook_connections h JOIN workspaces w ON w.id=h.workspace WHERE h.id=? AND w.deleting=0`, postgres: `SELECT h.body,h.lifetime,h.secret,h.previous,h.previous_until FROM ding_control.webhook_connections h JOIN ding_control.workspaces w ON w.id=h.workspace WHERE h.id=$1 AND w.deleting=false`}

var qWebhooks1 = statement{sqlite: `SELECT body FROM webhook_connections WHERE workspace=? ORDER BY id`, postgres: `SELECT body FROM ding_control.webhook_connections WHERE workspace=$1 ORDER BY id`}

var qRotateWebhook1 = statement{sqlite: `UPDATE webhook_connections SET previous=secret,previous_until=?,secret=? WHERE id=? AND workspace=?`, postgres: `UPDATE ding_control.webhook_connections SET previous=secret,previous_until=$1,secret=$2 WHERE id=$3 AND workspace=$4`}

var qDeleteWebhook1 = statement{sqlite: `DELETE FROM webhook_connections WHERE id=? AND workspace=?`, postgres: `DELETE FROM ding_control.webhook_connections WHERE id=$1 AND workspace=$2`}
