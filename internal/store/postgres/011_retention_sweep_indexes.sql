DROP INDEX ding_runtime.observations_retention;
CREATE INDEX observations_retention ON ding_runtime.observations(workspace_id,accepted_at,sequence);
DROP INDEX ding_runtime.events_retention;
CREATE INDEX events_retention ON ding_runtime.events(workspace_id,at,sequence);
CREATE INDEX watches_deleted_retention ON ding_runtime.watches(workspace_id,next_at,id) WHERE status='deleted';
