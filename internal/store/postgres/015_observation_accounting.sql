-- Preserve canonical row bytes while updating the workspace counter once for
-- an inserted batch. Updates/deletes retain their existing row accounting.
-- This migration changes no retained values and needs no backfill.
CREATE FUNCTION ding_runtime.account_observation_batch() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime SET bytea_output='hex' AS $$
 BEGIN
  UPDATE workspaces w SET logical_bytes=w.logical_bytes+b.bytes
  FROM (SELECT workspace_id,sum(octet_length((to_jsonb(r)-'workspace_id')::text)) bytes
        FROM inserted_observations r GROUP BY workspace_id) b
  WHERE w.workspace_id=b.workspace_id;
  RETURN NULL;
 END $$;
DROP TRIGGER account_row_bytes ON ding_runtime.observations;
CREATE TRIGGER account_row_bytes AFTER UPDATE OR DELETE ON ding_runtime.observations
 FOR EACH ROW EXECUTE FUNCTION ding_runtime.account_row_bytes();
CREATE TRIGGER account_observation_batch AFTER INSERT ON ding_runtime.observations
 REFERENCING NEW TABLE AS inserted_observations
 FOR EACH STATEMENT EXECUTE FUNCTION ding_runtime.account_observation_batch();
