"""Load qualification and evidence preservation regressions."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

from run import execute, qualified_load


class LoadQualification(unittest.TestCase):
    def test_every_planned_arrival_must_be_accepted_even_without_a_reported_error(self):
        load = {'generatorBusy': 0, 'unconfirmed': 0, 'rejected': 0, 'firstAccepted': 29999}
        self.assertFalse(qualified_load(load, 30000))
        load['firstAccepted'] = 30000
        self.assertTrue(qualified_load(load, 30000))

    def test_late_generator_cannot_qualify_fixed_arrival_load(self):
        load = {'generatorBusy': 0, 'unconfirmed': 0, 'rejected': 0, 'scheduleP99MS': 21}
        self.assertFalse(qualified_load(load))
        load['scheduleP99MS'] = 20
        self.assertTrue(qualified_load(load))

    def test_small_throttled_fraction_still_fails_the_zero_rejection_gate(self):
        load = {'generatorBusy': 0, 'unconfirmed': 0, 'rejected': 0,
                'firstAccepted': 29977, 'scheduled': 30000, 'throttled': 23,
                'statuses': {'202': 29977, '429': 23}}
        self.assertGreaterEqual(load['firstAccepted']/load['scheduled'], .999)
        self.assertFalse(qualified_load(load))
        load['throttled'] = 0
        self.assertFalse(qualified_load(load))
        load['statuses'] = {'202': 30000}
        self.assertTrue(qualified_load(load))


class LauncherEvidence(unittest.TestCase):
    def test_failed_database_observer_preserves_execution_evidence(self):
        class UnavailableDatabase:
            connections = Path('/unused-private-fixture.json')

            def sample(self):
                raise subprocess.CalledProcessError(1, ['docker', 'stats'])

        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            result = execute(Path('/usr/bin/false'), Path('/usr/bin/false'),
                             {'Rate': 1, 'Seconds': 1}, output, 'native',
                             pg=UnavailableDatabase())
            evidence = json.loads((output/'execution.json').read_text())
            self.assertFalse(result['stable'])
            self.assertNotEqual(result['exitCode'], 0)
            self.assertTrue(result['reportMissing'])
            self.assertEqual(evidence['databaseStart'], {'unavailable': True})
            self.assertEqual(evidence['databaseEnd'], {'unavailable': True})
            self.assertTrue(evidence['failure'])
            self.assertTrue((output/'run.log').exists())
