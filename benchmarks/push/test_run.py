"""Regression for evidence preservation when the local database observer fails."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

from run import execute


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
