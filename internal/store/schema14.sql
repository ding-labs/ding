-- Active scheduling rows are derived from outbox state, including manual retry.
UPDATE delivery_streams SET active=EXISTS(SELECT 1 FROM outbox o WHERE o.watch_id=delivery_streams.watch_id AND o.destination_id=delivery_streams.destination_id AND o.status IN ('pending','leased'));
INSERT INTO delivery_streams(watch_id,destination_id,active) SELECT DISTINCT watch_id,destination_id,1 FROM outbox WHERE status IN ('pending','leased') ON CONFLICT DO NOTHING;
CREATE TRIGGER stream_enqueue AFTER INSERT ON outbox WHEN NEW.status IN ('pending','leased') BEGIN
 INSERT INTO delivery_streams(watch_id,destination_id,last_turn,active) VALUES(NEW.watch_id,NEW.destination_id,(SELECT coalesce(max(last_turn),0)+1 FROM delivery_streams),1)
 ON CONFLICT(watch_id,destination_id) DO UPDATE SET active=1 WHERE delivery_streams.active=0;
END;
CREATE TRIGGER stream_retry AFTER UPDATE OF status ON outbox WHEN OLD.status NOT IN ('pending','leased') AND NEW.status IN ('pending','leased') BEGIN
 INSERT INTO delivery_streams(watch_id,destination_id,last_turn,active) VALUES(NEW.watch_id,NEW.destination_id,(SELECT coalesce(max(last_turn),0)+1 FROM delivery_streams),1)
 ON CONFLICT(watch_id,destination_id) DO UPDATE SET active=1 WHERE delivery_streams.active=0;
END;
CREATE TRIGGER stream_finish AFTER UPDATE OF status ON outbox WHEN OLD.status IN ('pending','leased') AND NEW.status NOT IN ('pending','leased') BEGIN
 UPDATE delivery_streams SET active=EXISTS(SELECT 1 FROM outbox o WHERE o.watch_id=OLD.watch_id AND o.destination_id=OLD.destination_id AND o.status IN ('pending','leased')) WHERE watch_id=OLD.watch_id AND destination_id=OLD.destination_id;
END;
CREATE TRIGGER stream_remove AFTER DELETE ON outbox WHEN OLD.status IN ('pending','leased') BEGIN
 UPDATE delivery_streams SET active=EXISTS(SELECT 1 FROM outbox o WHERE o.watch_id=OLD.watch_id AND o.destination_id=OLD.destination_id AND o.status IN ('pending','leased')) WHERE watch_id=OLD.watch_id AND destination_id=OLD.destination_id;
END;
