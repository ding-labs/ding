#!/usr/bin/env python3
"""Export allowlisted measurements; never copy bootstrap, ledger, or log data."""
import argparse
import csv
import json
from pathlib import Path

from run import qualified_load


def measurements(root):
    manifest = json.loads((root / 'manifest.json').read_text())
    for result in json.loads((root / 'summary.json').read_text()):
        case = result['case']
        directory = root / case
        report_path = directory / 'report.json'
        execution = json.loads((directory / 'execution.json').read_text())
        if not report_path.exists():
            config = json.loads((directory / 'config.json').read_text())
            yield {
                'run': root.name, 'case': case, 'revision': manifest['revision'],
                'environment': manifest['environment'], 'seconds': config['Seconds'],
                'offeredRate': config['Rate'], 'planned': config['Rate'] * config['Seconds'],
                'profiles': manifest.get('profiles', True), 'reportMissing': True,
                'cancelledByUser': execution.get('cancelledByUser', False),
                'serverExitCode': execution['exitCode'], 'generatorExitCode': execution['generatorExitCode'],
                'oomKilled': execution.get('container', {}).get('OOMKilled', False),
                'failedStatePreserved': bool(execution.get('failedStateArchive') or execution.get('preservedVolume')
                    or execution.get('failedStateDirectory')),
                'stable': False,
            }
            continue
        report = json.loads(report_path.read_text())
        config, load = report['Config'], report['Load']
        before, after = report['Before'], report['After']
        planned = config['Rate'] * config['Seconds']
        backend = config.get('Backend', 'sqlite')
        sqlite = backend == 'sqlite'
        yield {
            'run': root.name, 'case': case, 'revision': manifest['revision'],
            'reportMissing': False, 'backend': backend,
            'environment': manifest['environment'], 'seconds': config['Seconds'],
            'workspaces': config['Workspaces'], 'outputs': config['Outputs'],
            'groups': config['Groups'], 'bodyBytes': config['BodyBytes'],
            'bodyBytesMin': load.get('bodyBytesMin'), 'bodyBytesMax': load.get('bodyBytesMax'),
            'clockDiscontinuity': load.get('clockDiscontinuity'),
            'profiles': manifest.get('profiles', True),
            'retainedPadding': config['RetainPadding'], 'condition': config['Condition'],
            'receiver': config['Receiver'], 'noop': config['Noop'],
            'offeredRate': config['Rate'], 'planned': planned,
            'offered': load['offered'], 'accepted': load['accepted'],
            'firstAcceptedFraction': load['firstAccepted'] / max(1, planned),
            'acceptedRate': load['accepted'] / max(.001, load['durationSeconds']),
            'observationRate': (0 if config['Noop'] else load['accepted'] * config['Outputs'])
                / max(.001, load['durationSeconds']),
            'p99MS': load['p99MS'], 'httpP99MS': load['httpP99MS'],
            'scheduleP99MS': load['scheduleP99MS'], 'generatorBusy': load['generatorBusy'],
            'throttles': load.get('throttled', load['statuses'].get('429', 0)),
            'rejected': load['rejected'], 'unconfirmed': load['unconfirmed'],
            'resolved': report['Resolved'], 'reconciliationErrors': len(report['Errors'] or []),
            'readCount': report['ReadCount'], 'readErrors': report['ReadErrors'],
            'readP99MS': report['ReadP99MS'], 'preloadAccepted': report['Preload']['accepted']
                + sum(r['accepted'] for r in report.get('PreloadFills') or []),
            'beforeObservations': sum(a['Rows']['observations'] for a in before),
            'beforeGroups': sum(a['Rows']['entities'] for a in before),
            'databaseGrowthBytes': sum(a['Health']['databaseBytes'] for a in after)
                - sum(a['Health']['databaseBytes'] for a in before) if sqlite else None,
            'afterDatabaseBytes': sum(a['Health']['databaseBytes'] for a in after) if sqlite else None,
            'afterWALBytes': sum(a['Health']['walBytes'] for a in after) if sqlite else None,
            'quotaBytesAfterPostchecks': sum(a['Rows'].get('liveBytes', 0) for a in after),
            'pendingDeliveries': sum(a['Rows']['pending'] for a in after),
            'receiverUnique': report['ReceiverUnique'], 'deliveryDrainSeconds': report['DeliveryDrainSeconds'],
            # Older diagnostic reports did not record these gates. Keep them
            # unknown instead of inventing a pass or failing the whole export.
            'restartVerified': report.get('RestartVerified'),
            'postChecksVerified': report.get('PostChecksVerified'),
            'maxSampleHeapBytes': max((s['Heap'] for s in report['Samples']), default=0),
            'serverExitCode': execution['exitCode'], 'generatorExitCode': execution['generatorExitCode'],
            'oomKilled': execution.get('container', {}).get('OOMKilled', False),
            'reportedStable': result['stable'],
            'stable': result['stable'] and not manifest.get('profiles', True) and qualified_load(load, planned),
        }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('runs', nargs='+', type=Path)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    rows = [row for run in args.runs for row in measurements(run)]
    if not rows:
        parser.error('no completed cases')
    with args.output.open('x', newline='') as f:
        writer = csv.DictWriter(f, fieldnames=list(dict.fromkeys(key for row in rows for key in row)))
        writer.writeheader()
        writer.writerows(rows)


if __name__ == '__main__':
    main()
