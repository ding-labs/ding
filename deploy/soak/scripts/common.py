"""Private operator state. Tokens never belong in arguments or Git."""
import http.cookiejar
import json
import os
from pathlib import Path
import urllib.request

PRIVATE = Path.home() / '.config/ding-cloud/soak'
ORIGIN = 'https://cloud.ding.ing'

def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, 'w') as f:
        json.dump(value, f, indent=2)
        f.write('\n')

def load(name='active.json'):
    return json.loads((PRIVATE / name).read_text())

class Owner:
    def __init__(self):
        credentials = json.loads((PRIVATE.parent / 'railway/provision.json').read_text())
        jar = http.cookiejar.CookieJar()
        self.client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
        self.csrf = None
        self.call('/v1/auth/password', {'username': credentials['DING_BOOTSTRAP_USERNAME'], 'password': credentials['DING_BOOTSTRAP_PASSWORD']})
        self.csrf = self.call('/v1/browser/session')['data']['csrf']

    def call(self, path, data=None, method=None):
        headers = {'Origin': ORIGIN, 'Content-Type': 'application/json'}
        if self.csrf:
            headers['X-Ding-CSRF'] = self.csrf
        req = urllib.request.Request(ORIGIN + path, headers=headers,
             data=None if data is None else json.dumps(data).encode(), method=method)
        with self.client.open(req, timeout=25) as r:
            return json.load(r)

def observer(path, data=None, method=None):
    cfg = load()
    req = urllib.request.Request(cfg['workerURL'] + path,
        headers={'User-Agent': 'ding-soak-operator/1.0', 'Authorization': 'Bearer ' + load('secrets.json')['admin'], 'Content-Type': 'application/json'},
        data=None if data is None else json.dumps(data).encode(), method=method)
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)
