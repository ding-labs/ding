-- Integration secrets are stored as SHA-256 digests; returned once at pairing.
CREATE TABLE integration_grants (
 id TEXT PRIMARY KEY, token_hash TEXT NOT NULL UNIQUE, body BLOB NOT NULL,
 revoked INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE integration_previews (
 id TEXT PRIMARY KEY, grant_id TEXT NOT NULL, manifest TEXT NOT NULL,
 review BLOB NOT NULL, expires_at INTEGER NOT NULL,
 FOREIGN KEY(grant_id) REFERENCES integration_grants(id)
);
CREATE INDEX integration_previews_expiry ON integration_previews(expires_at);
-- Receipts deliberately do not expire: an old retry must never become a new action.
CREATE TABLE integration_operations (
 grant_id TEXT NOT NULL, operation_key TEXT NOT NULL, action TEXT NOT NULL,
 digest TEXT NOT NULL, result BLOB NOT NULL, created_at INTEGER NOT NULL,
 PRIMARY KEY(grant_id,operation_key),
 FOREIGN KEY(grant_id) REFERENCES integration_grants(id)
);
