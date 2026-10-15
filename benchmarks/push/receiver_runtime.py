"""Own a separate loopback-only receiver for local delivery measurements."""
import atexit
import json
import os
import subprocess
import time
from pathlib import Path
from resources import container_user


class LocalReceiver:
    def __init__(self, binary, output, mode, environment, namespace, timeout):
        self.output = output/'receiver'
        self.output.mkdir(mode=0o700)
        self.bootstrap = self.output/'bootstrap.json'
        self.container = 'ding-receiver-'+output.parent.name.lower()+'-'+output.name.lower() if environment=='container' else None
        self.environment = environment
        self.stopped = False
        self.log = (self.output/'process.log').open('x')
        env = {k:v for k,v in os.environ.items() if not k.startswith('DING_SCALE_RECEIVER_')}
        env.update(DING_SCALE_RECEIVER_ONLY='1', DING_SCALE_RECEIVER_OUTPUT=str(self.output),
                   DING_SCALE_RECEIVER_MODE=mode, DING_SCALE_RECEIVER_LISTEN='127.0.0.1:0')
        tail = ['-test.run=^TestLocalScaleReceiver$', '-test.v', '-test.timeout='+str(int(timeout))+'s']
        args = [str(binary)] + tail
        if self.container:
            network = ['--network', 'container:'+namespace] if namespace else ['-p', '127.0.0.1::8443']
            args = ['docker','run','--name',self.container,'--user',container_user(),'--cpus=0.5','--memory=256m','--memory-swap=256m'] + network + [
                '--mount','type=bind,source='+str(binary)+',target=/scale.test,readonly',
                '--mount','type=bind,source='+str(output)+',target=/evidence',
                '-e','DING_SCALE_RECEIVER_ONLY=1','-e','DING_SCALE_RECEIVER_OUTPUT=/evidence/receiver',
                '-e','DING_SCALE_RECEIVER_LISTEN=127.0.0.1:9443','-e','DING_SCALE_RECEIVER_MODE='+mode,
                'debian:bookworm-slim','/scale.test'] + tail
        self.process = subprocess.Popen(args, env=env, stdout=self.log, stderr=subprocess.STDOUT)
        atexit.register(self.stop)
        deadline = time.monotonic()+30
        while True:
            try:
                if json.loads(self.bootstrap.read_text())['Version']==1:
                    break
            except (OSError, ValueError, KeyError):
                pass
            if self.process.poll() is not None or time.monotonic()>deadline:
                raise RuntimeError('local receiver failed to bootstrap; inspect its private process.log')
            time.sleep(.1)
        self.metadata = {'placement':'separate_container' if self.container else 'separate_host_process',
                         'cpuLimit':.5 if self.container else None,
                         'memoryBytes':256*1024**2 if self.container else None,
                         'note':'Receiver resources are outside the app/database budget and sampled separately.'}

    def endpoint(self):
        raw = subprocess.check_output(['docker','port',self.container,'8443/tcp'],text=True).strip()
        return 'https://'+raw

    def sample(self):
        if self.container:
            raw = subprocess.check_output(['docker','stats','--no-stream','--format','{{json .}}',self.container],text=True)
            return json.loads(raw)
        return subprocess.check_output(['ps','-o','rss=,%cpu=','-p',str(self.process.pid)],text=True).strip()

    def stop(self):
        if self.stopped:
            return
        self.stopped = True
        if self.container:
            subprocess.run(['docker','rm','-f',self.container],capture_output=True,check=False)
        elif self.process.poll() is None:
            self.process.terminate()
        try:
            self.process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            self.process.kill();self.process.wait()
        self.log.close()
        self.bootstrap.unlink(missing_ok=True)
