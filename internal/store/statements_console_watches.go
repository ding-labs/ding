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
 (w.id IN (SELECT value FROM jsonb_array_elements_text($1::jsonb))) missing_credentials,
 (coalesce(e.opened,0)>0 OR coalesce(e.unhealthy,0)>0 OR w.last_error!='' OR coalesce(d.failed,0)>0 OR w.id IN (SELECT value FROM jsonb_array_elements_text($2::jsonb)))::int attention
 FROM watches w JOIN watch_revisions r ON r.workspace_id=w.workspace_id AND r.watch_id=w.id AND r.revision=w.revision
 LEFT JOIN entity e ON e.workspace_id=w.workspace_id AND e.watch_id=w.id
 LEFT JOIN delivery d ON d.workspace_id=w.workspace_id AND d.watch_id=w.id
 WHERE w.workspace_id=current_setting('ding.workspace')
) `}
var qWatchFilter = statement{sqlite: ` WHERE (?='' OR instr(lower(id||' '||name),lower(?))>0) AND (?='' OR status=?) AND (?='' OR source=?) AND (?='' OR attention=1) AND (?='' OR id=?) AND (?='' OR (?='open' AND opened>0) OR (?='none' AND opened=0)) AND (?='' OR (?='error' AND (unhealthy>0 OR last_error!='')) OR (?='clear' AND unhealthy=0 AND last_error='') OR (?='missing' AND unhealthy=0 AND last_error='' AND missing_credentials) OR (?='waiting' AND unhealthy=0 AND last_error='' AND NOT missing_credentials AND last_input_at=0) OR (?='received' AND unhealthy=0 AND last_error='' AND NOT missing_credentials AND last_input_at>0)) AND (?='' OR (?='failed' AND failed>0) OR (?='pending' AND pending>0) OR (?='queued' AND pending>0 AND failed=0) OR (?='clear' AND pending=0 AND failed=0)) AND (?='' OR EXISTS(SELECT 1 FROM json_each(targets) target WHERE json_extract(target.value,'$.ref')=?)) AND (?='' OR (?='waiting' AND entities=0) OR (?='open' AND entities>0 AND trigger='transition' AND opened>0) OR (?='clear' AND entities>0 AND trigger='transition' AND opened=0) OR (?='evaluated' AND entities>0 AND trigger!='transition')) AND (?='' OR (?='never' AND last_input_at=0) OR (?='received' AND last_input_at>0) OR (? IN ('hour','day','week') AND last_input_at>=? AND last_input_at>0))`, postgres: ` WHERE ($3='' OR strpos(lower(id||' '||name),lower($4))>0) AND ($5='' OR status=$6) AND ($7='' OR source=$8) AND ($9='' OR attention=1) AND ($10='' OR id=$11) AND ($12='' OR ($13='open' AND opened>0) OR ($14='none' AND opened=0)) AND ($15='' OR ($16='error' AND (unhealthy>0 OR last_error!='')) OR ($17='clear' AND unhealthy=0 AND last_error='') OR ($18='missing' AND unhealthy=0 AND last_error='' AND missing_credentials) OR ($19='waiting' AND unhealthy=0 AND last_error='' AND NOT missing_credentials AND last_input_at=0) OR ($20='received' AND unhealthy=0 AND last_error='' AND NOT missing_credentials AND last_input_at>0)) AND ($21='' OR ($22='failed' AND failed>0) OR ($23='pending' AND pending>0) OR ($24='queued' AND pending>0 AND failed=0) OR ($25='clear' AND pending=0 AND failed=0)) AND ($26='' OR EXISTS(SELECT 1 FROM jsonb_array_elements(targets) target(value) WHERE target.value->>'ref'=$27)) AND ($28='' OR ($29='waiting' AND entities=0) OR ($30='open' AND entities>0 AND trigger='transition' AND opened>0) OR ($31='clear' AND entities>0 AND trigger='transition' AND opened=0) OR ($32='evaluated' AND entities>0 AND trigger!='transition')) AND ($33='' OR ($34='never' AND last_input_at=0) OR ($35='received' AND last_input_at>0) OR ($36 IN ('hour','day','week') AND last_input_at>=$37 AND last_input_at>0))`}
var qWatchTotals = statement{sqlite: `SELECT count(*),coalesce(sum(attention),0),coalesce(sum(status='paused'),0) FROM summary`, postgres: `SELECT count(*),coalesce(sum(attention),0),coalesce(sum((status='paused')::int),0) FROM summary`}
var qWatchTail = statement{sqlite: ` AND id>? ORDER BY id LIMIT ?`, postgres: ` AND id>$38 ORDER BY id LIMIT $39`}
