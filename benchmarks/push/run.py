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
import signal
import subprocess
import time

from postgres_runtime import LocalPostgres
from receiver_runtime import LocalReceiver

REPO = Path(__file__).resolve().parents[2]


def command(args, **kwargs):
    return subprocess.check_output(args, cwd=REPO, text=True, **kwargs).strip()


def save(path, value):
    with path.open('x') as f:
        json.dump(value, f, indent=2)


def qualified_load(load):
    return (load['generatorBusy'] == 0 and load['unconfirmed'] == 0 and load['rejected'] == 0
            and not load.get('clockDiscontinuity', False))


def capture_failed_state(volume, output):
    archive = output / 'failed-state.tar.gz'
    command(['docker', 'run', '--rm',
             '--mount', 'type=volume,source='+volume+',target=/lab,readonly',
             '--mount', 'type=bind,source='+str(output)+',target=/evidence',
             'debian:bookworm-slim', 'tar', '-C', '/lab', '-czf', '/evidence/'+archive.name, '.'])
    archive.chmod(0o600)
    return archive.name


def sample_database(pg):
    if pg is None:
        return None
    try:
        return pg.sample()
    except (subprocess.CalledProcessError, OSError, json.JSONDecodeError):
        return {'unavailable': True}


def execute(binary, driver, config, output, environment, pg=None, units=1, generator_placement="host"):
    if generator_placement not in ("host", "container") or (generator_placement == "container" and environment != "container"):
        raise ValueError("container generator requires the Linux container environment")
    env = dict(os.environ, DING_SCALE_CONFIG=str(output / 'config.json'),
               DING_SCALE_OUTPUT=str(output), DING_SCALE_EXTERNAL=environment)
    fixture_tmp = output/'fixture-tmp'
    fixture_tmp.mkdir(mode=0o700)
    env['TMPDIR'] = str(fixture_tmp)
    if pg:
        env['DING_TEST_POSTGRES_FILE'] = str(pg.connections)
    database_start = sample_database(pg)
    timeout = config['Seconds'] + config.get('Preload', 0) / 100 + 900
    receiver = LocalReceiver(binary,output,config['Receiver'],environment,
                             pg.container if pg and environment=='container' else None,timeout) if config.get('ReceiverProcess') and config.get('Receiver') else None
    if receiver:
        env['DING_SCALE_RECEIVER_FILE'] = str(receiver.bootstrap)
    container = 'ding-scale-' + output.parent.name.lower() + '-' + output.name.lower()
    volume = container + '-data'
    args = [str(binary), '-test.run=^TestLocalPushScale$', '-test.v', '-test.timeout='+str(int(timeout))+'s']
    if environment == 'container':
        command(['docker', 'volume', 'create', volume])
        app_cpus = units/2 if pg else units
        app_memory = str(units*512 if pg else units*1024)+'m'
        network = ['--network', 'container:'+pg.container] if pg else (['--network','container:'+receiver.container] if receiver else ['-p','127.0.0.1::8443'])
        fixture = ['--mount', 'type=bind,source='+str(pg.connections)+',target=/postgres-fixture.json,readonly',
                   '-e', 'DING_TEST_POSTGRES_FILE=/postgres-fixture.json'] if pg else []
        if receiver:
            fixture += ['-e','DING_SCALE_RECEIVER_FILE=/evidence/receiver/bootstrap.json']
        args = ['docker', 'run', '--name', container, '--cpus='+str(app_cpus), '--memory='+app_memory, '--memory-swap='+app_memory] + network + fixture + [ '--mount', 'type=volume,source='+volume+',target=/lab',
                '--mount', 'type=bind,source='+str(binary)+',target=/scale.test,readonly',
                '--mount', 'type=bind,source='+str(output)+',target=/evidence',
                '-e', 'DING_SCALE_CONFIG=/evidence/config.json', '-e', 'DING_SCALE_OUTPUT=/evidence',
                '-e', 'DING_SCALE_EXTERNAL=container', '-e', 'TMPDIR=/lab', 'debian:bookworm-slim'] + ['/scale.test'] + args[1:]
    started = time.monotonic()
    samples = []
    bootstrap = output/'bootstrap.json'
    generator = None
    generator_container = None
    generator_metadata = {"placement": generator_placement}
    code, generator_code, failure = 124, None, None
    observer_errors = 0
    cancelled = False
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
                        endpoint = ('https://127.0.0.1:8443' if generator_placement == 'container' else
                                    pg.endpoint() if pg else (receiver.endpoint() if receiver else 'https://'+command(['docker','port',container,'8443/tcp'])))
                    generator_args = [str(driver),'-bootstrap',str(bootstrap),'-url',endpoint,
                        '-output',str(output),'-seconds',str(config['Seconds']),'-rate',str(config['Rate']),
                        '-workers',str(config.get('Workers',512))]
                    if generator_placement == 'container':
                        generator_container = container+'-generator'
                        namespace = pg.container if pg else (receiver.container if receiver else container)
                        generator_args = ['docker','run','--name',generator_container,'--cpus=2',
                            '--memory=512m','--memory-swap=512m','--network','container:'+namespace,
                            '--mount','type=bind,source='+str(driver)+',target=/load,readonly',
                            '--mount','type=bind,source='+str(output)+',target=/evidence',
                            'debian:bookworm-slim','/load','-bootstrap','/evidence/bootstrap.json',
                            '-url',endpoint,'-output','/evidence']+generator_args[7:]
                        generator_metadata.update(cpuLimit=2,memoryBytes=512*1024**2,
                            note='Separate load producer outside app/database resource budget; Linux loopback bypasses Desktop port forwarding.')
                    generator = subprocess.Popen(generator_args,cwd=REPO,stdout=generator_log,stderr=subprocess.STDOUT)
                if generator is not None and generator.poll() not in (None,0):
                    raise RuntimeError('local generator failed; inspect private generator.log')
                if generator is not None:
                    try:
                        value = (command(['docker','stats','--no-stream','--format','{{json .}}',container])
                            if environment == 'container' else command(['ps','-o','rss=,%cpu=','-p',str(process.pid)]))
                        producer = (command(['docker','stats','--no-stream','--format','{{json .}}',generator_container]) if generator_container else
                                    command(['ps','-o','rss=,%cpu=','-p',str(generator.pid)])) if generator.poll() is None else None
                        samples.append({'at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'elapsed':time.monotonic()-started,'server':value,'generator':producer,
                                        'database':pg.sample() if pg else None,
                                        'receiver':receiver.sample() if receiver else None})
                    except (subprocess.CalledProcessError, OSError, json.JSONDecodeError):
                        if process.poll() is None and generator.poll() is None:
                            observer_errors += 1
                time.sleep(1 if generator is None else 5)
            code = process.wait()
            if code != 0 and generator is not None and generator.poll() is None:
                generator.kill()
            generator_code = generator.wait() if generator is not None else None
        except (subprocess.CalledProcessError, OSError):
            failure = 'Local test infrastructure became unavailable.'
        except (RuntimeError, TimeoutError) as error:
            failure = str(error)
        except KeyboardInterrupt:
            cancelled, code, failure = True, 130, 'Local case cancelled by operator.'
        finally:
            if process.poll() is None:
                process.kill();process.wait()
            if generator is not None and generator.poll() is None:
                generator.kill();generator.wait()
            if generator is not None:
                generator_code = generator.returncode
            bootstrap.unlink(missing_ok=True)
            if environment == 'container':
                subprocess.run(['docker','kill',container],capture_output=True)
    if generator_container:
        try:
            inspected = json.loads(command(['docker','inspect',generator_container]))[0]
            limits = inspected['HostConfig']
            generator_metadata['state'] = inspected['State']
            generator_metadata['limitsVerified'] = (limits['NanoCpus'] == 2*10**9 and
                limits['Memory'] == 512*1024**2 and limits['MemorySwap'] == limits['Memory'])
            if not generator_metadata['limitsVerified']:
                failure = failure or 'Local generator resource limits do not match the manifest.'
        except (subprocess.CalledProcessError, OSError, json.JSONDecodeError):
            failure = failure or 'Local generator inspection became unavailable.'
        subprocess.run(['docker','rm','-f',generator_container],capture_output=True,check=False)
    database_end = sample_database(pg)
    if any(snapshot and snapshot.get('unavailable') for snapshot in (database_start, database_end)):
        failure = failure or 'Local database observation became unavailable.'
        if code == 0:
            code = 125
    metadata = {'exitCode':code,'generatorExitCode':generator_code,
                'wallSeconds':time.monotonic()-started,'resources':samples,'failure':failure,
                'cancelledByUser':cancelled, 'generator':generator_metadata, 'receiver':receiver.metadata if receiver else {'placement':'cohosted' if config.get('Receiver') else 'none'},
                'databaseStart':database_start,
                'databaseEnd':database_end, 'observerErrors':observer_errors, 'serverExitCode':process.returncode,
                'appCPULimit':(units/2 if pg else units) if environment=='container' else None,
                'appMemoryBytes':(units*512 if pg else units*1024)*1024**2 if environment=='container' else None}
    if environment == 'native' and (code != 0 or failure):
        for directory in [output/'failed-state', fixture_tmp]:
            if any(directory.rglob('ding.db')):
                metadata['failedStateDirectory'] = directory.name
                break
    if environment == 'container':
        remove_volume = True
        try:
            metadata['container'] = json.loads(command(['docker','inspect',container]))[0]['State']
        except (subprocess.CalledProcessError, json.JSONDecodeError):
            metadata['container'] = {'inspectionUnavailable': True}
        if code != 0 or generator_code != 0 or failure or not (output/'report.json').exists() or metadata['container'].get('inspectionUnavailable'):
            try:
                metadata['failedStateArchive'] = capture_failed_state(volume, output)
            except (subprocess.CalledProcessError, OSError):
                remove_volume = False
                metadata['preservedVolume'] = volume
                metadata['failedStateCapture'] = 'failed; owned volume retained'
        subprocess.run(['docker','rm','-f',container],check=False,capture_output=True)
        if remove_volume:
            subprocess.run(['docker','volume','rm',volume],check=False,capture_output=True)
    if receiver:
        receiver.stop()
    save(output/'execution.json',metadata)
    report_path = output/'report.json'
    if cancelled or not report_path.exists():
        if code == 0:
            code = 125
        return {'case':output.name,'exitCode':code,'stable':False,
                'reportMissing':not report_path.exists(),'cancelledByUser':cancelled}
    report = json.loads(report_path.read_text())
    load = report['Load']
    fraction = load['firstAccepted']/max(1,config['Rate']*config['Seconds'])
    pending = sum(a['Rows']['pending'] for a in report['After'])
    postchecks = report.get('PostChecksVerified',False) or config.get('Noop',False)
    return {'case':output.name,'exitCode':code,'offeredRate':config['Rate'],
            'acceptedRate':load['accepted']/max(.001,load['durationSeconds']), 'acceptedFraction':fraction,
            'p99MS':load['p99MS'],'httpP99MS':load['httpP99MS'],'scheduleP99MS':load['scheduleP99MS'],
            'generatorBusy':load['generatorBusy'],'statuses':load['statuses'],'resolved':report['Resolved'],
            'reconciliationErrors':report['Errors'],'readP99MS':report['ReadP99MS'],
            'pendingDeliveries':pending,'postChecksVerified':postchecks,
            'inconclusive': ('producer clock discontinuity' if load.get('clockDiscontinuity') else
                             'server clock discontinuity' if report.get('InternalObservations',0) else
                             'local resource observation unavailable' if observer_errors or failure else None),
            'stable':code==0 and generator_code==0 and not failure and not observer_errors and qualified_load(load) and fraction>=.999 and load['p99MS']<250 and not report['Errors'] and not report['ReadErrors'] and not report.get('InternalObservations',0) and pending==0 and postchecks}


def main():
    os.umask(0o077)
    def cancel_run(_signal, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, cancel_run)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment', choices=['native','container'], default='native')
    parser.add_argument('--generator-placement', choices=['host','container'], default='host',
                        help='separate load producer location; container bypasses Desktop port forwarding')
    parser.add_argument('--receiver-process', action='store_true', help='use a separately resourced local receiver for delivery cases')
    parser.add_argument('--backend', choices=['sqlite','postgres'], default='sqlite')
    parser.add_argument('--resource-units', type=int, choices=[1,2,4], default=1,
                        help='total CPU cores and GiB; PostgreSQL splits app/database equally')
    parser.add_argument('--pool-limit', type=int, choices=[1,2,4,8,16], default=8)
    parser.add_argument('--credential-readers', type=int, choices=[1,2,4,8], default=4)
    parser.add_argument('--rate', type=int, help='override planned requests per second')
    parser.add_argument('--cases', required=True, help='comma-separated names from cases.json')
    parser.add_argument('--seconds', type=int, help='override measured interval')
    parser.add_argument('--repeat', type=int, default=1)
    parser.add_argument('--profiles', action='store_true', help='collect diagnostic profiles; off for capacity runs')
    parser.add_argument('--stop-file', type=Path, help='finish the active case and defer queued cases when this file exists')
    parser.add_argument('--output', type=Path, default=Path.home()/'.config/ding-cloud/local-scale/runs')
    args = parser.parse_args()
    if args.generator_placement == "container" and args.environment != "container":
        parser.error("container generator requires --environment container")
    if args.rate is not None and not 1 <= args.rate <= 100000:
        parser.error('rate outside local bounds')
    if not 1 <= args.repeat <= 10 or (args.seconds is not None and not 1 <= args.seconds <= 3600):
        parser.error('repeat or duration outside local bounds')
    cases = {c['Name']:c for c in json.loads((REPO/'benchmarks/push/cases.json').read_text())}
    requested = args.cases.split(',')
    if any(name not in cases for name in requested):
        parser.error('unknown case name')
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    root = args.output.resolve()/(stamp+'-'+args.environment+'-'+args.backend)
    root.mkdir(mode=0o700, parents=True)
    binary = root/'scale.test'
    driver = root/'load'
    build_env = dict(os.environ)
    if args.environment == 'container':
        build_env.update(GOOS='linux',CGO_ENABLED='0')
        command(['docker','pull','debian:bookworm-slim'])
    subprocess.run(['go','build','-o',str(driver),'./cmd/ding-local-load'],cwd=REPO,
                   env=build_env if args.generator_placement=='container' else None,check=True)
    subprocess.run(['go','test','-c','-o',str(binary),'./internal/cloud'],cwd=REPO,env=build_env,check=True)
    pg = LocalPostgres(root/'postgres-lab', args.environment, args.resource_units) if args.backend=='postgres' else None
    save(root/'manifest.json',{'backend':args.backend,'poolLimit':args.pool_limit,
         'credentialReaders':args.credential_readers,'resourceUnits':args.resource_units,
         'receiverProcess':args.receiver_process,'generatorPlacement':args.generator_placement,
         'driverSHA256':hashlib.sha256(driver.read_bytes()).hexdigest(),
         'runnerImage':command(['docker','image','inspect','--format','{{.Id}}','debian:bookworm-slim']) if args.environment=='container' else None,
         'totalCPULimit':args.resource_units if args.environment=='container' else None,
         'totalMemoryBytes':args.resource_units*1024**3 if args.environment=='container' else None,
         'postgres':pg.metadata if pg else None,
         'pgx':command(['go','list','-m','github.com/jackc/pgx/v5']),
         'revision':command(['git','rev-parse','HEAD']),
         'dirty':bool(command(['git','status','--porcelain'])), 'platform':platform.platform(),
         'environment':args.environment,'binarySHA256':hashlib.sha256(binary.read_bytes()).hexdigest(),
         'createdAt':stamp,'cases':requested,'repeat':args.repeat,'secondsOverride':args.seconds,
         'profiles':args.profiles,
         'note':'Separate recorded generator. Linux app and database share the declared total CPU/RAM budget with no swap. Native exploration has an unlimited app. Owned Docker volumes preserve database files; ledgers use the host filesystem.'})
    results = []
    stop_file = args.stop_file.resolve() if args.stop_file else root/'STOP'
    print('Evidence: '+str(root),flush=True)
    print('Stop after current case: create '+str(stop_file),flush=True)
    for name in requested:
        for repeat in range(args.repeat):
            if stop_file.exists():
                break
            if shutil.disk_usage(root).free < 20<<30:
                raise SystemExit('Host free-disk floor reached; preserve evidence and stop.')
            config = dict(cases[name])
            config['Profiles'] = args.profiles
            config.update(Backend=args.backend, PoolLimit=args.pool_limit, CredentialReaders=args.credential_readers, ReceiverProcess=args.receiver_process)
            if args.rate is not None:
                config['Rate']=args.rate
            if args.seconds is not None:
                config['Seconds']=args.seconds
            output = root/(name+'-'+str(repeat+1));output.mkdir(mode=0o700)
            save(output/'config.json',config)
            print('Starting '+output.name,flush=True)
            result = execute(binary,driver,config,output,args.environment,pg,args.resource_units,args.generator_placement)
            results.append(result)
            with (root/'summary.json').open('w') as f:
                json.dump(results,f,indent=2)
            print(json.dumps(result),flush=True)
            if result.get('cancelledByUser') or result['exitCode'] != 0:
                stop_file.touch(mode=0o600)
        if stop_file.exists():
            break
    completed = {r['case'] for r in results}
    deferred = [name+'-'+str(i+1) for name in requested for i in range(args.repeat)
                if name+'-'+str(i+1) not in completed]
    cancelled = [r['case'] for r in results if r.get('cancelledByUser')]
    failed = [r['case'] for r in results if r['exitCode'] != 0 and not r.get('cancelledByUser')]
    save(root/'run-state.json', {'state':'cancelled' if cancelled else ('failed' if failed else ('deferred' if deferred else 'complete')),
         'failed':failed,
         'completed':[r['case'] for r in results if not r.get('cancelledByUser')],
         'cancelled':cancelled, 'deferred':deferred})
    if pg:
        pg.stop()
    print('Completed '+str(len(results))+' cases.',flush=True)
    if cancelled:
        raise SystemExit(130)
    if any(result['exitCode'] != 0 for result in results):
        raise SystemExit(1)


if __name__ == '__main__':
    main()
