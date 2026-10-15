import copy
import base64
import json
from pathlib import Path
import tempfile
import unittest
from railway_run import qualifies, save_bootstrap


class Qualification(unittest.TestCase):
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
                          'After':[],'PostChecksVerified':True,'RestartVerified':True}}
        self.assertTrue(qualifies(good,config))
        for key,value in [('firstAccepted',299),('scheduleP99MS',21),('clockDiscontinuity',True),('generatorBusy',1),('throttled',1)]:
            bad=copy.deepcopy(good);bad['report']['Load'][key]=value
            self.assertFalse(qualifies(bad,config),key)
        bad=copy.deepcopy(good);bad['report']['RestartVerified']=False
        self.assertFalse(qualifies(bad,config))
