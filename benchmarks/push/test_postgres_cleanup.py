import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import postgres


class OwnedCleanup(unittest.TestCase):
    def test_refuses_an_unrelated_project_before_any_docker_command(self):
        with tempfile.TemporaryDirectory() as directory:
            (Path(directory)/'lab.json').write_text(json.dumps({'project':'another-project'}))
            with patch('sys.argv', ['postgres.py','destroy','--root',directory,'--discard-data']), \
                 patch('postgres.subprocess.run') as run, patch('sys.stderr'):
                with self.assertRaises(SystemExit):
                    postgres.main()
                run.assert_not_called()

    def test_removes_only_generated_credentials_after_owned_destruction(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root/'lab.json').write_text(json.dumps({'project':'ding-pg-0123456789'}))
            (root/'connections.json').write_text('private')
            (root/'report.json').write_text('preserve')
            with patch('sys.argv', ['postgres.py','destroy','--root',directory,'--discard-data']), \
                 patch('postgres.subprocess.run') as run:
                postgres.main()
                self.assertEqual(run.call_args.args[0][-2:], ['down','--volumes'])
            self.assertFalse((root/'connections.json').exists())
            self.assertEqual((root/'report.json').read_text(), 'preserve')
