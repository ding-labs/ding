#!/usr/bin/env python3
"""Run only the isolated Go stress lab; no cloud URLs or credentials accepted."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import time

REPO = Path(__file__).resolve().parents[2]


def command(args, **kwargs):
    return subprocess.check_output(args, cwd=REPO, text=True, **kwargs).strip()


def save(path, value):
    with path.open('x') as f:
        json.dump(value, f, indent=2)


def execute(binary, driver, config, output, environment):
    env = dict(os.environ, DING_SCALE_CONFIG=str(output / 'config.json'),
               DING_SCALE_OUTPUT=str(output), DING_SCALE_EXTERNAL=environment)
    timeout = config['Seconds'] + config.get('Preload', 0) / 500 + 900
    container = 'ding-scale-' + output.parent.name.lower() + '-' + output.name.lower()
    volume = container + '-data'
    args = [str(binary), '-test.run=^TestLocalPushScale$', '-test.v', '-test.timeout='+str(int(timeout))+'s']
    if environment == 'container':
        command(['docker', 'volume', 'create', volume])
        args = ['docker', 'run', '--name', container, '--cpus=1', '--memory=1g', '--memory-swap=1g',
                '-p', '127.0.0.1::8443', '--mount', 'type=volume,source='+volume+',target=/lab',
                '--mount', 'type=bind,source='+str(binary)+',target=/scale.test,readonly',
                '--mount', 'type=bind,source='+str(output)+',target=/evidence',
                '-e', 'DING_SCALE_CONFIG=/evidence/config.json', '-e', 'DING_SCALE_OUTPUT=/evidence',
                '-e', 'DING_SCALE_EXTERNAL=container', '-e', 'TMPDIR=/lab', 'debian:bookworm-slim'] + ['/scale.test'] + args[1:]
    started = time.monotonic()
    samples = []
    bootstrap = output/'bootstrap.json'
    generator = None
    code, generator_code, failure = 124, None, None
    with (output/'run.log').open('x') as log, (output/'generator.log').open('x') as generator_log:
        process = subprocess.Popen(args, cwd=REPO, env=env, stdout=log, stderr=subprocess.STDOUT)
        try:
            while process.poll() is None:
                if time.monotonic()-started > timeout+60:
                    raise TimeoutError('local case timed out')
                if generator is None and bootstrap.exists():
                    try:
                        endpoint = json.loads(bootstrap.read_text())['URL']
                    except (json.JSONDecodeError, KeyError):
                        time.sleep(.1)
                        continue
                    if environment == 'container':
                        endpoint = 'https://'+command(['docker','port',container,'8443/tcp'])
                    generator = subprocess.Popen([str(driver),'-bootstrap',str(bootstrap),'-url',endpoint,
                        '-output',str(output),'-seconds',str(config['Seconds']),'-rate',str(config['Rate']),
                        '-workers',str(config.get('Workers',128))],cwd=REPO,stdout=generator_log,stderr=subprocess.STDOUT)
                if generator is not None and generator.poll() not in (None,0):
                    raise RuntimeError('local generator failed; inspect private generator.log')
                if generator is not None:
                    try:
                        value = (command(['docker','stats','--no-stream','--format','{{json .}}',container])
                            if environment == 'container' else command(['ps','-o','rss=,%cpu=','-p',str(process.pid)]))
                        producer = command(['ps','-o','rss=,%cpu=','-p',str(generator.pid)]) if generator.poll() is None else None
                        samples.append({'elapsed':time.monotonic()-started,'server':value,'generator':producer})
                    except subprocess.CalledProcessError:
                        pass
                time.sleep(1 if generator is None else 5)
            code = process.wait()
            generator_code = generator.wait() if generator is not None else None
        except (RuntimeError, TimeoutError) as error:
            failure = str(error)
        finally:
            if process.poll() is None:
                process.kill();process.wait()
            if generator is not None and generator.poll() is None:
                generator.kill();generator.wait()
            bootstrap.unlink(missing_ok=True)
            if environment == 'container':
                subprocess.run(['docker','kill',container],capture_output=True)
    metadata = {'exitCode':code,'generatorExitCode':generator_code,
                'wallSeconds':time.monotonic()-started,'resources':samples,'failure':failure}
    if environment == 'container':
        try:
            metadata['container'] = json.loads(command(['docker','inspect',container]))[0]['State']
        finally:
            subprocess.run(['docker','rm','-f',container],check=False,capture_output=True)
            subprocess.run(['docker','volume','rm',volume],check=False,capture_output=True)
    save(output/'execution.json',metadata)
    report_path = output/'report.json'
    if not report_path.exists():
        return {'case':output.name,'exitCode':code,'stable':False,'reportMissing':True}
    report = json.loads(report_path.read_text())
    load = report['Load']
    fraction = load['firstAccepted']/max(1,config['Rate']*config['Seconds'])
    return {'case':output.name,'exitCode':code,'offeredRate':config['Rate'],
            'acceptedRate':load['accepted']/max(.001,load['durationSeconds']), 'acceptedFraction':fraction,
            'p99MS':load['p99MS'],'httpP99MS':load['httpP99MS'],'scheduleP99MS':load['scheduleP99MS'],
            'generatorBusy':load['generatorBusy'],'statuses':load['statuses'],'resolved':report['Resolved'],
            'reconciliationErrors':report['Errors'],
            'stable':code==0 and generator_code==0 and fraction>=.999 and load['p99MS']<250 and not report['Errors'] and not report['ReadErrors']}


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment', choices=['native','container'], default='native')
    parser.add_argument('--cases', required=True, help='comma-separated names from cases.json')
    parser.add_argument('--seconds', type=int, help='override measured interval')
    parser.add_argument('--repeat', type=int, default=1)
    parser.add_argument('--output', type=Path, default=Path.home()/'.config/ding-cloud/local-scale/runs')
    args = parser.parse_args()
    if not 1 <= args.repeat <= 10 or (args.seconds is not None and not 1 <= args.seconds <= 3600):
        parser.error('repeat or duration outside local bounds')
    cases = {c['Name']:c for c in json.loads((REPO/'benchmarks/push/cases.json').read_text())}
    requested = args.cases.split(',')
    if any(name not in cases for name in requested):
        parser.error('unknown case name')
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    root = args.output.resolve()/(stamp+'-'+args.environment)
    root.mkdir(mode=0o700, parents=True)
    binary = root/'scale.test'
    driver = root/'load'
    subprocess.run(['go','build','-o',str(driver),'./cmd/ding-local-load'],cwd=REPO,check=True)
    build_env = dict(os.environ)
    if args.environment == 'container':
        build_env.update(GOOS='linux',CGO_ENABLED='0')
        command(['docker','pull','debian:bookworm-slim'])
    subprocess.run(['go','test','-c','-o',str(binary),'./internal/cloud'],cwd=REPO,env=build_env,check=True)
    save(root/'manifest.json',{'revision':command(['git','rev-parse','HEAD']),
         'dirty':bool(command(['git','status','--porcelain'])), 'platform':platform.platform(),
         'environment':args.environment,'binarySHA256':hashlib.sha256(binary.read_bytes()).hexdigest(),
         'createdAt':stamp,'cases':requested,'repeat':args.repeat,'secondsOverride':args.seconds,
         'note':'Separate host generator. Only the server is CPU/memory constrained. Container databases use a Docker local volume; ledgers use the host filesystem.'})
    results = []
    print('Evidence: '+str(root),flush=True)
    for name in requested:
        for repeat in range(args.repeat):
            if shutil.disk_usage(root).free < 20<<30:
                raise SystemExit('Host free-disk floor reached; preserve evidence and stop.')
            config = dict(cases[name])
            if args.seconds is not None:
                config['Seconds']=args.seconds
            output = root/(name+'-'+str(repeat+1));output.mkdir(mode=0o700)
            save(output/'config.json',config)
            print('Starting '+output.name,flush=True)
            result = execute(binary,driver,config,output,args.environment)
            results.append(result)
            with (root/'summary.json').open('w') as f:
                json.dump(results,f,indent=2)
            print(json.dumps(result),flush=True)
    print('Completed '+str(len(results))+' cases.',flush=True)


if __name__ == '__main__':
    main()
