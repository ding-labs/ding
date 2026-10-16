CREATE INDEX outbox_retention ON ding_runtime.outbox(workspace_id,created_at,id) WHERE status NOT IN ('pending','leased');
CREATE INDEX watch_revision_retention ON ding_runtime.watch_revisions(workspace_id,created_at,watch_id,revision);
CREATE INDEX destination_revision_retention ON ding_runtime.destination_revisions(workspace_id,created_at,destination_id,revision);
