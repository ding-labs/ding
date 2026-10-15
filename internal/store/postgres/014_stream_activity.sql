ALTER TABLE ding_runtime.delivery_streams DISABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.outbox DISABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.workspaces DISABLE ROW LEVEL SECURITY;
UPDATE ding_runtime.delivery_streams s SET active=EXISTS(SELECT 1 FROM ding_runtime.outbox o WHERE o.workspace_id=s.workspace_id AND o.watch_id=s.watch_id AND o.destination_id=s.destination_id AND o.status IN ('pending','leased'));
INSERT INTO ding_runtime.delivery_streams(workspace_id,watch_id,destination_id,active) SELECT DISTINCT workspace_id,watch_id,destination_id,true FROM ding_runtime.outbox WHERE status IN ('pending','leased') ON CONFLICT DO NOTHING;
ALTER TABLE ding_runtime.workspaces ENABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.delivery_streams ENABLE ROW LEVEL SECURITY;
CREATE FUNCTION ding_runtime.stream_activity() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime AS $$
BEGIN
 IF TG_OP='INSERT' OR (TG_OP='UPDATE' AND OLD.status NOT IN ('pending','leased')) THEN
  IF NEW.status IN ('pending','leased') THEN
   INSERT INTO delivery_streams(workspace_id,watch_id,destination_id,last_turn,active)
    VALUES(NEW.workspace_id,NEW.watch_id,NEW.destination_id,(SELECT coalesce(max(last_turn),0)+1 FROM delivery_streams WHERE workspace_id=NEW.workspace_id),true)
    ON CONFLICT(workspace_id,watch_id,destination_id) DO UPDATE SET active=true WHERE NOT delivery_streams.active;
  END IF;
 ELSIF OLD.status IN ('pending','leased') AND (TG_OP='DELETE' OR NEW.status NOT IN ('pending','leased')) THEN
  UPDATE delivery_streams s SET active=EXISTS(SELECT 1 FROM outbox o WHERE o.workspace_id=OLD.workspace_id AND o.watch_id=OLD.watch_id AND o.destination_id=OLD.destination_id AND o.status IN ('pending','leased'))
   WHERE s.workspace_id=OLD.workspace_id AND s.watch_id=OLD.watch_id AND s.destination_id=OLD.destination_id;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER stream_activity AFTER INSERT OR UPDATE OF status OR DELETE ON ding_runtime.outbox FOR EACH ROW EXECUTE FUNCTION ding_runtime.stream_activity();
