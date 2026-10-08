CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE watch_revisions (
 watch_id TEXT NOT NULL, revision TEXT NOT NULL, fingerprint TEXT NOT NULL,
 definition BLOB NOT NULL, created_at INTEGER NOT NULL,
 PRIMARY KEY(watch_id,revision)
);
CREATE TRIGGER watch_revisions_immutable BEFORE UPDATE ON watch_revisions BEGIN SELECT RAISE(ABORT,'immutable watch revision'); END;
CREATE TABLE watches (
 id TEXT PRIMARY KEY, revision TEXT NOT NULL, generation INTEGER NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('running','paused','deleted')),
 next_at INTEGER NOT NULL, cursor TEXT NOT NULL DEFAULT '', last_input_at INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 FOREIGN KEY(id,revision) REFERENCES watch_revisions(watch_id,revision)
);
CREATE INDEX watches_due ON watches(status,next_at);
CREATE TABLE destination_revisions (
 destination_id TEXT NOT NULL, revision TEXT NOT NULL, definition BLOB NOT NULL,
 created_at INTEGER NOT NULL, PRIMARY KEY(destination_id,revision)
);
CREATE TRIGGER destination_revisions_immutable BEFORE UPDATE ON destination_revisions BEGIN SELECT RAISE(ABORT,'immutable destination revision'); END;
CREATE TABLE destinations (
 id TEXT PRIMARY KEY, revision TEXT NOT NULL,
 FOREIGN KEY(id,revision) REFERENCES destination_revisions(destination_id,revision)
);
CREATE TABLE observations (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT, watch_id TEXT NOT NULL,
 revision TEXT NOT NULL, generation INTEGER NOT NULL, input_id TEXT NOT NULL,
 accepted_at INTEGER NOT NULL, body BLOB NOT NULL,
 UNIQUE(watch_id,generation,input_id),
 FOREIGN KEY(watch_id,revision) REFERENCES watch_revisions(watch_id,revision)
);
CREATE INDEX observations_watch_time ON observations(watch_id,accepted_at);
CREATE TABLE input_receipts (
 watch_id TEXT NOT NULL,generation INTEGER NOT NULL,input_id TEXT NOT NULL,
 first_sequence INTEGER NOT NULL,last_sequence INTEGER NOT NULL,expires_at INTEGER NOT NULL,
 PRIMARY KEY(watch_id,generation,input_id), FOREIGN KEY(watch_id) REFERENCES watches(id)
);
CREATE INDEX receipts_expiry ON input_receipts(expires_at);
CREATE TABLE entities (
 watch_id TEXT NOT NULL,entity_key TEXT NOT NULL,revision TEXT NOT NULL,
 state BLOB NOT NULL,updated_at INTEGER NOT NULL,
 PRIMARY KEY(watch_id,entity_key),
 FOREIGN KEY(watch_id,revision) REFERENCES watch_revisions(watch_id,revision)
);
CREATE INDEX entities_expiry ON entities(updated_at);
CREATE TABLE timers (
 watch_id TEXT NOT NULL,entity_key TEXT NOT NULL,kind TEXT NOT NULL,
 generation INTEGER NOT NULL,due_at INTEGER NOT NULL,body BLOB NOT NULL,
 PRIMARY KEY(watch_id,entity_key,kind), FOREIGN KEY(watch_id) REFERENCES watches(id)
);
CREATE INDEX timers_due ON timers(due_at);
CREATE TABLE events (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT NOT NULL UNIQUE,
 watch_id TEXT NOT NULL,revision TEXT NOT NULL,at INTEGER NOT NULL,type TEXT NOT NULL,body BLOB NOT NULL,
 FOREIGN KEY(watch_id,revision) REFERENCES watch_revisions(watch_id,revision)
);
CREATE INDEX events_watch_sequence ON events(watch_id,sequence);
CREATE INDEX events_retention ON events(at);
CREATE TABLE event_evidence (
 event_id TEXT NOT NULL,observation_sequence INTEGER NOT NULL,
 PRIMARY KEY(event_id,observation_sequence),
 FOREIGN KEY(event_id) REFERENCES events(id) ON DELETE CASCADE,
 FOREIGN KEY(observation_sequence) REFERENCES observations(sequence)
);
CREATE TABLE outbox (
 id INTEGER PRIMARY KEY AUTOINCREMENT,event_id TEXT NOT NULL,watch_id TEXT NOT NULL,
 destination_id TEXT NOT NULL,destination_revision TEXT NOT NULL,
 payload BLOB NOT NULL,status TEXT NOT NULL CHECK(status IN ('pending','leased','delivered','permanent','exhausted','canceled')),
 attempts INTEGER NOT NULL DEFAULT 0,next_at INTEGER NOT NULL,created_at INTEGER NOT NULL,
 lease_token TEXT NOT NULL DEFAULT '',lease_until INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',
 UNIQUE(event_id,destination_id), FOREIGN KEY(event_id) REFERENCES events(id),
 FOREIGN KEY(destination_id,destination_revision) REFERENCES destination_revisions(destination_id,revision)
);
CREATE INDEX outbox_due ON outbox(status,next_at,lease_until);
CREATE INDEX outbox_ordering ON outbox(watch_id,destination_id,id,status);
CREATE TABLE delivery_attempts (
 id INTEGER PRIMARY KEY AUTOINCREMENT,outbox_id INTEGER NOT NULL,
 attempt INTEGER NOT NULL,at INTEGER NOT NULL,outcome TEXT NOT NULL,detail TEXT NOT NULL,
 FOREIGN KEY(outbox_id) REFERENCES outbox(id) ON DELETE CASCADE
);
CREATE INDEX attempts_outbox ON delivery_attempts(outbox_id,id);
