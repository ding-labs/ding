import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('update', Path(__file__).with_name('package-chatgpt-update.py'))
update = importlib.util.module_from_spec(spec)
spec.loader.exec_module(update)


class PrivateUpdateTest(unittest.TestCase):
    def test_keeps_connection_and_ships_only_skills_assets(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            old, new, out = (root / name for name in ('old.zip', 'new.zip', 'out.zip'))
            binding = {'apps': {'dev-example': {'id': 'asdk_app_example'}}}
            with zipfile.ZipFile(old, 'w') as z:
                z.writestr('.codex-plugin/plugin.json', json.dumps({'name': 'dev-example', 'apps': './.app.json', 'interface': {'displayName': 'Existing Ding'}}))
                z.writestr('.app.json', json.dumps(binding))
            with zipfile.ZipFile(new, 'w') as z:
                z.writestr('plugin.json', json.dumps({'version': '0.2.0', 'description': 'Ding', 'extensions': {'com.openai': {'publication': {'release_notes': 'PRIVATE QUALIFICATION ONLY'}, 'interface': {'displayName': 'New Ding'}}}}))
                z.writestr('mcp.json', '{"mustNotCreateNewConnection":true}')
                z.writestr('skills/ding-setup/SKILL.md', 'setup')
                z.writestr('skills/ding-watch/SKILL.md', 'watch')
                z.writestr('assets/icon.svg', '<svg/>')
            self.assertEqual(len(update.build(old, new, out)), 64)
            with zipfile.ZipFile(out) as z:
                self.assertEqual(json.loads(z.read('.app.json')), binding)
                manifest = json.loads(z.read('.codex-plugin/plugin.json'))
                self.assertEqual(manifest['name'], 'dev-example')
                self.assertEqual(manifest['interface']['displayName'], 'Existing Ding')
                self.assertEqual(manifest['skills'], './skills/')
                self.assertNotIn('mcp.json', z.namelist())
                self.assertEqual(z.read('skills/ding-watch/SKILL.md'), b'watch')
            with self.assertRaises(FileExistsError):
                update.build(old, new, out)
            with zipfile.ZipFile(old, 'w') as z:
                z.writestr('.codex-plugin/plugin.json', json.dumps({'name': 'dev-example', 'apps': './.app.json', 'interface': {'displayName': 'Existing Ding'}}))
                z.writestr('.app.json', json.dumps({'apps': {'another': {'id': 'asdk_app_other'}}}))
            with self.assertRaises(ValueError):
                update.build(old, new, root / 'bad.zip')
            self.assertFalse((root / 'bad.zip').exists())
