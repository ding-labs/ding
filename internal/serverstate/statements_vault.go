package state

var qPut1 = statement{sqlite: `SELECT COUNT(*),COUNT(CASE WHEN name=? THEN 1 END) FROM secrets WHERE workspace=?`, postgres: `SELECT COUNT(*),COUNT(CASE WHEN name=$1 THEN 1 END) FROM ding_control.secrets WHERE workspace=$2`}

var qPut2 = statement{sqlite: `INSERT INTO secrets(workspace,name,ciphertext) VALUES(?,?,?) ON CONFLICT(workspace,name) DO UPDATE SET ciphertext=excluded.ciphertext`, postgres: `INSERT INTO ding_control.secrets(workspace,name,ciphertext) VALUES($1,$2,$3) ON CONFLICT(workspace,name) DO UPDATE SET ciphertext=excluded.ciphertext`}

var qGetWithRevision1 = statement{sqlite: `SELECT ciphertext FROM secrets WHERE workspace=? AND name=?`, postgres: `SELECT ciphertext FROM ding_control.secrets WHERE workspace=$1 AND name=$2`}

var qNames1 = statement{sqlite: `SELECT name FROM secrets WHERE workspace=? ORDER BY name`, postgres: `SELECT name FROM ding_control.secrets WHERE workspace=$1 ORDER BY name`}

var qDelete1 = statement{sqlite: `DELETE FROM secrets WHERE workspace=? AND name=?`, postgres: `DELETE FROM ding_control.secrets WHERE workspace=$1 AND name=$2`}

var qMCPWorkspaces1 = statement{sqlite: `SELECT w.id,w.name,m.role FROM mcp_bindings b JOIN memberships m ON m.user=b.user AND m.workspace=b.workspace JOIN workspaces w ON w.id=b.workspace JOIN users u ON u.id=b.user WHERE b.user=? AND b.client=? AND w.deleting=0 AND u.disabled=0 ORDER BY w.id`, postgres: `SELECT w.id,w.name,m.role FROM ding_control.mcp_bindings b JOIN ding_control.memberships m ON m.ding_user=b.ding_user AND m.workspace=b.workspace JOIN ding_control.workspaces w ON w.id=b.workspace JOIN ding_control.users u ON u.id=b.ding_user WHERE b.ding_user=$1 AND b.client=$2 AND w.deleting=false AND u.disabled=false ORDER BY w.id`}

var qResolveMCPWorkspace1 = statement{sqlite: `SELECT b.workspace FROM mcp_bindings b JOIN memberships m ON m.user=b.user AND m.workspace=b.workspace JOIN workspaces w ON w.id=b.workspace WHERE b.user=? AND b.client=? AND w.deleting=0 AND (?='' OR b.workspace=?) LIMIT 2`, postgres: `SELECT b.workspace FROM ding_control.mcp_bindings b JOIN ding_control.memberships m ON m.ding_user=b.ding_user AND m.workspace=b.workspace JOIN ding_control.workspaces w ON w.id=b.workspace WHERE b.ding_user=$1 AND b.client=$2 AND w.deleting=false AND ($3='' OR b.workspace=$4) LIMIT 2`}

var qBindMCP1 = statement{sqlite: `SELECT COUNT(*) FROM mcp_bindings WHERE user=? AND workspace=?`, postgres: `SELECT COUNT(*) FROM ding_control.mcp_bindings WHERE ding_user=$1 AND workspace=$2`}

var qBindMCP2 = statement{sqlite: `INSERT INTO mcp_bindings(user,workspace,client,credential_hash,ciphertext) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO ding_control.mcp_bindings(ding_user,workspace,client,credential_hash,ciphertext) VALUES($1,$2,$3,$4,$5)`}

var qMCP1 = statement{sqlite: `SELECT ciphertext FROM mcp_bindings WHERE user=? AND workspace=? AND client=?`, postgres: `SELECT ciphertext FROM ding_control.mcp_bindings WHERE ding_user=$1 AND workspace=$2 AND client=$3`}

var qMCPClients1 = statement{sqlite: `SELECT client FROM mcp_bindings WHERE user=? AND workspace=? ORDER BY client`, postgres: `SELECT client FROM ding_control.mcp_bindings WHERE ding_user=$1 AND workspace=$2 ORDER BY client`}

var qUnbindMCP1 = statement{sqlite: `DELETE FROM mcp_bindings WHERE user=? AND workspace=? AND client=?`, postgres: `DELETE FROM ding_control.mcp_bindings WHERE ding_user=$1 AND workspace=$2 AND client=$3`}

var qLocalAgentBinding1 = statement{sqlite: `SELECT user,workspace,client FROM mcp_bindings WHERE credential_hash=? AND client LIKE 'local:%'`, postgres: `SELECT ding_user,workspace,client FROM ding_control.mcp_bindings WHERE credential_hash=$1 AND client LIKE 'local:%'`}
