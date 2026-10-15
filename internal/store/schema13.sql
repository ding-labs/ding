ALTER TABLE delivery_streams ADD COLUMN active INTEGER NOT NULL DEFAULT 0 CHECK(active IN (0,1));
UPDATE delivery_streams SET active=EXISTS(SELECT 1 FROM outbox o WHERE o.watch_id=delivery_streams.watch_id AND o.destination_id=delivery_streams.destination_id AND o.status IN ('pending','leased'));
CREATE INDEX delivery_stream_active ON delivery_streams(last_turn,watch_id,destination_id) WHERE active=1;
