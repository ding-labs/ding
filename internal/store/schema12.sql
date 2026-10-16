CREATE INDEX watches_deleted_retention ON watches(next_at,id) WHERE status='deleted';
