"""Bootstrap owned test roles; never print or return credential values publicly."""
import hashlib
import hmac
import json
import os
from pathlib import Path
from urllib.parse import quote, urlencode, urlsplit, urlunsplit


def connection_url(source, ca, user=None, password=None):
    u = urlsplit(source)
    if u.hostname != 'postgres.railway.internal' or u.scheme not in ('postgres', 'postgresql'):
        raise ValueError('expected the dedicated private PostgreSQL service')
    authority = u.netloc
    if user is not None:
        authority = quote(user, safe='') + ':' + quote(password, safe='') + '@' + u.hostname + ':' + str(u.port or 5432)
    return urlunsplit(('postgres', authority, u.path, urlencode({'sslmode': 'verify-full', 'sslrootcert': str(ca)}), ''))


def bootstrap(root):
    import psycopg
    from psycopg import sql
    if not os.environ.get('DING_TEST_RAILWAY_PROJECT') or os.environ['DING_TEST_RAILWAY_PROJECT'] != os.environ.get('RAILWAY_PROJECT_ID'):
        raise RuntimeError('Railway project authorization mismatch')
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    ca = root/'postgres-ca.crt'
    ca.write_text(os.environ['DING_LAB_PG_CA'])
    admin = connection_url(os.environ['PGADMIN_URL'], ca)
    urls = {'admin': admin}
    for kind, role in [('app', 'ding_app'), ('migration', 'ding_migrator')]:
        password = hmac.new(os.environ['DING_LAB_CONTROL'].encode(), role.encode(), hashlib.sha256).hexdigest()
        urls[kind] = connection_url(admin, ca, role, password)
        with psycopg.connect(admin, autocommit=True) as db:
            row = db.execute('SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname=%s', (role,)).fetchone()
            if row is None:
                db.execute(sql.SQL('CREATE ROLE {} LOGIN PASSWORD {}').format(sql.Identifier(role), sql.Literal(password)))
            elif row != (False, False):
                raise RuntimeError('existing role privileges do not match the lab')
            # Only the owned lab role enables timing. No cluster-wide setting or
            # durability policy changes; dropping the role removes these defaults.
            if kind == 'app':
                db.execute('ALTER ROLE ding_app SET track_io_timing=on')
                db.execute('ALTER ROLE ding_app SET track_wal_io_timing=on')
        # Existing roles must authenticate with this lab's deterministic secret;
        # never alter an unrelated role to seize it after a restart.
        with psycopg.connect(urls[kind]):
            pass
    path = root/'connections.json'
    path.write_text(json.dumps(urls))
    path.chmod(0o600)
    os.environ['DING_TEST_POSTGRES_FILE'] = str(path)
    os.environ['DING_TEST_POSTGRES_HOST'] = 'postgres.railway.internal'
    with psycopg.connect(admin, autocommit=True) as db:
        settings = db.execute("""SELECT version(), current_setting('fsync'), current_setting('synchronous_commit'),
            current_setting('full_page_writes'), current_setting('shared_buffers'), current_setting('max_connections'),
            (SELECT ssl FROM pg_stat_ssl WHERE pid=pg_backend_pid())""").fetchone()
        if settings[1:4] != ('on', 'on', 'on') or settings[-1] is not True:
            raise RuntimeError('database durability or TLS verification failed')
    (root/'database-settings.json').write_text(json.dumps(dict(zip(
        ['version','fsync','synchronousCommit','fullPageWrites','sharedBuffers','maxConnections','tls'], settings)), indent=2))
    return admin


def sample(admin):
    import psycopg
    with psycopg.connect(admin, autocommit=True) as db:
        return db.execute("""SELECT json_build_object('at',clock_timestamp(),
            'cpuStat',pg_read_file('/sys/fs/cgroup/cpu.stat'),
            'cpuMax',pg_read_file('/sys/fs/cgroup/cpu.max'),
            'memoryBytes',pg_read_file('/sys/fs/cgroup/memory.current'),
            'memoryMax',pg_read_file('/sys/fs/cgroup/memory.max'),
            'ioStat',pg_read_file('/sys/fs/cgroup/io.stat'),
            'commits',sum(xact_commit),'rollbacks',sum(xact_rollback),
            'deadlocks',sum(deadlocks),'blocksRead',sum(blks_read),'blocksHit',sum(blks_hit),
            'databaseBytes',(SELECT sum(pg_database_size(oid)) FROM pg_database WHERE NOT datistemplate),
            'wal',(SELECT row_to_json(w) FROM pg_stat_wal w),
            'io',(SELECT json_agg(i) FROM pg_stat_io i WHERE backend_type='client backend'),
            'slru',(SELECT json_agg(s) FROM pg_stat_slru s WHERE name ILIKE 'multixact%'),
            'checkpoints',(SELECT row_to_json(c) FROM pg_stat_checkpointer c),
            'appLockWaiters',(SELECT count(*) FROM pg_stat_activity WHERE usename='ding_app' AND wait_event_type='Lock'),
            'appWaits',(SELECT json_agg(w) FROM (SELECT state,wait_event_type,wait_event,count(*) count
                FROM pg_stat_activity WHERE usename='ding_app'
                GROUP BY state,wait_event_type,wait_event) w),
            'connections',(SELECT count(*) FROM pg_stat_activity)) FROM pg_stat_database""").fetchone()[0]
