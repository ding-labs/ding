"""Private, owned PostgreSQL resources for the local capacity runner."""
import atexit
import json
import os
from pathlib import Path
import subprocess
import sys
from urllib.parse import urlsplit, urlunsplit

HERE = Path(__file__).resolve().parent


def output(args):
    return subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL).strip()


class LocalPostgres:
    def __init__(self, root, environment, units, cpu_share=.5):
        self.root = root
        self.root.mkdir(mode=0o700)
        self.env = dict(os.environ, DING_PG_CPUS=str(units*cpu_share),
                        DING_PG_MEMORY=str(units*512)+'m')
        self.stopped = False
        with (root/'setup.log').open('x') as log:
            subprocess.run([sys.executable, str(HERE/'postgres.py'), 'start', '--root', str(root)],
                           env=self.env, stdout=log, stderr=subprocess.STDOUT, check=True)
        self.manifest = json.loads((root/'lab.json').read_text())
        self.container = self.manifest['container']
        atexit.register(self.stop)
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
        if limits['NanoCpus'] != int(units*cpu_share*1e9) or limits['Memory'] != units*512*1024**2 or limits['MemorySwap'] != limits['Memory']:
            raise RuntimeError('PostgreSQL resource limits do not match the declared budget')
        self.metadata = dict(self.manifest, cpuLimit=units*cpu_share, memoryBytes=limits['Memory'],
                             memorySwapBytes=limits['MemorySwap'], settings=self.sql("""SELECT json_build_object(
                'version',version(),'fsync',current_setting('fsync'),
                'fullPageWrites',current_setting('full_page_writes'),
                'synchronousCommit',current_setting('synchronous_commit'),
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
        return {'runtime': json.loads(output(['docker', 'stats', '--no-stream', '--format', '{{json .}}', self.container])),
                'storage': self.sql("""SELECT json_build_object(
                    'databaseBytes',sum(pg_database_size(oid)),
                    'walBytes',(SELECT sum(size) FROM pg_ls_waldir()),
                    'walPosition',pg_current_wal_lsn()::text,
                    'connections',(SELECT count(*) FROM pg_stat_activity))
                    FROM pg_database WHERE NOT datistemplate""")}

    def endpoint(self):
        return 'https://'+output(['docker', 'port', self.container, '8443/tcp'])

    def stop(self):
        if self.stopped:
            return
        self.stopped = True
        with (self.root/'stop.log').open('a') as log:
            subprocess.run([sys.executable, str(HERE/'postgres.py'), 'stop', '--root', str(self.root)],
                           env=self.env, stdout=log, stderr=subprocess.STDOUT, check=False)
