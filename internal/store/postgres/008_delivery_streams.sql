CREATE TABLE ding_runtime.delivery_streams(
 workspace_id text NOT NULL DEFAULT current_setting('ding.workspace'),
 watch_id text NOT NULL,destination_id text NOT NULL,last_turn bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(workspace_id,watch_id,destination_id),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id));
CREATE INDEX delivery_stream_turn ON ding_runtime.delivery_streams(workspace_id,last_turn,watch_id,destination_id);
CREATE INDEX outbox_stream_lease ON ding_runtime.outbox(workspace_id,watch_id,destination_id,destination_revision,lease_until) WHERE status='leased';
ALTER TABLE ding_runtime.outbox DISABLE ROW LEVEL SECURITY;
INSERT INTO ding_runtime.delivery_streams(workspace_id,watch_id,destination_id) SELECT DISTINCT workspace_id,watch_id,destination_id FROM ding_runtime.outbox;
ALTER TABLE ding_runtime.outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.workspaces DISABLE ROW LEVEL SECURITY;
UPDATE ding_runtime.workspaces w SET logical_bytes=w.logical_bytes+s.n FROM (SELECT workspace_id,sum(octet_length((to_jsonb(r)-'workspace_id')::text)) n FROM ding_runtime.delivery_streams r GROUP BY workspace_id) s WHERE w.workspace_id=s.workspace_id;
ALTER TABLE ding_runtime.workspaces ENABLE ROW LEVEL SECURITY;
CREATE TRIGGER account_row_bytes AFTER INSERT OR UPDATE OR DELETE ON ding_runtime.delivery_streams FOR EACH ROW EXECUTE FUNCTION ding_runtime.account_row_bytes();
ALTER TABLE ding_runtime.delivery_streams ENABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.delivery_streams FORCE ROW LEVEL SECURITY;
CREATE POLICY workspace_scope ON ding_runtime.delivery_streams USING(workspace_id=current_setting('ding.workspace',true)) WITH CHECK(workspace_id=current_setting('ding.workspace',true));
