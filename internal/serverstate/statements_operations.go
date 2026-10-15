package state

var qReserveBudget1 = statement{sqlite: `DELETE FROM reservations WHERE workspace=? AND created_at<?`, postgres: `DELETE FROM ding_control.reservations WHERE workspace=$1 AND created_at<$2`}

var qReserveBudget2 = statement{sqlite: `SELECT COUNT(*) FROM reservations WHERE workspace=?`, postgres: `SELECT COUNT(*) FROM ding_control.reservations WHERE workspace=$1`}

var qReserveBudget3 = statement{sqlite: `INSERT OR IGNORE INTO usage(workspace,period) VALUES(?,?)`, postgres: `INSERT INTO ding_control.usage(workspace,period) VALUES($1,$2) ON CONFLICT DO NOTHING`}

var qReserveBudget4 = statement{sqlite: `UPDATE usage SET checks=checks+?,deliveries=deliveries+?,bytes=bytes+?
WHERE workspace=? AND period=? AND (?=0 OR checks+?<=?) AND (?=0 OR deliveries+?<=?) AND (?=0 OR bytes+?<=?)`, postgres: `UPDATE ding_control.usage SET checks=checks+$1,deliveries=deliveries+$2,bytes=bytes+$3
WHERE workspace=$4 AND period=$5 AND ($6=0 OR checks+$7<=$8) AND ($9=0 OR deliveries+$10<=$11) AND ($12=0 OR bytes+$13<=$14)`}

var qReserveBudget5 = statement{sqlite: `INSERT INTO reservations(id,workspace,period,created_at) VALUES(?,?,?,?)`, postgres: `INSERT INTO ding_control.reservations(id,workspace,period,created_at) VALUES($1,$2,$3,$4)`}

var qFinish1 = statement{sqlite: `DELETE FROM reservations WHERE id=? AND workspace=? AND period=?`, postgres: `DELETE FROM ding_control.reservations WHERE id=$1 AND workspace=$2 AND period=$3`}

var qFinish2 = statement{sqlite: `UPDATE usage SET bytes=bytes-? WHERE workspace=? AND period=?`, postgres: `UPDATE ding_control.usage SET bytes=bytes-$1 WHERE workspace=$2 AND period=$3`}

var qUsage1 = statement{sqlite: `SELECT COALESCE((SELECT checks FROM usage WHERE workspace=? AND period=?),0),COALESCE((SELECT deliveries FROM usage WHERE workspace=? AND period=?),0),COALESCE((SELECT bytes FROM usage WHERE workspace=? AND period=?),0)`, postgres: `SELECT COALESCE((SELECT checks FROM ding_control.usage WHERE workspace=$1 AND period=$2),0),COALESCE((SELECT deliveries FROM ding_control.usage WHERE workspace=$3 AND period=$4),0),COALESCE((SELECT bytes FROM ding_control.usage WHERE workspace=$5 AND period=$6),0)`}

var qRevision1 = statement{sqlite: `SELECT ciphertext FROM secrets WHERE workspace=? AND name=?`, postgres: `SELECT ciphertext FROM ding_control.secrets WHERE workspace=$1 AND name=$2`}

var qBeginProbe1 = statement{sqlite: `SELECT digest,outcome,at FROM probes WHERE workspace=? AND id=?`, postgres: `SELECT digest,outcome,at FROM ding_control.probes WHERE workspace=$1 AND id=$2`}

var qBeginProbe2 = statement{sqlite: `SELECT COUNT(*) FROM probes WHERE workspace=?`, postgres: `SELECT COUNT(*) FROM ding_control.probes WHERE workspace=$1`}

var qBeginProbe3 = statement{sqlite: `INSERT INTO probes(workspace,id,digest,outcome,at) VALUES(?,?,?,?,?)`, postgres: `INSERT INTO ding_control.probes(workspace,id,digest,outcome,at) VALUES($1,$2,$3,$4,$5)`}

var qFinishProbe1 = statement{sqlite: `UPDATE probes SET outcome=? WHERE workspace=? AND id=? AND outcome='pending'`, postgres: `UPDATE ding_control.probes SET outcome=$1 WHERE workspace=$2 AND id=$3 AND outcome='pending'`}

var qSaveHandoffProof1 = statement{sqlite: `SELECT COUNT(*) FROM handoff_proofs WHERE workspace=? AND id!=?`, postgres: `SELECT COUNT(*) FROM ding_control.handoff_proofs WHERE workspace=$1 AND id!=$2`}

var qSaveHandoffProof2 = statement{sqlite: `INSERT INTO handoff_proofs(workspace,id,body) VALUES(?,?,?) ON CONFLICT(workspace,id) DO UPDATE SET body=excluded.body`, postgres: `INSERT INTO ding_control.handoff_proofs(workspace,id,body) VALUES($1,$2,$3) ON CONFLICT(workspace,id) DO UPDATE SET body=excluded.body`}

var qHandoffProof1 = statement{sqlite: `SELECT body FROM handoff_proofs WHERE workspace=? AND id=?`, postgres: `SELECT body FROM ding_control.handoff_proofs WHERE workspace=$1 AND id=$2`}

var qBeginWorkspaceDelete1 = statement{sqlite: `UPDATE workspaces SET deleting=1 WHERE id=?`, postgres: `UPDATE ding_control.workspaces SET deleting=true WHERE id=$1`}

var qDeletingWorkspaces1 = statement{sqlite: `SELECT id FROM workspaces WHERE deleting=1`, postgres: `SELECT id FROM ding_control.workspaces WHERE deleting=true`}

var qFinishWorkspaceDelete1 = statement{sqlite: `DELETE FROM workspaces WHERE id=? AND deleting=1`, postgres: `DELETE FROM ding_control.workspaces WHERE id=$1 AND deleting=true`}

var qCollect1 = statement{sqlite: `DELETE FROM sessions WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.sessions WHERE expires_at<=$1`}

var qCollect2 = statement{sqlite: `DELETE FROM logins WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.logins WHERE expires_at<=$1`}

var qCollect3 = statement{sqlite: `DELETE FROM devices WHERE expires_at<=?`, postgres: `DELETE FROM ding_control.devices WHERE expires_at<=$1`}

var qCollect4 = statement{sqlite: `DELETE FROM reservations WHERE created_at<?`, postgres: `DELETE FROM ding_control.reservations WHERE created_at<$1`}

var qCollect5 = statement{sqlite: `DELETE FROM usage WHERE period<?`, postgres: `DELETE FROM ding_control.usage WHERE period<$1`}
