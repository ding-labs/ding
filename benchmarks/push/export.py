#!/usr/bin/env python3
"""Export allowlisted measurements; never copy bootstrap, ledger, or log data."""
import argparse
import csv
import json
from pathlib import Path


def measurements(root):
    manifest = json.loads((root / 'manifest.json').read_text())
    for result in json.loads((root / 'summary.json').read_text()):
        case = result['case']
        directory = root / case
        report_path = directory / 'report.json'
        if not report_path.exists():
            raise ValueError('case has no report: ' + case)
        report = json.loads(report_path.read_text())
        execution = json.loads((directory / 'execution.json').read_text())
        config, load = report['Config'], report['Load']
        before, after = report['Before'], report['After']
        planned = config['Rate'] * config['Seconds']
        yield {
            'run': root.name, 'case': case, 'revision': manifest['revision'],
            'environment': manifest['environment'], 'seconds': config['Seconds'],
            'workspaces': config['Workspaces'], 'outputs': config['Outputs'],
            'groups': config['Groups'], 'bodyBytes': config['BodyBytes'],
            'retainedPadding': config['RetainPadding'], 'condition': config['Condition'],
            'receiver': config['Receiver'], 'noop': config['Noop'],
            'offeredRate': config['Rate'], 'planned': planned,
            'offered': load['offered'], 'accepted': load['accepted'],
            'firstAcceptedFraction': load['firstAccepted'] / max(1, planned),
            'acceptedRate': load['accepted'] / max(.001, load['durationSeconds']),
            'observationRate': load['accepted'] * config['Outputs'] / max(.001, load['durationSeconds']),
            'p99MS': load['p99MS'], 'httpP99MS': load['httpP99MS'],
            'scheduleP99MS': load['scheduleP99MS'], 'generatorBusy': load['generatorBusy'],
            'throttles': load.get('throttled', load['statuses'].get('429', 0)),
            'rejected': load['rejected'], 'unconfirmed': load['unconfirmed'],
            'resolved': report['Resolved'], 'reconciliationErrors': len(report['Errors'] or []),
            'readCount': report['ReadCount'], 'readErrors': report['ReadErrors'],
            'readP99MS': report['ReadP99MS'], 'preloadAccepted': report['Preload']['accepted'],
            'beforeObservations': sum(a['Rows']['observations'] for a in before),
            'beforeGroups': sum(a['Rows']['entities'] for a in before),
            'databaseGrowthBytes': sum(a['Health']['databaseBytes'] for a in after)
                - sum(a['Health']['databaseBytes'] for a in before),
            'afterDatabaseBytes': sum(a['Health']['databaseBytes'] for a in after),
            'afterWALBytes': sum(a['Health']['walBytes'] for a in after),
            'pendingDeliveries': sum(a['Rows']['pending'] for a in after),
            'receiverUnique': report['ReceiverUnique'], 'deliveryDrainSeconds': report['DeliveryDrainSeconds'],
            'restartVerified': report['RestartVerified'], 'postChecksVerified': report['PostChecksVerified'],
            'maxSampleHeapBytes': max((s['Heap'] for s in report['Samples']), default=0),
            'serverExitCode': execution['exitCode'], 'generatorExitCode': execution['generatorExitCode'],
            'oomKilled': execution.get('container', {}).get('OOMKilled', False),
            'stable': result['stable'],
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
        writer = csv.DictWriter(f, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)


if __name__ == '__main__':
    main()
