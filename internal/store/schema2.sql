CREATE TABLE event_replays (
 event_id TEXT PRIMARY KEY, body BLOB NOT NULL,
 FOREIGN KEY(event_id) REFERENCES events(id) ON DELETE CASCADE
);
INSERT INTO metadata(key,value) VALUES('store_id',lower(hex(randomblob(16))));
CREATE TRIGGER event_cursor_expiry AFTER DELETE ON events BEGIN
 INSERT INTO metadata(key,value) VALUES('event_floor:',CAST(OLD.sequence AS TEXT))
 ON CONFLICT(key) DO UPDATE SET value=CAST(MAX(CAST(value AS INTEGER),OLD.sequence) AS TEXT);
 INSERT INTO metadata(key,value) VALUES('event_floor:'||OLD.watch_id,CAST(OLD.sequence AS TEXT))
 ON CONFLICT(key) DO UPDATE SET value=CAST(MAX(CAST(value AS INTEGER),OLD.sequence) AS TEXT);
END;
ALTER TABLE outbox ADD COLUMN retry_started_at INTEGER NOT NULL DEFAULT 0;
