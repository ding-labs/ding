ALTER TABLE ding_runtime.delivery_streams ADD COLUMN active boolean NOT NULL DEFAULT false;
ALTER TABLE ding_runtime.delivery_streams DISABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.workspaces DISABLE ROW LEVEL SECURITY;
-- Account the added column before normal row-update triggers adjust its value.
UPDATE ding_runtime.workspaces w SET logical_bytes=w.logical_bytes+s.n FROM (
 SELECT workspace_id,sum(octet_length((to_jsonb(r)-'workspace_id')::text)-octet_length((to_jsonb(r)-'workspace_id'-'active')::text)) n
 FROM ding_runtime.delivery_streams r GROUP BY workspace_id) s WHERE w.workspace_id=s.workspace_id;
UPDATE ding_runtime.delivery_streams s SET active=EXISTS(SELECT 1 FROM ding_runtime.outbox o WHERE o.workspace_id=s.workspace_id AND o.watch_id=s.watch_id AND o.destination_id=s.destination_id AND o.status IN ('pending','leased'));
ALTER TABLE ding_runtime.workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.delivery_streams ENABLE ROW LEVEL SECURITY;
CREATE INDEX delivery_stream_active ON ding_runtime.delivery_streams(workspace_id,last_turn,watch_id,destination_id) WHERE active;
