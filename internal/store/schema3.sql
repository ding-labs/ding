-- Keep queue ordering independent of delivered history. Correlated predecessor
-- lookups must not scan every previously delivered event for each candidate.
CREATE INDEX outbox_live_order ON outbox(watch_id,destination_id,id) WHERE status IN ('pending','leased');
CREATE INDEX outbox_live_due ON outbox(next_at,id) WHERE status IN ('pending','leased');
CREATE INDEX outbox_live_lease ON outbox(destination_id,destination_revision,lease_until) WHERE status='leased';
CREATE INDEX evidence_observation ON event_evidence(observation_sequence);
CREATE INDEX observations_retention ON observations(accepted_at);
