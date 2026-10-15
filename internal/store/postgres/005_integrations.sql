CREATE TABLE ding_runtime.integration_grants (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 id text NOT NULL,token_hash text NOT NULL,body bytea NOT NULL,revoked boolean NOT NULL DEFAULT false,
 PRIMARY KEY(workspace_id,id),UNIQUE(workspace_id,token_hash),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id)
);
CREATE TABLE ding_runtime.integration_previews (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 id text NOT NULL,grant_id text NOT NULL,manifest text NOT NULL,review bytea NOT NULL,expires_at bigint NOT NULL,
 PRIMARY KEY(workspace_id,id),
 FOREIGN KEY(workspace_id,grant_id) REFERENCES ding_runtime.integration_grants(workspace_id,id)
);
CREATE INDEX integration_previews_expiry ON ding_runtime.integration_previews(workspace_id,expires_at);
CREATE TABLE ding_runtime.integration_operations (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 grant_id text NOT NULL,operation_key text NOT NULL,action text NOT NULL,digest text NOT NULL,result bytea NOT NULL,created_at bigint NOT NULL,
 PRIMARY KEY(workspace_id,grant_id,operation_key),
 FOREIGN KEY(workspace_id,grant_id) REFERENCES ding_runtime.integration_grants(workspace_id,id)
);
CREATE TABLE ding_runtime.handoffs (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 id text NOT NULL,watch_id text NOT NULL,held boolean NOT NULL,body bytea NOT NULL,
 PRIMARY KEY(workspace_id,id),
 FOREIGN KEY(workspace_id,watch_id) REFERENCES ding_runtime.watches(workspace_id,id)
);
CREATE UNIQUE INDEX handoffs_held_watch ON ding_runtime.handoffs(workspace_id,watch_id) WHERE held;
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['integration_grants','integration_previews','integration_operations','handoffs'] LOOP
  EXECUTE format('ALTER TABLE ding_runtime.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE ding_runtime.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY workspace_scope ON ding_runtime.%I USING (workspace_id=nullif(current_setting(''ding.workspace'',true),'''')) WITH CHECK (workspace_id=nullif(current_setting(''ding.workspace'',true),''''))',tab);
 END LOOP;
END $$;
