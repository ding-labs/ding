package state

var qInitializeDisplayName = statement{sqlite: `UPDATE users SET name=? WHERE id=? AND name='' AND disabled=0`, postgres: `UPDATE ding_control.users SET name=$1 WHERE id=$2 AND name='' AND disabled=false`}

var qUpdateDisplayName = statement{sqlite: `UPDATE users SET name=? WHERE id=? AND disabled=0`, postgres: `UPDATE ding_control.users SET name=$1 WHERE id=$2 AND disabled=false`}

var qRenameWorkspace = statement{sqlite: `UPDATE workspaces SET name=? WHERE id=? AND deleting=0`, postgres: `UPDATE ding_control.workspaces SET name=$1 WHERE id=$2 AND deleting=false`}
