package state

var qNewSession1 = statement{sqlite: `DELETE FROM sessions WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.sessions WHERE expires_at<=$1`}

var qNewSession2 = statement{sqlite: `SELECT COUNT(*) FROM sessions WHERE user=?`, postgres: `SELECT COUNT(*) FROM ding_control.sessions WHERE ding_user=$1`}

var qNewSession3 = statement{sqlite: `SELECT COUNT(*) FROM memberships m JOIN users u ON u.id=m.user JOIN workspaces w ON w.id=m.workspace WHERE m.user=? AND m.workspace=? AND u.disabled=0 AND w.deleting=0`, postgres: `SELECT COUNT(*) FROM ding_control.memberships m JOIN ding_control.users u ON u.id=m.ding_user JOIN ding_control.workspaces w ON w.id=m.workspace WHERE m.ding_user=$1 AND m.workspace=$2 AND u.disabled=false AND w.deleting=false`}

var qNewSession4 = statement{sqlite: `INSERT INTO sessions(hash,user,workspace,scopes,csrf,kind,expires_at) VALUES(?,?,?,?,?,?,?)`, postgres: `INSERT INTO ding_control.sessions(hash,ding_user,workspace,scopes,csrf,kind,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`}

var qSession1 = statement{sqlite: `SELECT user,workspace,scopes,kind,csrf,expires_at FROM sessions WHERE hash=? AND expires_at>? AND user IN (SELECT id FROM users WHERE disabled=0) AND ((kind='browser' AND EXISTS(SELECT 1 FROM memberships m JOIN workspaces w ON w.id=m.workspace WHERE m.user=sessions.user AND w.deleting=0)) OR (kind!='browser' AND workspace IN (SELECT id FROM workspaces WHERE deleting=0)))`, postgres: `SELECT ding_user,workspace,scopes,kind,csrf,expires_at FROM ding_control.sessions WHERE hash=$1 AND expires_at>$2 AND ding_user IN (SELECT id FROM ding_control.users WHERE disabled=false) AND ((kind='browser' AND EXISTS(SELECT 1 FROM ding_control.memberships m JOIN ding_control.workspaces w ON w.id=m.workspace WHERE m.ding_user=sessions.ding_user AND w.deleting=false)) OR (kind!='browser' AND workspace IN (SELECT id FROM ding_control.workspaces WHERE deleting=false)))`}

var qRevokeSession1 = statement{sqlite: `DELETE FROM sessions WHERE hash=?`, postgres: `DELETE FROM ding_control.sessions WHERE hash=$1`}

var qBeginLogin1 = statement{sqlite: `DELETE FROM logins WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.logins WHERE expires_at<=$1`}

var qBeginLogin2 = statement{sqlite: `SELECT COUNT(*) FROM logins`, postgres: `SELECT COUNT(*) FROM ding_control.logins`}

var qBeginLogin3 = statement{sqlite: `INSERT INTO logins(hash,ciphertext,expires_at) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.logins(hash,ciphertext,expires_at) VALUES($1,$2,$3)`}

var qConsumeLogin1 = statement{sqlite: `DELETE FROM logins WHERE hash=? AND expires_at>? RETURNING ciphertext`, postgres: `DELETE FROM ding_control.logins WHERE hash=$1 AND expires_at>$2 RETURNING ciphertext`}

var qBeginDevice1 = statement{sqlite: `DELETE FROM devices WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.devices WHERE expires_at<=$1`}

var qBeginDevice2 = statement{sqlite: `SELECT COUNT(*) FROM devices`, postgres: `SELECT COUNT(*) FROM ding_control.devices`}

var qBeginDevice3 = statement{sqlite: `INSERT INTO devices(id,challenge,expires_at) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.devices(id,challenge,expires_at) VALUES($1,$2,$3)`}

var qDevicePending1 = statement{sqlite: `SELECT COUNT(*) FROM devices WHERE id=? AND workspace IS NULL AND expires_at>?`, postgres: `SELECT COUNT(*) FROM ding_control.devices WHERE id=$1 AND workspace IS NULL AND expires_at>$2`}

var qApproveDevice1 = statement{sqlite: `UPDATE devices SET user=?,workspace=?,scopes=? WHERE id=? AND workspace IS NULL AND expires_at>?`, postgres: `UPDATE ding_control.devices SET ding_user=$1,workspace=$2,scopes=$3 WHERE id=$4 AND workspace IS NULL AND expires_at>$5`}

var qClaimDevice1 = statement{sqlite: `SELECT challenge,COALESCE(user,''),COALESCE(workspace,''),COALESCE(scopes,'[]') FROM devices WHERE id=? AND expires_at>?`, postgres: `SELECT challenge,COALESCE(ding_user,''),COALESCE(workspace,''),COALESCE(scopes,'[]') FROM ding_control.devices WHERE id=$1 AND expires_at>$2`}

var qClaimDevice2 = statement{sqlite: `SELECT m.role FROM memberships m JOIN users u ON u.id=m.user JOIN workspaces w ON w.id=m.workspace WHERE m.user=? AND m.workspace=? AND u.disabled=0 AND w.deleting=0`, postgres: `SELECT m.role FROM ding_control.memberships m JOIN ding_control.users u ON u.id=m.ding_user JOIN ding_control.workspaces w ON w.id=m.workspace WHERE m.ding_user=$1 AND m.workspace=$2 AND u.disabled=false AND w.deleting=false`}

var qClaimDevice3 = statement{sqlite: `DELETE FROM sessions WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.sessions WHERE expires_at<=$1`}

var qClaimDevice4 = statement{sqlite: `SELECT COUNT(*) FROM sessions WHERE user=?`, postgres: `SELECT COUNT(*) FROM ding_control.sessions WHERE ding_user=$1`}

var qClaimDevice5 = statement{sqlite: `INSERT INTO sessions(hash,user,workspace,scopes,csrf,kind,expires_at) VALUES(?,?,?,?,?,?,?)`, postgres: `INSERT INTO ding_control.sessions(hash,ding_user,workspace,scopes,csrf,kind,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`}

var qClaimDevice6 = statement{sqlite: `DELETE FROM devices WHERE id=?`, postgres: `DELETE FROM ding_control.devices WHERE id=$1`}
