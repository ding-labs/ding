CREATE TABLE ding_runtime.events (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 sequence bigint NOT NULL DEFAULT ding_runtime.next_sequence('event'),id text NOT NULL,
 watch_id text NOT NULL,revision text NOT NULL,at bigint NOT NULL,type text NOT NULL,body bytea NOT NULL,
 PRIMARY KEY(workspace_id,sequence),UNIQUE(workspace_id,id),
 FOREIGN KEY(workspace_id,watch_id,revision) REFERENCES ding_runtime.watch_revisions(workspace_id,watch_id,revision)
);
CREATE INDEX events_watch_sequence ON ding_runtime.events(workspace_id,watch_id,sequence);
CREATE INDEX events_retention ON ding_runtime.events(workspace_id,at);
CREATE TABLE ding_runtime.event_evidence (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 event_id text NOT NULL,observation_sequence bigint NOT NULL,
 PRIMARY KEY(workspace_id,event_id,observation_sequence),
 FOREIGN KEY(workspace_id,event_id) REFERENCES ding_runtime.events(workspace_id,id) ON DELETE CASCADE,
 FOREIGN KEY(workspace_id,observation_sequence) REFERENCES ding_runtime.observations(workspace_id,sequence)
);
CREATE INDEX evidence_observation ON ding_runtime.event_evidence(workspace_id,observation_sequence);
CREATE TABLE ding_runtime.event_replays (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 event_id text NOT NULL,body bytea NOT NULL,
 PRIMARY KEY(workspace_id,event_id),
 FOREIGN KEY(workspace_id,event_id) REFERENCES ding_runtime.events(workspace_id,id) ON DELETE CASCADE
);
CREATE TABLE ding_runtime.outbox (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 id bigint NOT NULL DEFAULT ding_runtime.next_sequence('outbox'),event_id text NOT NULL,watch_id text NOT NULL,
 destination_id text NOT NULL,destination_revision text NOT NULL,payload bytea NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','leased','delivered','permanent','exhausted','canceled')),
 attempts integer NOT NULL DEFAULT 0,next_at bigint NOT NULL,created_at bigint NOT NULL,
 lease_token text NOT NULL DEFAULT '',lease_until bigint NOT NULL DEFAULT 0,last_error text NOT NULL DEFAULT '',
 retry_started_at bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(workspace_id,id),UNIQUE(workspace_id,event_id,destination_id),
 FOREIGN KEY(workspace_id,event_id) REFERENCES ding_runtime.events(workspace_id,id),
 FOREIGN KEY(workspace_id,destination_id,destination_revision) REFERENCES ding_runtime.destination_revisions(workspace_id,destination_id,revision)
);
CREATE INDEX outbox_due ON ding_runtime.outbox(workspace_id,status,next_at,lease_until);
CREATE INDEX outbox_ordering ON ding_runtime.outbox(workspace_id,watch_id,destination_id,id,status);
CREATE INDEX outbox_live_order ON ding_runtime.outbox(workspace_id,watch_id,destination_id,id) WHERE status IN ('pending','leased');
CREATE INDEX outbox_live_due ON ding_runtime.outbox(workspace_id,next_at,id) WHERE status IN ('pending','leased');
CREATE INDEX outbox_live_lease ON ding_runtime.outbox(workspace_id,destination_id,destination_revision,lease_until) WHERE status='leased';
CREATE TABLE ding_runtime.delivery_attempts (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 id bigint NOT NULL DEFAULT ding_runtime.next_sequence('attempt'),outbox_id bigint NOT NULL,
 attempt integer NOT NULL,at bigint NOT NULL,outcome text NOT NULL,detail text NOT NULL,
 PRIMARY KEY(workspace_id,id),
 FOREIGN KEY(workspace_id,outbox_id) REFERENCES ding_runtime.outbox(workspace_id,id) ON DELETE CASCADE
);
CREATE INDEX attempts_outbox ON ding_runtime.delivery_attempts(workspace_id,outbox_id,id);
CREATE TABLE ding_runtime.audit (
 workspace_id text NOT NULL DEFAULT nullif(current_setting('ding.workspace',true),''),
 sequence bigint NOT NULL DEFAULT ding_runtime.next_sequence('audit'),
 actor text NOT NULL,workspace text NOT NULL,grant_id text NOT NULL,action text NOT NULL,target text NOT NULL,at bigint NOT NULL,
 PRIMARY KEY(workspace_id,sequence)
);
CREATE FUNCTION ding_runtime.event_cursor_expiry() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime AS $$ BEGIN
 INSERT INTO metadata(key,value) VALUES('event_floor:',OLD.sequence::text)
 ON CONFLICT(workspace_id,key) DO UPDATE SET value=greatest(metadata.value::bigint,OLD.sequence)::text;
 INSERT INTO metadata(key,value) VALUES('event_floor:'||OLD.watch_id,OLD.sequence::text)
 ON CONFLICT(workspace_id,key) DO UPDATE SET value=greatest(metadata.value::bigint,OLD.sequence)::text;
 RETURN OLD;
END $$;
CREATE TRIGGER event_cursor_expiry AFTER DELETE ON ding_runtime.events
 FOR EACH ROW EXECUTE FUNCTION ding_runtime.event_cursor_expiry();
DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['events','event_evidence','event_replays','outbox','delivery_attempts','audit'] LOOP
  EXECUTE format('ALTER TABLE ding_runtime.%I ENABLE ROW LEVEL SECURITY',tab);
  EXECUTE format('ALTER TABLE ding_runtime.%I FORCE ROW LEVEL SECURITY',tab);
  EXECUTE format('CREATE POLICY workspace_scope ON ding_runtime.%I USING (workspace_id=nullif(current_setting(''ding.workspace'',true),'''')) WITH CHECK (workspace_id=nullif(current_setting(''ding.workspace'',true),''''))',tab);
 END LOOP;
END $$;
