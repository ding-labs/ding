"""Bounded labels for disposable SQL profiles; never export SQL or parameters."""
import re

# All labels are repository-owned names, independent of workspace/input data.
TABLES = ('push_receipts', 'push_usage', 'observations', 'observation_refs',
          'entities', 'watches', 'watch_revisions', 'workspaces', 'outbox',
          'events', 'timers', 'audit', 'service_keys', 'sessions')
FIELDS = ('calls', 'plans', 'total_plan_time', 'total_exec_time', 'rows',
          'shared_blks_hit', 'shared_blks_read', 'wal_bytes')


def classify(query):
    query = query.lower()
    operation = query.split(maxsplit=1)[0] if query.strip() else ''
    if operation not in ('select', 'insert', 'update', 'delete', 'with',
                         'begin', 'commit', 'rollback'):
        return 'other'
    if 'pg_locks' in query or 'ding_meta.owner' in query:
        return 'ownership'
    if 'set_config' in query:
        return 'scope'
    for table in TABLES:
        if re.search(r'\b' + table + r'\b', query):
            return operation + '_' + table
    return operation + '_other'


def summarize(rows):
    totals = {}
    for row in rows:
        label = classify(row.get('query', ''))
        item = totals.setdefault(label, {'operation': label, **dict.fromkeys(FIELDS, 0)})
        for key in FIELDS:
            item[key] += row.get(key, 0) or 0
    return sorted(totals.values(), key=lambda r: r['total_exec_time'] + r['total_plan_time'], reverse=True)
