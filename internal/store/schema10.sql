CREATE TABLE entity_pins(watch_id TEXT NOT NULL,entity_key TEXT NOT NULL,kind TEXT NOT NULL,value TEXT NOT NULL,
 PRIMARY KEY(watch_id,entity_key,kind,value),FOREIGN KEY(watch_id,entity_key) REFERENCES entities(watch_id,entity_key) ON DELETE CASCADE);
CREATE INDEX entity_pin_reference ON entity_pins(kind,value);
INSERT OR IGNORE INTO entity_pins SELECT e.watch_id,e.entity_key,'input',CAST(j.value AS TEXT) FROM entities e,json_each(json_array(json_extract(e.state,'$.lastSequence'),json_extract(e.state,'$.freshSequence'),json_extract(e.state,'$.baselineSequence'))) j WHERE j.type='integer' AND j.value>0;
INSERT OR IGNORE INTO entity_pins SELECT e.watch_id,e.entity_key,'input',CAST(j.value AS TEXT) FROM entities e,json_each(e.state,'$.evidence') j WHERE j.type='integer' AND j.value>0;
INSERT OR IGNORE INTO entity_pins SELECT e.watch_id,e.entity_key,'input',CAST(json_extract(j.value,'$.sequence') AS TEXT) FROM entities e,json_each(e.state,'$.samples') j WHERE j.type='object' AND json_extract(j.value,'$.sequence')>0;
INSERT OR IGNORE INTO entity_pins SELECT e.watch_id,e.entity_key,'event',j.value FROM entities e,json_each(json_array(json_extract(e.state,'$.incidentEvent'),json_extract(e.state,'$.healthEvent'))) j WHERE j.type='text' AND j.value!='';
