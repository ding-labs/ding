package state

var qEnroll1 = statement{sqlite: `SELECT user FROM identities WHERE issuer=? AND subject=?`, postgres: `SELECT ding_user FROM ding_control.identities WHERE issuer=$1 AND subject=$2`}

var qEnroll2 = statement{sqlite: `SELECT COUNT(*) FROM workspaces`, postgres: `SELECT COUNT(*) FROM ding_control.workspaces`}

var qEnroll3 = statement{sqlite: `INSERT INTO users(id,name,created_at) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.users(id,name,created_at) VALUES($1,$2,$3)`}

var qEnroll4 = statement{sqlite: `INSERT INTO identities(issuer,subject,user) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.identities(issuer,subject,ding_user) VALUES($1,$2,$3)`}

var qEnroll5 = statement{sqlite: `INSERT INTO workspaces(id,name,created_at) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.workspaces(id,name,created_at) VALUES($1,$2,$3)`}

var qEnroll6 = statement{sqlite: `INSERT INTO memberships(user,workspace,role) VALUES(?,?,?)`, postgres: `INSERT INTO ding_control.memberships(ding_user,workspace,role) VALUES($1,$2,$3)`}

var qIdentityUser1 = statement{sqlite: `SELECT u.id,u.name,COALESCE(u.username,''),u.disabled FROM users u JOIN identities i ON i.user=u.id WHERE i.issuer=? AND i.subject=? AND u.disabled=0`, postgres: `SELECT u.id,u.name,COALESCE(u.username,''),u.disabled FROM ding_control.users u JOIN ding_control.identities i ON i.ding_user=u.id WHERE i.issuer=$1 AND i.subject=$2 AND u.disabled=false`}

var qIdentitySubject1 = statement{sqlite: `SELECT i.subject FROM identities i JOIN users u ON u.id=i.user WHERE i.user=? AND i.issuer=? AND u.disabled=0 LIMIT 2`, postgres: `SELECT i.subject FROM ding_control.identities i JOIN ding_control.users u ON u.id=i.ding_user WHERE i.ding_user=$1 AND i.issuer=$2 AND u.disabled=false LIMIT 2`}

var qIdentityWorkspace1 = statement{sqlite: `SELECT w.id,w.name,w.created_at FROM workspaces w JOIN memberships m ON m.workspace=w.id WHERE m.user=? AND w.deleting=0 ORDER BY w.id LIMIT 1`, postgres: `SELECT w.id,w.name,w.created_at FROM ding_control.workspaces w JOIN ding_control.memberships m ON m.workspace=w.id WHERE m.ding_user=$1 AND w.deleting=false ORDER BY w.id LIMIT 1`}

var qUser1 = statement{sqlite: `SELECT id,name,COALESCE(username,''),disabled FROM users WHERE id=?`, postgres: `SELECT id,name,COALESCE(username,''),disabled FROM ding_control.users WHERE id=$1`}

var qPrincipal1 = statement{sqlite: `SELECT m.role FROM memberships m JOIN users u ON u.id=m.user JOIN workspaces w ON w.id=m.workspace WHERE m.user=? AND m.workspace=? AND u.disabled=0 AND w.deleting=0`, postgres: `SELECT m.role FROM ding_control.memberships m JOIN ding_control.users u ON u.id=m.ding_user JOIN ding_control.workspaces w ON w.id=m.workspace WHERE m.ding_user=$1 AND m.workspace=$2 AND u.disabled=false AND w.deleting=false`}

var qMemberships1 = statement{sqlite: `SELECT w.id,w.name,m.role FROM memberships m JOIN workspaces w ON w.id=m.workspace JOIN users u ON u.id=m.user WHERE m.user=? AND u.disabled=0 AND w.deleting=0 ORDER BY w.name,w.id LIMIT 100`, postgres: `SELECT w.id,w.name,m.role FROM ding_control.memberships m JOIN ding_control.workspaces w ON w.id=m.workspace JOIN ding_control.users u ON u.id=m.ding_user WHERE m.ding_user=$1 AND u.disabled=false AND w.deleting=false ORDER BY w.name,w.id LIMIT 100`}

var qMembers1 = statement{sqlite: `SELECT u.id,u.name,COALESCE(u.username,''),u.disabled,m.role FROM memberships m JOIN users u ON u.id=m.user WHERE m.workspace=? ORDER BY u.name,u.id LIMIT 1000`, postgres: `SELECT u.id,u.name,COALESCE(u.username,''),u.disabled,m.role FROM ding_control.memberships m JOIN ding_control.users u ON u.id=m.ding_user WHERE m.workspace=$1 ORDER BY u.name,u.id LIMIT 1000`}

var qmemberActor1 = statement{sqlite: `SELECT m.role FROM memberships m JOIN users u ON u.id=m.user JOIN workspaces w ON w.id=m.workspace WHERE m.user=? AND m.workspace=? AND u.disabled=0 AND w.deleting=0`, postgres: `SELECT m.role FROM ding_control.memberships m JOIN ding_control.users u ON u.id=m.ding_user JOIN ding_control.workspaces w ON w.id=m.workspace WHERE m.ding_user=$1 AND m.workspace=$2 AND u.disabled=false AND w.deleting=false`}

var qChangeMember1 = statement{sqlite: `SELECT role FROM memberships WHERE user=? AND workspace=?`, postgres: `SELECT role FROM ding_control.memberships WHERE ding_user=$1 AND workspace=$2`}

var qChangeMember2 = statement{sqlite: `SELECT COUNT(*) FROM memberships m JOIN users u ON u.id=m.user WHERE workspace=? AND role='owner' AND u.disabled=0 AND (u.password IS NOT NULL OR EXISTS(SELECT 1 FROM identities i WHERE i.user=u.id))`, postgres: `SELECT COUNT(*) FROM ding_control.memberships m JOIN ding_control.users u ON u.id=m.ding_user WHERE workspace=$1 AND role='owner' AND u.disabled=false AND (u.password IS NOT NULL OR EXISTS(SELECT 1 FROM ding_control.identities i WHERE i.ding_user=u.id))`}

var qChangeMember3 = statement{sqlite: `DELETE FROM memberships WHERE user=? AND workspace=?`, postgres: `DELETE FROM ding_control.memberships WHERE ding_user=$1 AND workspace=$2`}

var qChangeMember4 = statement{sqlite: `SELECT COUNT(*) FROM users WHERE id=? AND disabled=0`, postgres: `SELECT COUNT(*) FROM ding_control.users WHERE id=$1 AND disabled=false`}

var qChangeMember5 = statement{sqlite: `INSERT INTO memberships(user,workspace,role) VALUES(?,?,?) ON CONFLICT(user,workspace) DO UPDATE SET role=excluded.role`, postgres: `INSERT INTO ding_control.memberships(ding_user,workspace,role) VALUES($1,$2,$3) ON CONFLICT(ding_user,workspace) DO UPDATE SET role=excluded.role`}

var qChangeMember6 = statement{sqlite: `DELETE FROM sessions WHERE user=? AND workspace=? AND kind!='browser'`, postgres: `DELETE FROM ding_control.sessions WHERE ding_user=$1 AND workspace=$2 AND kind!='browser'`}

var qChangeMember7 = statement{sqlite: `DELETE FROM devices WHERE user=? AND workspace=?`, postgres: `DELETE FROM ding_control.devices WHERE ding_user=$1 AND workspace=$2`}

var qChangeMember8 = statement{sqlite: `DELETE FROM mcp_bindings WHERE user=? AND workspace=?`, postgres: `DELETE FROM ding_control.mcp_bindings WHERE ding_user=$1 AND workspace=$2`}

var qChangeMember9 = statement{sqlite: `DELETE FROM activations WHERE user=? AND workspace=?`, postgres: `DELETE FROM ding_control.activations WHERE ding_user=$1 AND workspace=$2`}

var qDeleteUser1 = statement{sqlite: `SELECT COUNT(*) FROM memberships mine WHERE mine.user=? AND mine.role='owner' AND NOT EXISTS(SELECT 1 FROM memberships other JOIN users u ON u.id=other.user WHERE other.workspace=mine.workspace AND other.user!=mine.user AND other.role='owner' AND u.disabled=0 AND (u.password IS NOT NULL OR EXISTS(SELECT 1 FROM identities i WHERE i.user=u.id)))`, postgres: `SELECT COUNT(*) FROM ding_control.memberships mine WHERE mine.ding_user=$1 AND mine.role='owner' AND NOT EXISTS(SELECT 1 FROM ding_control.memberships other JOIN ding_control.users u ON u.id=other.ding_user WHERE other.workspace=mine.workspace AND other.ding_user!=mine.ding_user AND other.role='owner' AND u.disabled=false AND (u.password IS NOT NULL OR EXISTS(SELECT 1 FROM ding_control.identities i WHERE i.ding_user=u.id)))`}

var qDeleteUser2 = statement{sqlite: `DELETE FROM users WHERE id=?`, postgres: `DELETE FROM ding_control.users WHERE id=$1`}

var qaudit1 = statement{sqlite: `INSERT INTO audit(actor,workspace,action,target,at) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO ding_control.audit(actor,workspace,action,target,at) VALUES($1,$2,$3,$4,$5)`}

var qaudit2 = statement{sqlite: `DELETE FROM audit WHERE sequence <= (SELECT COALESCE(MAX(sequence),0)-10000 FROM audit)`, postgres: `DELETE FROM ding_control.audit WHERE sequence <= (SELECT COALESCE(MAX(sequence),0)-10000 FROM ding_control.audit)`}

var qAudit1 = statement{sqlite: `SELECT sequence,actor,workspace,action,target,at FROM audit WHERE workspace=? AND (?=0 OR sequence<?) ORDER BY sequence DESC LIMIT 100`, postgres: `SELECT sequence,actor,workspace,action,target,at FROM ding_control.audit WHERE workspace=$1 AND ($2=0 OR sequence<$3) ORDER BY sequence DESC LIMIT 100`}

var qConfigureDataKey1 = statement{sqlite: `INSERT INTO settings(key,value) VALUES('data_key',?) ON CONFLICT(key) DO NOTHING`, postgres: `INSERT INTO ding_control.settings(key,value) VALUES('data_key',$1) ON CONFLICT(key) DO NOTHING`}

var qConfigureDataKey2 = statement{sqlite: `SELECT value FROM settings WHERE key='data_key'`, postgres: `SELECT value FROM ding_control.settings WHERE key='data_key'`}

var qConfigureHosting1 = statement{sqlite: `INSERT INTO settings(key,value) VALUES('hosting',?) ON CONFLICT(key) DO NOTHING`, postgres: `INSERT INTO ding_control.settings(key,value) VALUES('hosting',$1) ON CONFLICT(key) DO NOTHING`}

var qConfigureHosting2 = statement{sqlite: `SELECT value FROM settings WHERE key='hosting'`, postgres: `SELECT value FROM ding_control.settings WHERE key='hosting'`}

var qConfigureLoginMethods1 = statement{sqlite: `INSERT INTO settings(key,value) VALUES('login_methods',?) ON CONFLICT(key) DO NOTHING`, postgres: `INSERT INTO ding_control.settings(key,value) VALUES('login_methods',$1) ON CONFLICT(key) DO NOTHING`}

var qConfigureLoginMethods2 = statement{sqlite: `SELECT value FROM settings WHERE key='login_methods'`, postgres: `SELECT value FROM ding_control.settings WHERE key='login_methods'`}

var qWorkspace1 = statement{sqlite: `SELECT id,name,created_at FROM workspaces WHERE id=? AND deleting=0`, postgres: `SELECT id,name,created_at FROM ding_control.workspaces WHERE id=$1 AND deleting=false`}

var qWorkspaces1 = statement{sqlite: `SELECT id,name,created_at FROM workspaces WHERE deleting=0 ORDER BY id`, postgres: `SELECT id,name,created_at FROM ding_control.workspaces WHERE deleting=false ORDER BY id`}
