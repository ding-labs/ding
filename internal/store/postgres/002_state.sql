-- Allocators are row updates, so a rolled-back batch consumes no range. The
-- workspace row also establishes one write order across unrelated tables.
CREATE FUNCTION ding_runtime.next_sequence(kind text) RETURNS bigint
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime AS $$
 DECLARE n bigint;
 BEGIN
  CASE kind
   WHEN 'observation' THEN UPDATE workspaces SET observation_sequence=observation_sequence+1 WHERE workspace_id=current_setting('ding.workspace') RETURNING observation_sequence INTO n;
   WHEN 'event' THEN UPDATE workspaces SET event_sequence=event_sequence+1 WHERE workspace_id=current_setting('ding.workspace') RETURNING event_sequence INTO n;
   WHEN 'outbox' THEN UPDATE workspaces SET outbox_sequence=outbox_sequence+1 WHERE workspace_id=current_setting('ding.workspace') RETURNING outbox_sequence INTO n;
   WHEN 'attempt' THEN UPDATE workspaces SET attempt_sequence=attempt_sequence+1 WHERE workspace_id=current_setting('ding.workspace') RETURNING attempt_sequence INTO n;
   WHEN 'audit' THEN UPDATE workspaces SET audit_sequence=audit_sequence+1 WHERE workspace_id=current_setting('ding.workspace') RETURNING audit_sequence INTO n;
   ELSE RAISE EXCEPTION 'unknown sequence' USING ERRCODE='22023';
  END CASE;
  IF n IS NULL THEN RAISE EXCEPTION 'missing workspace sequence scope' USING ERRCODE='42501'; END IF;
  RETURN n;
 END $$;
CREATE TABLE ding_runtime.observations (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 sequence bigint NOT NULL DEFAULT ding_runtime.next_sequence('observation'),
 watch_id text NOT NULL,revision text NOT NULL,generation bigint NOT NULL,
 input_id text NOT NULL,accepted_at bigint NOT NULL,body bytea NOT NULL,
 PRIMARY KEY(workspace_id,sequence),UNIQUE(workspace_id,watch_id,generation,input_id),
 FOREIGN KEY(workspace_id,watch_id,revision) REFERENCES ding_runtime.watch_revisions(workspace_id,watch_id,revision)
);
CREATE INDEX observations_watch_time ON ding_runtime.observations(workspace_id,watch_id,accepted_at);
CREATE INDEX observations_retention ON ding_runtime.observations(workspace_id,accepted_at);
CREATE TABLE ding_runtime.entities (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 watch_id text NOT NULL,entity_key text NOT NULL,revision text NOT NULL,
 state bytea NOT NULL,updated_at bigint NOT NULL,
 PRIMARY KEY(workspace_id,watch_id,entity_key),
 FOREIGN KEY(workspace_id,watch_id,revision) REFERENCES ding_runtime.watch_revisions(workspace_id,watch_id,revision)
);
CREATE INDEX entities_expiry ON ding_runtime.entities(workspace_id,updated_at);
CREATE TABLE ding_runtime.input_receipts (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 watch_id text NOT NULL,generation bigint NOT NULL,input_id text NOT NULL,
 first_sequence bigint NOT NULL,last_sequence bigint NOT NULL,expires_at bigint NOT NULL,
 PRIMARY KEY(workspace_id,watch_id,generation,input_id),
 FOREIGN KEY(workspace_id,watch_id) REFERENCES ding_runtime.watches(workspace_id,id)
);
CREATE INDEX receipts_expiry ON ding_runtime.input_receipts(workspace_id,expires_at);
CREATE TABLE ding_runtime.timers (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 watch_id text NOT NULL,entity_key text NOT NULL,kind text NOT NULL,
 generation bigint NOT NULL,due_at bigint NOT NULL,body bytea NOT NULL,
 PRIMARY KEY(workspace_id,watch_id,entity_key,kind),
 FOREIGN KEY(workspace_id,watch_id) REFERENCES ding_runtime.watches(workspace_id,id)
);
CREATE INDEX timers_due ON ding_runtime.timers(workspace_id,due_at);
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['observations','entities','input_receipts','timers'] LOOP
  EXECUTE format('ALTER TABLE ding_runtime.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE ding_runtime.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY workspace_scope ON ding_runtime.%I USING (workspace_id=nullif(current_setting(''ding.workspace'',true),'''')) WITH CHECK (workspace_id=nullif(current_setting(''ding.workspace'',true),''''))',tab);
 END LOOP;
END $$;
