-- Transfer holds are persistent ownership fences. Older runtimes must refuse
-- this schema rather than resume a watch without understanding the hold.
CREATE TABLE handoffs (
 id TEXT PRIMARY KEY, watch_id TEXT NOT NULL REFERENCES watches(id),
 held INTEGER NOT NULL CHECK(held IN (0,1)), body BLOB NOT NULL
);
CREATE UNIQUE INDEX handoffs_held_watch ON handoffs(watch_id) WHERE held=1;
