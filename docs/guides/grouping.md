# Group a watch by entity

Add `groupBy: [service]` under a watch's `spec` and retain `service` in its source
fields. Input such as `{"service":"checkout","latency_ms":350}` then evaluates a
separate state for that service. IDs retain scalar type and field presence.

Each entity keeps its own continuity, incident, baseline, and timers. A watch can
contain an open incident for one service and a normal condition for another.
Inspect the entity summaries and their pagination instead of assigning one
entity's status to the whole watch.

Set `limits.maxEntities` for expected cardinality. Selected fields and group values
are retained evidence: avoid unnecessary identifiers. Quota pressure rejects work
or produces explicit unknown state; it does not pretend a partial exact window
is complete. See [limits and retention](../operate/index.md#limits-and-retention).
