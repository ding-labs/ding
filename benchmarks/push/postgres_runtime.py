"""Private, owned PostgreSQL resources for the local capacity runner."""
import atexit
import json
import os
from pathlib import Path
import subprocess
import sys
from urllib.parse import urlsplit, urlunsplit
from resources import cgroup_sample
from postgres_profile import summarize

HERE = Path(__file__).resolve().parent


def output(args):
    return subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL).strip()


class LocalPostgres:
    def __init__(self, root, environment, units, cpu_share=.5, *, cpus=None, memory_mib=None, profiles=False):
        cpus = units*cpu_share if cpus is None else cpus
        memory_mib = units*512 if memory_mib is None else memory_mib
        self.profiles = profiles
        self.root = root
        self.root.mkdir(mode=0o700)
        self.env = dict(os.environ, DING_PG_CPUS=str(cpus),
                        DING_PG_MEMORY=str(memory_mib)+'m',
                        DING_PG_PROFILE_LIBRARY='pg_stat_statements' if profiles else '')
        self.stopped = False
        with (root/'setup.log').open('x') as log:
            subprocess.run([sys.executable, str(HERE/'postgres.py'), 'start', '--root', str(root)],
                           env=self.env, stdout=log, stderr=subprocess.STDOUT, check=True)
        self.manifest = json.loads((root/'lab.json').read_text())
        self.container = self.manifest['container']
        atexit.register(self.stop)
        if profiles:
            output(['docker', 'exec', self.container, 'psql', '-U', 'postgres', '-d', 'postgres',
                    '-c', 'CREATE EXTENSION pg_stat_statements'])
        self.connections = root/'connections.json'
        if environment == 'container':
            values = json.loads(self.connections.read_text())
            for role, value in values.items():
                u = urlsplit(value)
                userinfo = u.netloc.rsplit('@', 1)[0]
                values[role] = urlunsplit((u.scheme, userinfo+'@127.0.0.1:5432', u.path, u.query, ''))
            self.connections = root/'container-connections.json'
            fd = os.open(self.connections, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, 'w') as f:
                json.dump(values, f)
        info = json.loads(output(['docker', 'inspect', self.container]))[0]
        limits = info['HostConfig']
        if limits['NanoCpus'] != int(cpus*1e9) or limits['Memory'] != memory_mib*1024**2 or limits['MemorySwap'] != limits['Memory']:
            raise RuntimeError('PostgreSQL resource limits do not match the declared budget')
        self.metadata = dict(self.manifest, cpuLimit=cpus, memoryBytes=limits['Memory'],
                             memorySwapBytes=limits['MemorySwap'], settings=self.sql("""SELECT json_build_object(
                'statementProfiles',current_setting('shared_preload_libraries'),'version',version(),'fsync',current_setting('fsync'),
                'fullPageWrites',current_setting('full_page_writes'),
                'synchronousCommit',current_setting('synchronous_commit'),
                'trackIOTiming',current_setting('track_io_timing'),
                'trackWALIOTiming',current_setting('track_wal_io_timing'),
                'sharedBuffers',current_setting('shared_buffers'),
                'maxConnections',current_setting('max_connections'))"""))
        settings = self.metadata['settings']
        if any(settings[key] != 'on' for key in ['fsync', 'fullPageWrites', 'synchronousCommit']):
            raise RuntimeError('PostgreSQL durability is not enabled')

    def sql(self, query):
        # Only the owned local container's Unix socket is used. No DSNs or
        # passwords are placed in arguments, output, or exported metadata.
        raw = output(['docker', 'exec', self.container, 'psql', '-U', 'postgres', '-d', 'postgres', '-Atc', query])
        return json.loads(raw)

    def sample(self):
        result = {'runtime': json.loads(output(['docker', 'stats', '--no-stream', '--format', '{{json .}}', self.container])),
                'cpuStat': cgroup_sample(self.container),
                'counters': self.sql("""SELECT json_build_object(
                    'at',clock_timestamp(),'commits',sum(xact_commit),'rollbacks',sum(xact_rollback),
                    'blocksRead',sum(blks_read),'blocksHit',sum(blks_hit),
                    'readMS',sum(blk_read_time),'writeMS',sum(blk_write_time),
                    'tempBytes',sum(temp_bytes),'deadlocks',sum(deadlocks),
                    'checkpoints',(SELECT row_to_json(c) FROM pg_stat_checkpointer c),
                    'appLockWaiters',(SELECT count(*) FROM pg_stat_activity
                        WHERE usename='ding_app' AND wait_event_type='Lock'),
                    'oldestAppTransactionSeconds',(SELECT coalesce(max(extract(epoch FROM clock_timestamp()-xact_start)),0)
                        FROM pg_stat_activity WHERE usename='ding_app'),
                    'wal',(SELECT row_to_json(w) FROM pg_stat_wal w))
                    FROM pg_stat_database"""),
                'storage': self.sql("""SELECT json_build_object(
                    'databaseBytes',sum(pg_database_size(oid)),
                    'walBytes',(SELECT sum(size) FROM pg_ls_waldir()),
                    'walPosition',pg_current_wal_lsn()::text,
                    'connections',(SELECT count(*) FROM pg_stat_activity))
                    FROM pg_database WHERE NOT datistemplate""")}

        if self.profiles:
            # Read only app-role statistics, never bootstrap credentials. SQL is
            # classified in memory and omitted from the exported measurements.
            rows = self.sql("""SELECT coalesce(json_agg(s), '[]'::json) FROM (
                SELECT query,calls,plans,total_plan_time,total_exec_time,rows,
                       shared_blks_hit,shared_blks_read,wal_bytes
                FROM pg_stat_statements WHERE userid=(SELECT oid FROM pg_roles WHERE rolname='ding_app')
                ) s""")
            result['statements'] = summarize(rows)
        return result

    def endpoint(self):
        return 'https://'+output(['docker', 'port', self.container, '8443/tcp'])

    def stop(self):
        if self.stopped:
            return
        self.stopped = True
        with (self.root/'stop.log').open('a') as log:
            subprocess.run([sys.executable, str(HERE/'postgres.py'), 'stop', '--root', str(self.root)],
                           env=self.env, stdout=log, stderr=subprocess.STDOUT, check=False)
