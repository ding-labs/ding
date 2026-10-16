-- Accounting must not depend on a connection's bytea output preference.
ALTER FUNCTION ding_runtime.account_row_bytes() SET bytea_output='hex';
SET LOCAL bytea_output='hex';
ALTER TABLE ding_runtime.workspaces DISABLE ROW LEVEL SECURITY;
UPDATE ding_runtime.workspaces SET logical_bytes=0;
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['metadata','watch_revisions','watches','destination_revisions','destinations','observations','entities','input_receipts','timers','events','event_evidence','event_replays','outbox','delivery_attempts','audit','integration_grants','integration_previews','integration_operations','handoffs','push_receipts','push_usage','push_last','push_receipt_stats'] LOOP
  EXECUTE format('ALTER TABLE ding_runtime.%I DISABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('UPDATE ding_runtime.workspaces w SET logical_bytes=w.logical_bytes+s.n FROM (SELECT workspace_id,sum(octet_length((to_jsonb(r)-''workspace_id'')::text)) n FROM ding_runtime.%I r GROUP BY workspace_id) s WHERE w.workspace_id=s.workspace_id',tab);
  EXECUTE format('ALTER TABLE ding_runtime.%I ENABLE ROW LEVEL SECURITY',tab);
 END LOOP;
END $$;
ALTER TABLE ding_runtime.workspaces ENABLE ROW LEVEL SECURITY;
