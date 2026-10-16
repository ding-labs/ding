ALTER TABLE ding_runtime.workspaces ADD COLUMN max_logical_bytes bigint NOT NULL DEFAULT 0 CHECK(max_logical_bytes>=0);

-- Logical quota units are the UTF-8 bytes of each retained row's JSONB text
-- representation, excluding the repeated workspace_id. bytea values use JSON's
-- hex representation. This is deterministic accounting, not physical disk size;
-- PostgreSQL indexes, TOAST, WAL and backups need their own host reserve.
CREATE FUNCTION ding_runtime.account_row_bytes() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime AS $$
 DECLARE delta bigint:=0; scope text;
 BEGIN
  IF TG_OP!='DELETE' THEN
   scope:=NEW.workspace_id;
   delta:=octet_length((to_jsonb(NEW)-'workspace_id')::text);
  END IF;
  IF TG_OP!='INSERT' THEN
   scope:=OLD.workspace_id;
   delta:=delta-octet_length((to_jsonb(OLD)-'workspace_id')::text);
  END IF;
  IF delta<>0 THEN UPDATE workspaces SET logical_bytes=logical_bytes+delta WHERE workspace_id=scope; END IF;
  RETURN NULL;
 END $$;

-- Migrations are offline, under the instance/migration fence. Disabling RLS is
-- restricted to this owner transaction to backfill all existing workspaces;
-- every table regains its forced policy before the migration can commit.
ALTER TABLE ding_runtime.workspaces DISABLE ROW LEVEL SECURITY;
UPDATE ding_runtime.workspaces SET logical_bytes=0;
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['metadata','watch_revisions','watches','destination_revisions','destinations','observations','entities','input_receipts','timers','events','event_evidence','event_replays','outbox','delivery_attempts','audit','integration_grants','integration_previews','integration_operations','handoffs','push_receipts','push_usage','push_last','push_receipt_stats'] LOOP
  EXECUTE format('ALTER TABLE ding_runtime.%I DISABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('UPDATE ding_runtime.workspaces w SET logical_bytes=w.logical_bytes+s.n FROM (SELECT workspace_id,sum(octet_length((to_jsonb(r)-''workspace_id'')::text)) n FROM ding_runtime.%I r GROUP BY workspace_id) s WHERE w.workspace_id=s.workspace_id',tab);
  EXECUTE format('CREATE TRIGGER account_row_bytes AFTER INSERT OR UPDATE OR DELETE ON ding_runtime.%I FOR EACH ROW EXECUTE FUNCTION ding_runtime.account_row_bytes()',tab);
  EXECUTE format('ALTER TABLE ding_runtime.%I ENABLE ROW LEVEL SECURITY',tab);
 END LOOP;
END $$;
ALTER TABLE ding_runtime.workspaces ENABLE ROW LEVEL SECURITY;
