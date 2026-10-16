CREATE TABLE push_receipt_stats (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 total INTEGER NOT NULL CHECK(total>=0),
 expired INTEGER NOT NULL CHECK(expired>=0 AND expired<=total),
 checked_at INTEGER NOT NULL,
 earliest INTEGER NOT NULL
);
INSERT INTO push_receipt_stats(singleton,total,expired,checked_at,earliest)
 SELECT 1,count(*),coalesce(sum(CASE WHEN expires_at<=0 THEN 1 ELSE 0 END),0),
 0,coalesce(min(expires_at),0) FROM push_receipts;

CREATE TRIGGER push_receipt_stats_insert AFTER INSERT ON push_receipts BEGIN
 UPDATE push_receipt_stats SET total=total+1,
 expired=expired+CASE WHEN NEW.expires_at<=checked_at THEN 1 ELSE 0 END,
 earliest=CASE WHEN total=0 THEN NEW.expires_at ELSE min(earliest,NEW.expires_at) END
 WHERE singleton=1;
END;
CREATE TRIGGER push_receipt_stats_delete AFTER DELETE ON push_receipts BEGIN
 UPDATE push_receipt_stats SET total=total-1,
 expired=expired-CASE WHEN OLD.expires_at<=checked_at THEN 1 ELSE 0 END,
 earliest=coalesce((SELECT min(expires_at) FROM push_receipts),0)
 WHERE singleton=1;
END;
CREATE TRIGGER push_receipt_stats_expiry AFTER UPDATE OF expires_at ON push_receipts
 WHEN OLD.expires_at!=NEW.expires_at BEGIN
 UPDATE push_receipt_stats SET
 expired=expired-CASE WHEN OLD.expires_at<=checked_at THEN 1 ELSE 0 END
 +CASE WHEN NEW.expires_at<=checked_at THEN 1 ELSE 0 END,
 earliest=coalesce((SELECT min(expires_at) FROM push_receipts),0)
 WHERE singleton=1;
END;
