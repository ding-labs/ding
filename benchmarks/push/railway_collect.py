"""Run one private Railway case over SSH and preserve its evidence locally."""
import argparse
import datetime
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile


def run():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--label', required=True)
    parser.add_argument('--case', required=True)
    parser.add_argument('--rate', type=int, required=True)
    parser.add_argument('--seconds', type=int, required=True)
    parser.add_argument('--pool', type=int, choices=(2, 4, 8, 16, 32), default=4)
    parser.add_argument('--seed', type=int, default=0)
    parser.add_argument('--workspaces', type=int)
    args = parser.parse_args()
    if not re.fullmatch('[a-z0-9-]{1,80}', args.label):
        parser.error('label must be a short lowercase identifier')
    os.umask(0o077)
    manifest = json.loads(args.manifest.read_text())
    root = args.manifest.parent/'cases'/args.label
    root.mkdir(parents=True, mode=0o700)
    ssh_base = ['npx', '--yes', '@railway/cli@5.68.0', 'ssh', '-p', manifest['project'],
                '-e', manifest['environment'], '-s']
    ssh = ssh_base+[manifest['generator'], '--']
    metadata = {'startedAt': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                'manifest': manifest, 'arguments': {k:str(v) for k,v in vars(args).items()}}
    with (root/'launcher.stdout').open('xb') as output, (root/'launcher.stderr').open('xb') as errors:
        process = subprocess.run(ssh+['python3', '/opt/lab/railway_run.py', '--case', args.case,
                                     '--rate', str(args.rate), '--seconds', str(args.seconds), '--pool', str(args.pool)]
                                 + (['--seed', str(args.seed)] if args.seed else [])
                                 + (['--workspaces', str(args.workspaces)] if args.workspaces is not None else []),
                                 stdout=output, stderr=errors, timeout=args.seconds+2400)
    metadata.update(endedAt=datetime.datetime.now(datetime.timezone.utc).isoformat(), exitCode=process.returncode)
    (root/'execution.json').write_text(json.dumps(metadata, indent=2))
    summaries = [json.loads(line) for line in (root/'launcher.stdout').read_text().splitlines() if line.startswith('{')]
    if not summaries:
        print(json.dumps({'label':args.label, 'error':'No result; inspect private launcher logs', 'exitCode':process.returncode}), flush=True)
        return 2
    summary = summaries[-1]
    identity = summary['id']
    if not re.fullmatch('[a-f0-9]{32}', identity):
        raise RuntimeError('invalid remote evidence identity')
    remote = '/data/ding-lab/runs/'+identity
    names = ('result.json', 'summary.json', 'config.json', 'requests.jsonl')
    archive = root/'evidence.tar.gz'
    with archive.open('xb') as output:
        subprocess.run(ssh+['tar', '-czf', '-', '-C', remote, *names],
                       stdout=output, stderr=subprocess.PIPE, check=True, timeout=180)
    with tarfile.open(archive) as evidence:
        if set(evidence.getnames()) != set(names) or any(not member.isfile() for member in evidence.getmembers()):
            raise RuntimeError('unexpected evidence archive members')
        # Only fixed JSON filenames are materialized; the full ledger stays in
        # the compressed archive and no archive-controlled paths are extracted.
        for name in names[:3]:
            (root/name).write_bytes(evidence.extractfile(name).read())
    config = json.loads((root/'config.json').read_text())
    result = json.loads((root/'result.json').read_text())
    if config.get('Receiver') and result.get('report'):
        # Delivery evidence is written by the app-side receiver, whereas the
        # request journal lives on the generator. Preserve both before cleanup.
        receiver_archive = root/'receiver.tar.gz'
        with receiver_archive.open('xb') as output:
            subprocess.run(ssh_base+[manifest['app'], '--', 'tar', '-czf', '-', '-C', remote, 'receiver.jsonl'],
                           stdout=output, stderr=subprocess.PIPE, check=True, timeout=180)
        with tarfile.open(receiver_archive) as evidence:
            if evidence.getnames() != ['receiver.jsonl'] or not evidence.getmembers()[0].isfile():
                raise RuntimeError('unexpected receiver evidence archive')
    load = summary['load']
    print(json.dumps({'label':args.label, 'id':identity, 'pass':summary['pass'],
                      'rate':args.rate, 'seconds':args.seconds, 'accepted':load.get('firstAccepted'),
                      'ackP99MS':load.get('p99MS'), 'scheduleP99MS':load.get('scheduleP99MS'),
                      'throttled':load.get('throttled'), 'errors':(summary.get('errors') or [])[:5],
                      'errorCount':len(summary.get('errors') or []),
                      'artifact':str(root/'result.json')}), flush=True)
    return process.returncode


if __name__ == '__main__':
    raise SystemExit(run())
