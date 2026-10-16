package store

var qPushReceipt = statement{sqlite: `SELECT digest,body FROM push_receipts WHERE lifetime=? AND producer=? AND input_key=? AND expires_at>?`, postgres: `SELECT digest,body FROM push_receipts WHERE workspace_id=current_setting('ding.workspace') AND lifetime=$1 AND producer=$2 AND input_key=$3 AND expires_at>$4`}

var qSavePushReceipt = statement{sqlite: `INSERT INTO push_receipts(lifetime,producer,input_key,digest,body,expires_at) VALUES(?,?,?,?,?,?) ON CONFLICT(lifetime,producer,input_key) DO UPDATE SET digest=excluded.digest,body=excluded.body,expires_at=excluded.expires_at`, postgres: `INSERT INTO push_receipts(lifetime,producer,input_key,digest,body,expires_at) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace_id,lifetime,producer,input_key) DO UPDATE SET digest=excluded.digest,body=excluded.body,expires_at=excluded.expires_at`}

var qSaveLastPush = statement{sqlite: `INSERT INTO push_last(watch,lifetime,body) VALUES(?,?,?) ON CONFLICT(watch) DO UPDATE SET lifetime=excluded.lifetime,body=excluded.body`, postgres: `INSERT INTO push_last(watch,lifetime,body) VALUES($1,$2,$3) ON CONFLICT(workspace_id,watch) DO UPDATE SET lifetime=excluded.lifetime,body=excluded.body`}

var qAddPushUsage = statement{sqlite: `UPDATE push_usage SET requests=requests+1,bytes=bytes+?,observations=observations+? WHERE singleton=1`, postgres: `UPDATE push_usage SET requests=requests+1,bytes=bytes+$1,observations=observations+$2 WHERE workspace_id=current_setting('ding.workspace') AND singleton=1`}

var qPushReceiptStats = statement{sqlite: `SELECT total,expired,checked_at,earliest FROM push_receipt_stats WHERE singleton=1`, postgres: `SELECT total,expired,checked_at,earliest FROM push_receipt_stats WHERE workspace_id=current_setting('ding.workspace') AND singleton=1`}

var qExpiredPushCount = statement{sqlite: `SELECT count(*) FROM push_receipts WHERE expires_at<=?`, postgres: `SELECT count(*) FROM push_receipts WHERE workspace_id=current_setting('ding.workspace') AND expires_at<=$1`}

var qNewExpiredPushCount = statement{sqlite: `SELECT count(*) FROM push_receipts WHERE expires_at>? AND expires_at<=?`, postgres: `SELECT count(*) FROM push_receipts WHERE workspace_id=current_setting('ding.workspace') AND expires_at>$1 AND expires_at<=$2`}

var qAdvancePushExpiry = statement{sqlite: `UPDATE push_receipt_stats SET expired=?,checked_at=? WHERE singleton=1`, postgres: `UPDATE push_receipt_stats SET expired=$1,checked_at=$2 WHERE workspace_id=current_setting('ding.workspace') AND singleton=1`}

var qRebuildPushCount = statement{sqlite: `SELECT count(*),coalesce(sum(CASE WHEN expires_at<=? THEN 1 ELSE 0 END),0),coalesce(min(expires_at),0) FROM push_receipts`, postgres: `SELECT count(*),coalesce(sum(CASE WHEN expires_at<=$1 THEN 1 ELSE 0 END),0),coalesce(min(expires_at),0) FROM push_receipts WHERE workspace_id=current_setting('ding.workspace')`}

var qSavePushCount = statement{sqlite: `INSERT INTO push_receipt_stats(singleton,total,expired,checked_at,earliest) VALUES(1,?,?,?,?)`, postgres: `INSERT INTO push_receipt_stats(singleton,total,expired,checked_at,earliest) VALUES(1,$1,$2,$3,$4)`}

var qPushUsage = statement{sqlite: `SELECT requests,bytes,observations FROM push_usage WHERE singleton=1`, postgres: `SELECT requests,bytes,observations FROM push_usage WHERE workspace_id=current_setting('ding.workspace') AND singleton=1`}

var qLastPush = statement{sqlite: `SELECT body FROM push_last WHERE lifetime=? AND watch=?`, postgres: `SELECT body FROM push_last WHERE workspace_id=current_setting('ding.workspace') AND lifetime=$1 AND watch=$2`}
