CREATE SCHEMA ding_runtime;
REVOKE ALL ON SCHEMA ding_runtime FROM PUBLIC;

-- All IDs and sequence allocators belong to a workspace. Updating this row
-- serializes that workspace's writes across processes and rolls back its ranges.
CREATE TABLE ding_runtime.workspaces (
 workspace_id text PRIMARY KEY DEFAULT nullif(current_setting('ding.workspace',true),''),
 observation_sequence bigint NOT NULL DEFAULT 0,
 event_sequence bigint NOT NULL DEFAULT 0,
 outbox_sequence bigint NOT NULL DEFAULT 0,
 attempt_sequence bigint NOT NULL DEFAULT 0,
 audit_sequence bigint NOT NULL DEFAULT 0,
 logical_bytes bigint NOT NULL DEFAULT 0 CHECK(logical_bytes>=0)
);
CREATE TABLE ding_runtime.metadata (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 key text NOT NULL,value text NOT NULL,
 PRIMARY KEY(workspace_id,key),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id)
);
CREATE TABLE ding_runtime.watch_revisions (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 watch_id text NOT NULL,revision text NOT NULL,fingerprint text NOT NULL,
 definition bytea NOT NULL,created_at bigint NOT NULL,
 PRIMARY KEY(workspace_id,watch_id,revision),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id)
);
CREATE TABLE ding_runtime.watches (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 id text NOT NULL,revision text NOT NULL,generation bigint NOT NULL,
 status text NOT NULL CHECK(status IN ('running','paused','deleted')),
 next_at bigint NOT NULL,cursor text NOT NULL DEFAULT '',
 last_input_at bigint NOT NULL DEFAULT 0,last_error text NOT NULL DEFAULT '',
 lifetime text NOT NULL,
 PRIMARY KEY(workspace_id,id),
 FOREIGN KEY(workspace_id,id,revision) REFERENCES ding_runtime.watch_revisions(workspace_id,watch_id,revision)
);
CREATE INDEX watches_due ON ding_runtime.watches(workspace_id,status,next_at);
CREATE TABLE ding_runtime.destination_revisions (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 destination_id text NOT NULL,revision text NOT NULL,definition bytea NOT NULL,created_at bigint NOT NULL,
 PRIMARY KEY(workspace_id,destination_id,revision),
 FOREIGN KEY(workspace_id) REFERENCES ding_runtime.workspaces(workspace_id)
);
CREATE TABLE ding_runtime.destinations (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 id text NOT NULL,revision text NOT NULL,
 PRIMARY KEY(workspace_id,id),
 FOREIGN KEY(workspace_id,id,revision) REFERENCES ding_runtime.destination_revisions(workspace_id,destination_id,revision)
);
CREATE FUNCTION ding_runtime.reject_revision_update() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog AS $$
 BEGIN RAISE EXCEPTION 'immutable revision' USING ERRCODE='23514'; END $$;
CREATE TRIGGER immutable_watch_revision BEFORE UPDATE ON ding_runtime.watch_revisions
 FOR EACH ROW EXECUTE FUNCTION ding_runtime.reject_revision_update();
CREATE TRIGGER immutable_destination_revision BEFORE UPDATE ON ding_runtime.destination_revisions
 FOR EACH ROW EXECUTE FUNCTION ding_runtime.reject_revision_update();

-- FORCE also protects accidental use by an owner, although runtime startup
-- separately rejects owner, superuser and BYPASSRLS application roles.
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['workspaces','metadata','watch_revisions','watches','destination_revisions','destinations'] LOOP
  EXECUTE format('ALTER TABLE ding_runtime.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE ding_runtime.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY workspace_scope ON ding_runtime.%I USING (workspace_id=nullif(current_setting(''ding.workspace'',true),'''')) WITH CHECK (workspace_id=nullif(current_setting(''ding.workspace'',true),''''))',tab);
 END LOOP;
END $$;
