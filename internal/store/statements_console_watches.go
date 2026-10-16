package store

var qWatchView = statement{sqlite: watchView, postgres: `WITH entity AS (
 SELECT workspace_id,watch_id,count(*) entities,
 coalesce(sum(CASE WHEN (convert_from(state,'UTF8')::jsonb->>'open')::boolean THEN 1 ELSE 0 END),0) opened,
 coalesce(sum(CASE WHEN (convert_from(state,'UTF8')::jsonb->>'sourceUnhealthy')::boolean THEN 1 ELSE 0 END),0) unhealthy
 FROM entities WHERE workspace_id=current_setting('ding.workspace') GROUP BY workspace_id,watch_id
), delivery AS (
 SELECT workspace_id,watch_id,sum((status IN ('permanent','exhausted'))::int) failed,sum((status IN ('pending','leased'))::int) pending
 FROM outbox WHERE workspace_id=current_setting('ding.workspace') GROUP BY workspace_id,watch_id
), summary AS (
 SELECT w.*,convert_from(r.definition,'UTF8')::jsonb #> '{spec,destinations}' targets,
 coalesce(convert_from(r.definition,'UTF8')::jsonb #>> '{metadata,name}','') name,
 convert_from(r.definition,'UTF8')::jsonb #>> '{spec,source,type}' source,
 convert_from(r.definition,'UTF8')::jsonb #>> '{spec,policy,trigger}' trigger,
 coalesce(convert_from(r.definition,'UTF8')::jsonb #>> '{spec,condition,operator}','') operator,
 coalesce(e.entities,0) entities,coalesce(e.opened,0) opened,coalesce(e.unhealthy,0) unhealthy,
 coalesce(d.failed,0) failed,coalesce(d.pending,0) pending,
 (coalesce(e.opened,0)>0 OR coalesce(e.unhealthy,0)>0 OR w.last_error!='' OR coalesce(d.failed,0)>0 OR w.id IN (SELECT value FROM jsonb_array_elements_text($1::jsonb)))::int attention
 FROM watches w JOIN watch_revisions r ON r.workspace_id=w.workspace_id AND r.watch_id=w.id AND r.revision=w.revision
 LEFT JOIN entity e ON e.workspace_id=w.workspace_id AND e.watch_id=w.id
 LEFT JOIN delivery d ON d.workspace_id=w.workspace_id AND d.watch_id=w.id
 WHERE w.workspace_id=current_setting('ding.workspace')
) `}
var qWatchFilter = statement{sqlite: ` WHERE (?='' OR instr(lower(id||' '||name),lower(?))>0) AND (?='' OR status=?) AND (?='' OR source=?) AND (?='' OR attention=1) AND (?='' OR id=?) AND (?='' OR (?='open' AND opened>0) OR (?='none' AND opened=0)) AND (?='' OR (?='error' AND (unhealthy>0 OR last_error!='')) OR (?='clear' AND unhealthy=0 AND last_error='')) AND (?='' OR (?='failed' AND failed>0) OR (?='pending' AND pending>0)) AND (?='' OR EXISTS(SELECT 1 FROM json_each(targets) target WHERE json_extract(target.value,'$.ref')=?))`, postgres: ` WHERE ($2='' OR strpos(lower(id||' '||name),lower($3))>0) AND ($4='' OR status=$5) AND ($6='' OR source=$7) AND ($8='' OR attention=1) AND ($9='' OR id=$10) AND ($11='' OR ($12='open' AND opened>0) OR ($13='none' AND opened=0)) AND ($14='' OR ($15='error' AND (unhealthy>0 OR last_error!='')) OR ($16='clear' AND unhealthy=0 AND last_error='')) AND ($17='' OR ($18='failed' AND failed>0) OR ($19='pending' AND pending>0)) AND ($20='' OR EXISTS(SELECT 1 FROM jsonb_array_elements(targets) target(value) WHERE target.value->>'ref'=$21))`}
var qWatchTotals = statement{sqlite: `SELECT count(*),coalesce(sum(attention),0),coalesce(sum(status='paused'),0) FROM summary`, postgres: `SELECT count(*),coalesce(sum(attention),0),coalesce(sum((status='paused')::int),0) FROM summary`}
var qWatchTail = statement{sqlite: ` AND id>? ORDER BY id LIMIT ?`, postgres: ` AND id>$22 ORDER BY id LIMIT $23`}
