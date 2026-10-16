-- Entity/pin set writes must not repeatedly update the same workspace counter.
-- Transition tables retain the exact canonical row-byte definition, including
-- separate INSERT and UPDATE sets from an UPSERT. No retained data changes.
CREATE FUNCTION ding_runtime.account_inserted_rows() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime SET bytea_output='hex' AS $$
 BEGIN
  UPDATE workspaces w SET logical_bytes=w.logical_bytes+b.bytes
  FROM (SELECT workspace_id,sum(octet_length((to_jsonb(r)-'workspace_id')::text)) bytes
        FROM new_rows r GROUP BY workspace_id) b
  WHERE w.workspace_id=b.workspace_id;
  RETURN NULL;
 END $$;

CREATE FUNCTION ding_runtime.account_deleted_rows() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime SET bytea_output='hex' AS $$
 BEGIN
  UPDATE workspaces w SET logical_bytes=w.logical_bytes-b.bytes
  FROM (SELECT workspace_id,sum(octet_length((to_jsonb(r)-'workspace_id')::text)) bytes
        FROM old_rows r GROUP BY workspace_id) b
  WHERE w.workspace_id=b.workspace_id;
  RETURN NULL;
 END $$;

CREATE FUNCTION ding_runtime.account_updated_rows() RETURNS trigger
 LANGUAGE plpgsql SET search_path=pg_catalog,ding_runtime SET bytea_output='hex' AS $$
 BEGIN
  UPDATE workspaces w SET logical_bytes=w.logical_bytes+b.bytes
  FROM (SELECT workspace_id,sum(bytes) bytes FROM (
         SELECT workspace_id,octet_length((to_jsonb(r)-'workspace_id')::text) bytes FROM new_rows r
         UNION ALL
         SELECT workspace_id,-octet_length((to_jsonb(r)-'workspace_id')::text) bytes FROM old_rows r
        ) delta GROUP BY workspace_id HAVING sum(bytes)<>0) b
  WHERE w.workspace_id=b.workspace_id;
  RETURN NULL;
 END $$;

DO $$ DECLARE tab text; BEGIN
 FOREACH tab IN ARRAY ARRAY['entities','entity_pins'] LOOP
  EXECUTE format('DROP TRIGGER account_row_bytes ON ding_runtime.%I',tab);
  EXECUTE format('CREATE TRIGGER account_inserted_rows AFTER INSERT ON ding_runtime.%I REFERENCING NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION ding_runtime.account_inserted_rows()',tab);
  EXECUTE format('CREATE TRIGGER account_deleted_rows AFTER DELETE ON ding_runtime.%I REFERENCING OLD TABLE AS old_rows FOR EACH STATEMENT EXECUTE FUNCTION ding_runtime.account_deleted_rows()',tab);
  EXECUTE format('CREATE TRIGGER account_updated_rows AFTER UPDATE ON ding_runtime.%I REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows FOR EACH STATEMENT EXECUTE FUNCTION ding_runtime.account_updated_rows()',tab);
 END LOOP;
END $$;
