DROP INDEX ding_runtime.observations_watch_time;
CREATE INDEX observations_watch_time ON ding_runtime.observations(workspace_id,watch_id,accepted_at,sequence);
