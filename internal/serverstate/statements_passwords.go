package state

var qBootstrap1 = statement{sqlite: `SELECT COUNT(*) FROM users`, postgres: `SELECT COUNT(*) FROM ding_control.users`}

var qBootstrap2 = statement{sqlite: `INSERT INTO users(id,name,username,password,created_at) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO ding_control.users(id,name,username,password,created_at) VALUES($1,$2,$3,$4,$5)`}

var qBootstrap3 = statement{sqlite: `INSERT INTO workspaces(id,name,created_at) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.workspaces(id,name,created_at) VALUES($1,$2,$3)`}

var qBootstrap4 = statement{sqlite: `INSERT INTO memberships(user,workspace,role) VALUES(?,?,'owner')`, postgres: `INSERT INTO ding_control.memberships(ding_user,workspace,role) VALUES($1,$2,'owner')`}

var qPasswordUser1 = statement{sqlite: `SELECT id,name,username,password FROM users WHERE username=? AND password IS NOT NULL AND disabled=0`, postgres: `SELECT id,name,username,password FROM ding_control.users WHERE username=$1 AND password IS NOT NULL AND disabled=false`}

var qactivation1 = statement{sqlite: `DELETE FROM activations WHERE user=? OR expires_at<=?`, postgres: `DELETE FROM ding_control.activations WHERE ding_user=$1 OR expires_at<=$2`}

var qactivation2 = statement{sqlite: `INSERT INTO activations(hash,user,workspace,expires_at) VALUES(?,?,?,?)`, postgres: `INSERT INTO ding_control.activations(hash,ding_user,workspace,expires_at) VALUES($1,$2,$3,$4)`}

var qInvite1 = statement{sqlite: `SELECT COUNT(*) FROM users`, postgres: `SELECT COUNT(*) FROM ding_control.users`}

var qInvite2 = statement{sqlite: `INSERT INTO users(id,name,username,created_at) VALUES(?,?,?,?)`, postgres: `INSERT INTO ding_control.users(id,name,username,created_at) VALUES($1,$2,$3,$4)`}

var qInvite3 = statement{sqlite: `INSERT INTO memberships(user,workspace,role) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.memberships(ding_user,workspace,role) VALUES($1,$2,$3)`}

var qResetInvitation1 = statement{sqlite: `SELECT role FROM memberships WHERE user=? AND workspace=?`, postgres: `SELECT role FROM ding_control.memberships WHERE ding_user=$1 AND workspace=$2`}

var qResetInvitation2 = statement{sqlite: `SELECT COUNT(*) FROM memberships WHERE user=?`, postgres: `SELECT COUNT(*) FROM ding_control.memberships WHERE ding_user=$1`}

var qActivate1 = statement{sqlite: `DELETE FROM activations WHERE hash=? AND expires_at>? AND user IN (SELECT id FROM users WHERE disabled=0) RETURNING user,workspace`, postgres: `DELETE FROM ding_control.activations WHERE hash=$1 AND expires_at>$2 AND ding_user IN (SELECT id FROM ding_control.users WHERE disabled=false) RETURNING ding_user,workspace`}

var qActivate2 = statement{sqlite: `UPDATE users SET password=? WHERE id=?`, postgres: `UPDATE ding_control.users SET password=$1 WHERE id=$2`}

var qrevokeUser1 = statement{sqlite: `DELETE FROM sessions WHERE user=?`, postgres: `DELETE FROM ding_control.sessions WHERE ding_user=$1`}

var qrevokeUser2 = statement{sqlite: `DELETE FROM devices WHERE user=?`, postgres: `DELETE FROM ding_control.devices WHERE ding_user=$1`}

var qrevokeUser3 = statement{sqlite: `DELETE FROM activations WHERE user=?`, postgres: `DELETE FROM ding_control.activations WHERE ding_user=$1`}

var qrevokeUser4 = statement{sqlite: `DELETE FROM mcp_bindings WHERE user=?`, postgres: `DELETE FROM ding_control.mcp_bindings WHERE ding_user=$1`}

var qRecoverPassword1 = statement{sqlite: `UPDATE users SET password=?,disabled=0 WHERE username=? RETURNING id`, postgres: `UPDATE ding_control.users SET password=$1,disabled=false WHERE username=$2 RETURNING id`}

var qChangePassword1 = statement{sqlite: `SELECT password FROM users WHERE id=? AND disabled=0 AND password IS NOT NULL`, postgres: `SELECT password FROM ding_control.users WHERE id=$1 AND disabled=false AND password IS NOT NULL`}

var qChangePassword2 = statement{sqlite: `UPDATE users SET password=? WHERE id=?`, postgres: `UPDATE ding_control.users SET password=$1 WHERE id=$2`}

var qSessions1 = statement{sqlite: `SELECT hash,workspace,kind,expires_at FROM sessions WHERE user=? AND expires_at>? ORDER BY expires_at DESC LIMIT 20`, postgres: `SELECT hash,workspace,kind,expires_at FROM ding_control.sessions WHERE ding_user=$1 AND expires_at>$2 ORDER BY expires_at DESC LIMIT 20`}

var qRevokeUserSession1 = statement{sqlite: `DELETE FROM sessions WHERE user=? AND hash=?`, postgres: `DELETE FROM ding_control.sessions WHERE ding_user=$1 AND hash=$2`}

var qDisableUser1 = statement{sqlite: `SELECT role FROM memberships WHERE user=? AND workspace=?`, postgres: `SELECT role FROM ding_control.memberships WHERE ding_user=$1 AND workspace=$2`}

var qDisableUser2 = statement{sqlite: `SELECT COUNT(*) FROM memberships WHERE user=?`, postgres: `SELECT COUNT(*) FROM ding_control.memberships WHERE ding_user=$1`}

var qDisableUser3 = statement{sqlite: `SELECT COUNT(*) FROM memberships m JOIN users u ON u.id=m.user WHERE m.workspace=? AND m.user!=? AND m.role='owner' AND u.disabled=0 AND (u.password IS NOT NULL OR EXISTS(SELECT 1 FROM identities i WHERE i.user=u.id))`, postgres: `SELECT COUNT(*) FROM ding_control.memberships m JOIN ding_control.users u ON u.id=m.ding_user WHERE m.workspace=$1 AND m.ding_user!=$2 AND m.role='owner' AND u.disabled=false AND (u.password IS NOT NULL OR EXISTS(SELECT 1 FROM ding_control.identities i WHERE i.ding_user=u.id))`}

var qDisableUser4 = statement{sqlite: `UPDATE users SET disabled=? WHERE id=?`, postgres: `UPDATE ding_control.users SET disabled=$1 WHERE id=$2`}

var qEnableOIDC1 = statement{sqlite: `SELECT w.id FROM users u JOIN memberships m ON m.user=u.id JOIN workspaces w ON w.id=m.workspace WHERE u.id=? AND u.disabled=0 AND w.deleting=0 ORDER BY w.id LIMIT 1`, postgres: `SELECT w.id FROM ding_control.users u JOIN ding_control.memberships m ON m.ding_user=u.id JOIN ding_control.workspaces w ON w.id=m.workspace WHERE u.id=$1 AND u.disabled=false AND w.deleting=false ORDER BY w.id LIMIT 1`}

var qEnableOIDC2 = statement{sqlite: `SELECT user FROM identities WHERE issuer=? AND subject=?`, postgres: `SELECT ding_user FROM ding_control.identities WHERE issuer=$1 AND subject=$2`}

var qEnableOIDC3 = statement{sqlite: `INSERT INTO identities(issuer,subject,user) VALUES(?,?,?) ON CONFLICT(issuer,subject) DO NOTHING`, postgres: `INSERT INTO ding_control.identities(issuer,subject,ding_user) VALUES($1,$2,$3) ON CONFLICT(issuer,subject) DO NOTHING`}

var qenableOIDCProvider1 = statement{sqlite: `SELECT value FROM settings WHERE key='login_methods'`, postgres: `SELECT value FROM ding_control.settings WHERE key='login_methods'`}

var qenableOIDCProvider2 = statement{sqlite: `INSERT INTO settings(key,value) VALUES('oidc_issuer',?) ON CONFLICT(key) DO NOTHING`, postgres: `INSERT INTO ding_control.settings(key,value) VALUES('oidc_issuer',$1) ON CONFLICT(key) DO NOTHING`}

var qenableOIDCProvider3 = statement{sqlite: `SELECT value FROM settings WHERE key='oidc_issuer'`, postgres: `SELECT value FROM ding_control.settings WHERE key='oidc_issuer'`}

var qenableOIDCProvider4 = statement{sqlite: `UPDATE settings SET value='oidc,password' WHERE key='login_methods'`, postgres: `UPDATE ding_control.settings SET value='oidc,password' WHERE key='login_methods'`}

var qConfigureOIDCIssuer1 = statement{sqlite: `INSERT INTO settings(key,value) VALUES('oidc_issuer',?) ON CONFLICT(key) DO NOTHING`, postgres: `INSERT INTO ding_control.settings(key,value) VALUES('oidc_issuer',$1) ON CONFLICT(key) DO NOTHING`}

var qConfigureOIDCIssuer2 = statement{sqlite: `SELECT value FROM settings WHERE key='oidc_issuer'`, postgres: `SELECT value FROM ding_control.settings WHERE key='oidc_issuer'`}
