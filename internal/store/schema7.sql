ALTER TABLE watches ADD COLUMN lifetime TEXT NOT NULL DEFAULT '';
UPDATE watches SET lifetime=lower(hex(randomblob(32)));
CREATE TABLE push_receipts (
  lifetime TEXT NOT NULL,
  producer TEXT NOT NULL,
  input_key TEXT NOT NULL,
  digest TEXT NOT NULL,
  body BLOB NOT NULL,
  expires_at INTEGER NOT NULL,
  PRIMARY KEY(lifetime,producer,input_key)
);
CREATE INDEX push_receipts_expiry ON push_receipts(expires_at);
CREATE TABLE push_usage (
  singleton INTEGER PRIMARY KEY CHECK(singleton=1),
  requests INTEGER NOT NULL DEFAULT 0,
  bytes INTEGER NOT NULL DEFAULT 0,
  observations INTEGER NOT NULL DEFAULT 0
);
INSERT INTO push_usage(singleton) VALUES(1);

CREATE TABLE push_last (watch TEXT PRIMARY KEY REFERENCES watches(id) ON DELETE CASCADE, lifetime TEXT NOT NULL, body BLOB NOT NULL);
