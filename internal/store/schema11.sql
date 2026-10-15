CREATE INDEX outbox_retention ON outbox(created_at,id) WHERE status NOT IN ('pending','leased');
CREATE INDEX watch_revision_retention ON watch_revisions(created_at,watch_id,revision);
CREATE INDEX destination_revision_retention ON destination_revisions(created_at,destination_id,revision);
