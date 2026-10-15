"""Summarize preserved Railway cases using only their measured time windows."""
import argparse
import datetime
import json
import re
from pathlib import Path


def cpu_window(samples):
    if len(samples) < 2:
        return None
    first, last = samples[0], samples[-1]
    seconds = last['at']-first['at']
    if seconds <= 0:
        return None
    delta = {key:int(last['cpu'][key])-int(first['cpu'][key]) for key in first['cpu']}
    return {'cores':round(delta['usage_usec']/seconds/1e6,3),
            'throttledPeriodsPercent':round(100*delta['nr_throttled']/max(1,delta['nr_periods']),2),
            'peakMemoryMB':round(max(int(s['memoryBytes']) for s in samples)/1e6,1),
            'cpuMax':first['cpuMax'].strip(), 'memoryMax':int(first['memoryMax']), 'sampleCount':len(samples)}


def summarize(path):
    result = json.loads(path.read_text())
    summary = json.loads((path.parent/'summary.json').read_text())
    report = result['report']; load = report['Load']
    stamp = re.sub(r'(\.\d{6})\d+', r'\1', report['StartedAt']).replace('Z','+00:00')
    start = datetime.datetime.fromisoformat(stamp).timestamp()
    end = start+load['durationSeconds']
    app = [s for s in result['resources'] if start <= s['at'] <= end and 'cpu' in s]
    generator = [s for s in result['generatorResources'] if start <= s['at'] <= end]
    database = []
    for s in app:
        d = s.get('database',{})
        if 'cpuStat' in d:
            database.append({'at':s['at'], 'cpu':dict(line.split() for line in d['cpuStat'].splitlines()),
                             'memoryBytes':d['memoryBytes'], 'cpuMax':d['cpuMax'], 'memoryMax':d['memoryMax']})
    return {'label':path.parent.name, 'pass':summary['pass'], 'case':summary['case'],
            'rate':summary['rate'], 'seconds':summary['seconds'], 'pool':summary.get('pool',4),
            'seed':load.get('shuffleSeed',0),
            'accepted':load['firstAccepted'], 'ackP99MS':load['p99MS'],
            'scheduleP99MS':load['scheduleP99MS'], 'httpP99MS':load['httpP99MS'],
            'readP99MS':report['ReadP99MS'], 'readErrors':report['ReadErrors'],
            'throttled':load['throttled'], 'unconfirmed':load['unconfirmed'],
            'observationsPerSecond':round(load['acceptedObservations']/summary['seconds'],2),
            'inputMBPerSecond':round(load['acceptedBytes']/summary['seconds']/1e6,3),
            'payloads':load['payloads'], 'app':cpu_window(app), 'database':cpu_window(database),
            'generator':cpu_window(generator), 'errors':report['Errors']}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root',type=Path)
    args = parser.parse_args()
    for path in sorted(args.root.glob('*/result.json')):
        print(json.dumps(summarize(path)))
