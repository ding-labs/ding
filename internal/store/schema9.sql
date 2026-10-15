-- Scheduling state is per independent ordered watch/destination stream.
CREATE TABLE delivery_streams(watch_id TEXT NOT NULL,destination_id TEXT NOT NULL,last_turn INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(watch_id,destination_id));
CREATE INDEX delivery_stream_turn ON delivery_streams(last_turn,watch_id,destination_id);
INSERT INTO delivery_streams(watch_id,destination_id) SELECT DISTINCT watch_id,destination_id FROM outbox;
CREATE INDEX outbox_stream_lease ON outbox(watch_id,destination_id,destination_revision,lease_until) WHERE status='leased';
