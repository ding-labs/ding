package store

var qCreateIntegrationGrant1 = statement{sqlite: `SELECT count(*) FROM integration_grants`, postgres: `SELECT count(*) FROM integration_grants WHERE workspace_id=current_setting('ding.workspace')`}

var qCreateIntegrationGrant2 = statement{sqlite: `INSERT INTO integration_grants(id,token_hash,body) VALUES(?,?,?)`, postgres: `INSERT INTO integration_grants(id,token_hash,body) VALUES($1,$2,$3)`}

var qIntegrationGrant1 = statement{sqlite: `SELECT body,revoked FROM integration_grants WHERE id=?`, postgres: `SELECT body,revoked FROM integration_grants WHERE workspace_id=current_setting('ding.workspace') AND id=$1`}

var qIntegrationGrantByHash1 = statement{sqlite: `SELECT id FROM integration_grants WHERE token_hash=?`, postgres: `SELECT id FROM integration_grants WHERE workspace_id=current_setting('ding.workspace') AND token_hash=$1`}

var qIntegrationGrants1 = statement{sqlite: `SELECT body,revoked FROM integration_grants ORDER BY id`, postgres: `SELECT body,revoked FROM integration_grants WHERE workspace_id=current_setting('ding.workspace') ORDER BY id`}

var qRenewProviderIntegrationGrant1 = statement{sqlite: `UPDATE integration_grants SET body=? WHERE id=? AND revoked=0`, postgres: `UPDATE integration_grants SET body=$1 WHERE workspace_id=current_setting('ding.workspace') AND id=$2 AND revoked=false`}

var qRevokeIntegrationGrant1 = statement{sqlite: `UPDATE integration_grants SET revoked=1 WHERE id=?`, postgres: `UPDATE integration_grants SET revoked=true WHERE workspace_id=current_setting('ding.workspace') AND id=$1`}

var qSaveIntegrationPreview1 = statement{sqlite: `DELETE FROM integration_previews WHERE expires_at<=?`, postgres: `DELETE FROM integration_previews WHERE workspace_id=current_setting('ding.workspace') AND expires_at<=$1`}

var qSaveIntegrationPreview2 = statement{sqlite: `SELECT count(*) FROM integration_previews`, postgres: `SELECT count(*) FROM integration_previews WHERE workspace_id=current_setting('ding.workspace')`}

var qSaveIntegrationPreview3 = statement{sqlite: `INSERT INTO integration_previews(id,grant_id,manifest,review,expires_at) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO integration_previews(id,grant_id,manifest,review,expires_at) VALUES($1,$2,$3,$4,$5)`}

var qIntegrationPreview1 = statement{sqlite: `SELECT manifest,review,expires_at FROM integration_previews WHERE id=? AND grant_id=?`, postgres: `SELECT manifest,review,expires_at FROM integration_previews WHERE workspace_id=current_setting('ding.workspace') AND id=$1 AND grant_id=$2`}

var qIntegrationOperation1 = statement{sqlite: `SELECT action,digest,result,created_at FROM integration_operations WHERE grant_id=? AND operation_key=?`, postgres: `SELECT action,digest,result,created_at FROM integration_operations WHERE workspace_id=current_setting('ding.workspace') AND grant_id=$1 AND operation_key=$2`}

var qSaveIntegrationOperation1 = statement{sqlite: `SELECT count(*) FROM integration_operations`, postgres: `SELECT count(*) FROM integration_operations WHERE workspace_id=current_setting('ding.workspace')`}

var qSaveIntegrationOperation2 = statement{sqlite: `INSERT INTO integration_operations(grant_id,operation_key,action,digest,result,created_at) VALUES(?,?,?,?,?,?)`, postgres: `INSERT INTO integration_operations(grant_id,operation_key,action,digest,result,created_at) VALUES($1,$2,$3,$4,$5,$6)`}

var qHandoff1 = statement{sqlite: `SELECT body FROM handoffs WHERE id=?`, postgres: `SELECT body FROM handoffs WHERE workspace_id=current_setting('ding.workspace') AND id=$1`}

var qHandoffs1 = statement{sqlite: `SELECT body FROM handoffs ORDER BY id`, postgres: `SELECT body FROM handoffs WHERE workspace_id=current_setting('ding.workspace') ORDER BY id`}

var qHandoffHold1 = statement{sqlite: `SELECT id FROM handoffs WHERE watch_id=? AND held=1`, postgres: `SELECT id FROM handoffs WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND held=true`}

var qSaveHandoff1 = statement{sqlite: `SELECT COUNT(*) FROM handoffs WHERE id!=?`, postgres: `SELECT COUNT(*) FROM handoffs WHERE workspace_id=current_setting('ding.workspace') AND id!=$1`}

var qSaveHandoff2 = statement{sqlite: `INSERT INTO handoffs(id,watch_id,held,body) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET held=excluded.held,body=excluded.body`, postgres: `INSERT INTO handoffs(id,watch_id,held,body) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,id) DO UPDATE SET held=excluded.held,body=excluded.body`}

var qPendingForWatch1 = statement{sqlite: `SELECT count(*) FROM outbox WHERE watch_id=? AND status IN ('pending','leased')`, postgres: `SELECT count(*) FROM outbox WHERE workspace_id=current_setting('ding.workspace') AND watch_id=$1 AND status IN ('pending','leased')`}
