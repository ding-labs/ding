package store

var qSecretNames = statement{sqlite: `SELECT DISTINCT w.id,j.value FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision, json_tree(r.definition,'$.spec.source') j WHERE j.key='env' AND j.type='text' AND w.status!='deleted'
 UNION SELECT DISTINCT w.id,j.value FROM watches w JOIN watch_revisions r ON r.watch_id=w.id AND r.revision=w.revision, json_each(r.definition,'$.spec.destinations') target JOIN destinations d ON d.id=json_extract(target.value,'$.ref') JOIN destination_revisions dr ON dr.destination_id=d.id AND dr.revision=d.revision, json_tree(dr.definition,'$.spec') j WHERE j.key='env' AND j.type='text' AND w.status!='deleted' ORDER BY 1,2`, postgres: `SELECT DISTINCT w.id,j.value #>> '{}' FROM watches w
 JOIN watch_revisions r ON r.workspace_id=w.workspace_id AND r.watch_id=w.id AND r.revision=w.revision,
 jsonb_path_query(convert_from(r.definition,'UTF8')::jsonb #> '{spec,source}', '$.**.env ? (@.type() == "string")') j(value)
 WHERE w.workspace_id=current_setting('ding.workspace') AND w.status!='deleted'
 UNION SELECT DISTINCT w.id,j.value #>> '{}' FROM watches w
 JOIN watch_revisions r ON r.workspace_id=w.workspace_id AND r.watch_id=w.id AND r.revision=w.revision
 CROSS JOIN LATERAL jsonb_array_elements(convert_from(r.definition,'UTF8')::jsonb #> '{spec,destinations}') target(value)
 JOIN destinations d ON d.workspace_id=w.workspace_id AND d.id=target.value->>'ref'
 JOIN destination_revisions dr ON dr.workspace_id=d.workspace_id AND dr.destination_id=d.id AND dr.revision=d.revision,
 jsonb_path_query(convert_from(dr.definition,'UTF8')::jsonb->'spec', '$.**.env ? (@.type() == "string")') j(value)
 WHERE w.workspace_id=current_setting('ding.workspace') AND w.status!='deleted' ORDER BY 1,2`}

var qConsoleDestinations = statement{sqlite: `SELECT r.definition,d.revision,(SELECT count(DISTINCT w.id) FROM watches w JOIN watch_revisions wr ON wr.watch_id=w.id AND wr.revision=w.revision,json_each(wr.definition,'$.spec.destinations') target WHERE w.status!='deleted' AND json_extract(target.value,'$.ref')=d.id) FROM destinations d JOIN destination_revisions r ON r.destination_id=d.id AND r.revision=d.revision WHERE d.id>? ORDER BY d.id LIMIT ?`, postgres: `SELECT r.definition,d.revision,(SELECT count(DISTINCT w.id) FROM watches w
 JOIN watch_revisions wr ON wr.workspace_id=w.workspace_id AND wr.watch_id=w.id AND wr.revision=w.revision
 CROSS JOIN LATERAL jsonb_array_elements(convert_from(wr.definition,'UTF8')::jsonb #> '{spec,destinations}') target(value)
 WHERE w.workspace_id=d.workspace_id AND w.status!='deleted' AND target.value->>'ref'=d.id)
 FROM destinations d JOIN destination_revisions r ON r.workspace_id=d.workspace_id AND r.destination_id=d.id AND r.revision=d.revision
 WHERE d.workspace_id=current_setting('ding.workspace') AND d.id>$1 ORDER BY d.id LIMIT $2`}

var qEventFilter = statement{sqlite: ` WHERE (?='' OR watch_id=?) AND (?='' OR type=?)`, postgres: ` WHERE workspace_id=current_setting('ding.workspace') AND ($1='' OR watch_id=$2) AND ($3='' OR type=$4)`}
var qConsoleEventProjection = statement{sqlite: `SELECT id,sequence,watch_id,revision,type,at,coalesce(substr(json_extract(body,'$.message'),1,1000),''),coalesce(substr(json_extract(body,'$.entity'),1,500),'') FROM events`, postgres: `SELECT id,sequence,watch_id,revision,type,at,coalesce(substr(convert_from(body,'UTF8')::jsonb->>'message',1,1000),''),coalesce(substr(convert_from(body,'UTF8')::jsonb->>'entity',1,500),'') FROM events`}
var qConsoleDeliveryFilter = statement{sqlite: ` WHERE (?='' OR watch_id=?) AND (?='' OR event_id=?) AND (?='' OR destination_id=?) AND (?='' OR status=?)`, postgres: ` WHERE workspace_id=current_setting('ding.workspace') AND ($1='' OR watch_id=$2) AND ($3='' OR event_id=$4) AND ($5='' OR destination_id=$6) AND ($7='' OR status=$8)`}
