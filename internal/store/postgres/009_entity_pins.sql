CREATE TABLE ding_runtime.entity_pins(
 workspace_id text NOT NULL DEFAULT current_setting('ding.workspace'),watch_id text NOT NULL,entity_key text NOT NULL,kind text NOT NULL,value text NOT NULL,
 PRIMARY KEY(workspace_id,watch_id,entity_key,kind,value),
 FOREIGN KEY(workspace_id,watch_id,entity_key) REFERENCES ding_runtime.entities(workspace_id,watch_id,entity_key) ON DELETE CASCADE);
CREATE INDEX entity_pin_reference ON ding_runtime.entity_pins(workspace_id,kind,value);
ALTER TABLE ding_runtime.entities DISABLE ROW LEVEL SECURITY;
INSERT INTO ding_runtime.entity_pins SELECT e.workspace_id,e.watch_id,e.entity_key,'input',j.v::text
 FROM ding_runtime.entities e CROSS JOIN LATERAL (SELECT convert_from(e.state,'UTF8')::jsonb AS b) s
 CROSS JOIN LATERAL jsonb_array_elements(jsonb_build_array(b->'lastSequence',b->'freshSequence',b->'baselineSequence') || coalesce(b->'evidence','[]'::jsonb)) j(v)
 WHERE jsonb_typeof(j.v)='number' AND j.v::text::bigint>0 ON CONFLICT DO NOTHING;
INSERT INTO ding_runtime.entity_pins SELECT e.workspace_id,e.watch_id,e.entity_key,'input',j.v->>'sequence'
 FROM ding_runtime.entities e CROSS JOIN LATERAL jsonb_array_elements(coalesce(convert_from(e.state,'UTF8')::jsonb->'samples','[]'::jsonb)) j(v)
 WHERE (j.v->>'sequence')::bigint>0 ON CONFLICT DO NOTHING;
INSERT INTO ding_runtime.entity_pins SELECT e.workspace_id,e.watch_id,e.entity_key,'event',j.v#>>'{}'
 FROM ding_runtime.entities e CROSS JOIN LATERAL (SELECT convert_from(e.state,'UTF8')::jsonb AS b) s
 CROSS JOIN LATERAL jsonb_array_elements(jsonb_build_array(b->'incidentEvent',b->'healthEvent')) j(v)
 WHERE jsonb_typeof(j.v)='string' AND j.v#>>'{}'!='' ON CONFLICT DO NOTHING;
ALTER TABLE ding_runtime.entities ENABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.workspaces DISABLE ROW LEVEL SECURITY;
UPDATE ding_runtime.workspaces w SET logical_bytes=w.logical_bytes+s.n FROM (SELECT workspace_id,sum(octet_length((to_jsonb(r)-'workspace_id')::text)) n FROM ding_runtime.entity_pins r GROUP BY workspace_id) s WHERE w.workspace_id=s.workspace_id;
ALTER TABLE ding_runtime.workspaces ENABLE ROW LEVEL SECURITY;
CREATE TRIGGER account_row_bytes AFTER INSERT OR UPDATE OR DELETE ON ding_runtime.entity_pins FOR EACH ROW EXECUTE FUNCTION ding_runtime.account_row_bytes();
ALTER TABLE ding_runtime.entity_pins ENABLE ROW LEVEL SECURITY;
ALTER TABLE ding_runtime.entity_pins FORCE ROW LEVEL SECURITY;
CREATE POLICY workspace_scope ON ding_runtime.entity_pins USING(workspace_id=current_setting('ding.workspace',true)) WITH CHECK(workspace_id=current_setting('ding.workspace',true));
