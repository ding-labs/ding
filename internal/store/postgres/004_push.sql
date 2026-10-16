CREATE TABLE ding_runtime.push_receipts (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 lifetime text NOT NULL,producer text NOT NULL,input_key text NOT NULL,digest text NOT NULL,body bytea NOT NULL,expires_at bigint NOT NULL,
 PRIMARY KEY(workspace_id,lifetime,producer,input_key),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id)
);
CREATE INDEX push_receipts_expiry ON ding_runtime.push_receipts(workspace_id,expires_at);
CREATE TABLE ding_runtime.push_usage (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 singleton integer NOT NULL CHECK(singleton=1),requests bigint NOT NULL DEFAULT 0,bytes bigint NOT NULL DEFAULT 0,observations bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(workspace_id,singleton),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id)
);
CREATE TABLE ding_runtime.push_last (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 watch text NOT NULL,lifetime text NOT NULL,body bytea NOT NULL,
 PRIMARY KEY(workspace_id,watch),
 FOREIGN KEY(workspace_id,watch) REFERENCES ding_runtime.watches(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE ding_runtime.push_receipt_stats (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 singleton integer NOT NULL CHECK(singleton=1),total bigint NOT NULL CHECK(total>=0),
 expired bigint NOT NULL CHECK(expired>=0 AND expired<=total),checked_at bigint NOT NULL,earliest bigint NOT NULL,
 PRIMARY KEY(workspace_id,singleton),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id)
);
CREATE FUNCTION ding_runtime.push_receipt_stats_change() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime AS $$ BEGIN
 IF TG_OP='INSERT' THEN
  UPDATE push_receipt_stats SET total=total+1,
   expired=expired+CASE WHEN NEW.expires_at<=checked_at THEN 1 ELSE 0 END,
   earliest=CASE WHEN total=0 THEN NEW.expires_at ELSE least(earliest,NEW.expires_at) END
   WHERE workspace_id=NEW.workspace_id AND singleton=1;
 ELSIF TG_OP='DELETE' THEN
  UPDATE push_receipt_stats SET total=total-1,
   expired=expired-CASE WHEN OLD.expires_at<=checked_at THEN 1 ELSE 0 END,
   earliest=coalesce((SELECT min(expires_at) FROM push_receipts WHERE workspace_id=OLD.workspace_id),0)
   WHERE workspace_id=OLD.workspace_id AND singleton=1;
 ELSIF OLD.expires_at<>NEW.expires_at THEN
  UPDATE push_receipt_stats SET
   expired=expired-CASE WHEN OLD.expires_at<=checked_at THEN 1 ELSE 0 END+CASE WHEN NEW.expires_at<=checked_at THEN 1 ELSE 0 END,
   earliest=coalesce((SELECT min(expires_at) FROM push_receipts WHERE workspace_id=NEW.workspace_id),0)
   WHERE workspace_id=NEW.workspace_id AND singleton=1;
 END IF;
 RETURN NULL;
 END $$;
CREATE TRIGGER push_receipt_stats_change AFTER INSERT OR DELETE OR UPDATE OF expires_at ON ding_runtime.push_receipts
 FOR EACH ROW EXECUTE FUNCTION ding_runtime.push_receipt_stats_change();
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['push_receipts','push_usage','push_last','push_receipt_stats'] LOOP
  EXECUTE format('ALTER TABLE ding_runtime.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE ding_runtime.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY workspace_scope ON ding_runtime.%I USING (workspace_id=nullif(current_setting(''ding.workspace'',true),'''')) WITH CHECK (workspace_id=nullif(current_setting(''ding.workspace'',true),''''))',tab);
 END LOOP;
END $$;
