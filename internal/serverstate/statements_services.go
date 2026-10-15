package state

var qCreateBoundService1 = statement{sqlite: `DELETE FROM service_grants WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.service_grants WHERE expires_at<=$1`}

var qCreateBoundService2 = statement{sqlite: `SELECT COUNT(*) FROM service_grants WHERE workspace=?`, postgres: `SELECT COUNT(*) FROM ding_control.service_grants WHERE workspace=$1`}

var qCreateBoundService3 = statement{sqlite: `INSERT INTO service_grants(id,hash,name,workspace,created_by,scopes,watches,expires_at,lifetimes) VALUES(?,?,?,?,?,?,?,?,?)`, postgres: `INSERT INTO ding_control.service_grants(id,hash,name,workspace,created_by,scopes,watches,expires_at,lifetimes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`}

var qCreateBoundService4 = statement{sqlite: `INSERT INTO service_keys(id,service,hash,expires_at,created_at) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO ding_control.service_keys(id,service,hash,expires_at,created_at) VALUES($1,$2,$3,$4,$5)`}

var qServices1 = statement{sqlite: `SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM service_grants WHERE workspace=? ORDER BY id LIMIT 100`, postgres: `SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM ding_control.service_grants WHERE workspace=$1 ORDER BY id LIMIT 100`}

var qServiceToken1 = statement{sqlite: `SELECT s.id,s.name,s.workspace,s.created_by,s.scopes,s.watches,k.expires_at,s.lifetimes,k.id,k.last_used_at FROM service_grants s JOIN service_keys k ON k.service=s.id WHERE k.hash=? AND k.expires_at>? AND s.workspace IN (SELECT id FROM workspaces WHERE deleting=0)`, postgres: `SELECT s.id,s.name,s.workspace,s.created_by,s.scopes,s.watches,k.expires_at,s.lifetimes,k.id,k.last_used_at FROM ding_control.service_grants s JOIN ding_control.service_keys k ON k.service=s.id WHERE k.hash=$1 AND k.expires_at>$2 AND s.workspace IN (SELECT id FROM ding_control.workspaces WHERE deleting=false)`}

var qRevokeService1 = statement{sqlite: `DELETE FROM service_grants WHERE workspace=? AND id=?`, postgres: `DELETE FROM ding_control.service_grants WHERE workspace=$1 AND id=$2`}

var qNoteServiceUse1 = statement{sqlite: `UPDATE service_keys SET last_used_at=? WHERE id=? AND last_used_at<?`, postgres: `UPDATE ding_control.service_keys SET last_used_at=$1 WHERE id=$2 AND last_used_at<$3`}

var qRotateService1 = statement{sqlite: `SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM service_grants WHERE workspace=? AND id=?`, postgres: `SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM ding_control.service_grants WHERE workspace=$1 AND id=$2`}

var qRotateService2 = statement{sqlite: `DELETE FROM service_keys WHERE service=? AND id NOT IN (SELECT id FROM service_keys WHERE service=? ORDER BY created_at DESC,id DESC LIMIT 1)`, postgres: `DELETE FROM ding_control.service_keys WHERE service=$1 AND id NOT IN (SELECT id FROM ding_control.service_keys WHERE service=$2 ORDER BY created_at DESC,id DESC LIMIT 1)`}

var qRotateService3 = statement{sqlite: `UPDATE service_keys SET expires_at=min(expires_at,?) WHERE service=?`, postgres: `UPDATE ding_control.service_keys SET expires_at=least(expires_at,$1) WHERE service=$2`}

var qRotateService4 = statement{sqlite: `INSERT INTO service_keys(id,service,hash,expires_at,created_at) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO ding_control.service_keys(id,service,hash,expires_at,created_at) VALUES($1,$2,$3,$4,$5)`}

var qRotateService5 = statement{sqlite: `UPDATE service_grants SET expires_at=?,hash=? WHERE id=?`, postgres: `UPDATE ding_control.service_grants SET expires_at=$1,hash=$2 WHERE id=$3`}

var qServiceKeys1 = statement{sqlite: `SELECT k.id,k.expires_at,k.last_used_at FROM service_keys k JOIN service_grants s ON s.id=k.service WHERE s.workspace=? AND s.id=? ORDER BY k.created_at DESC`, postgres: `SELECT k.id,k.expires_at,k.last_used_at FROM ding_control.service_keys k JOIN ding_control.service_grants s ON s.id=k.service WHERE s.workspace=$1 AND s.id=$2 ORDER BY k.created_at DESC`}

var qRevokeServiceKey1 = statement{sqlite: `DELETE FROM service_keys WHERE id=? AND service=? AND service IN (SELECT id FROM service_grants WHERE workspace=?)`, postgres: `DELETE FROM ding_control.service_keys WHERE id=$1 AND service=$2 AND service IN (SELECT id FROM ding_control.service_grants WHERE workspace=$3)`}

var qUpdateService1 = statement{sqlite: `UPDATE service_grants SET name=?,scopes=?,watches=?,lifetimes=? WHERE id=? AND workspace=?`, postgres: `UPDATE ding_control.service_grants SET name=$1,scopes=$2,watches=$3,lifetimes=$4 WHERE id=$5 AND workspace=$6`}

var qUpdateService2 = statement{sqlite: `SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM service_grants WHERE id=? AND workspace=?`, postgres: `SELECT id,name,workspace,created_by,scopes,watches,expires_at,lifetimes FROM ding_control.service_grants WHERE id=$1 AND workspace=$2`}
