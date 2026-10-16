import copy
import base64
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from railway_app import selected_binary
from railway_run import qualifies, save_bootstrap


class Qualification(unittest.TestCase):
    def test_comparison_cannot_select_arbitrary_executables(self):
        for request in ({'variant':'/bin/sh'}, {'backend':'mysql'}):
            with self.assertRaises(ValueError):
                selected_binary(request)
        with tempfile.TemporaryDirectory() as directory, patch.dict('os.environ', {
                'RAILWAY_GIT_COMMIT_SHA':'candidate', 'RAILWAY_DEPLOYMENT_ID':'same-deployment'}):
            root = Path(directory)
            (root/'baseline.test').write_bytes(b'before')
            (root/'cloud.test').write_bytes(b'after')
            (root/'baseline-revision').write_text('baseline')
            before, backend, old = selected_binary({'variant':'before','backend':'sqlite'},root)
            after, _, new = selected_binary({'variant':'after'},root)
            self.assertEqual((before.name,after.name,backend),('baseline.test','cloud.test','sqlite'))
            self.assertEqual((old['revision'],new['revision']),('baseline','candidate'))
            self.assertNotEqual(old['sha256'],new['sha256'])
            self.assertEqual(old['deployment'],new['deployment'])

    def test_controller_handoff_preserves_exact_request_bytes(self):
        original = b'{"Producers":[{"body":{ "value" : "<tag>", "array":[1,2] }}]}\n'
        envelope = json.dumps({'bootstrap':base64.b64encode(original).decode()}, indent=2)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            save_bootstrap(root,json.loads(envelope)['bootstrap'])
            self.assertEqual((root/'bootstrap.json').read_bytes(),original)

    def test_strict_measurement_and_restart_gates(self):
        config = {'Rate':10,'Seconds':30,'Restart':True}
        good = {'exitCode':0,'generatorExitCode':0,'resources':[{}],'generatorResources':[{}],
                'report':{'Load':{'firstAccepted':300,'scheduleP99MS':2,'p99MS':20},'ReadErrors':0,
                          'Samples':[{'Metrics':'ding_cloud_ingest_seconds_count 300\n'}],
                          'After':[],'PostChecksVerified':True,'RestartVerified':True}}
        self.assertTrue(qualifies(good,config))
        for key,value in [('firstAccepted',299),('scheduleP99MS',21),('clockDiscontinuity',True),('generatorBusy',1),('throttled',1)]:
            bad=copy.deepcopy(good);bad['report']['Load'][key]=value
            self.assertFalse(qualifies(bad,config),key)
        bad=copy.deepcopy(good);bad['report']['RestartVerified']=False
        self.assertFalse(qualifies(bad,config))
        for samples in ([],[{'Metrics':'metrics collection unavailable\n'}]):
            bad=copy.deepcopy(good);bad['report']['Samples']=samples
            self.assertFalse(qualifies(bad,config))
