#!/usr/bin/env python3
"""Add qualified cloud skills to an exported private ChatGPT app plugin.

Keeps the host-assigned plugin/app identity and existing OAuth connection. This
is a private compatibility artifact, never a public marketplace submission.
"""
import argparse
import hashlib
import json
from pathlib import Path
import zipfile


def build(existing: Path, journey: Path, output: Path):
    with zipfile.ZipFile(existing) as old, zipfile.ZipFile(journey) as new:
        manifest = json.loads(old.read('.codex-plugin/plugin.json'))
        apps = json.loads(old.read('.app.json'))
        portable = json.loads(new.read('plugin.json'))
        publication = portable['extensions']['com.openai']['publication']
        if 'PRIVATE QUALIFICATION ONLY' not in publication['release_notes']:
            raise ValueError('Use a cloud-chatgpt -private qualification archive')
        name = manifest['name']
        bindings = apps.get('apps', {})
        if manifest.get('apps') != './.app.json' or list(bindings) != [name]:
            raise ValueError('Expected one existing app binding matching the plugin name')
        if set(bindings[name]) != {'id'} or not bindings[name]['id'].startswith('asdk_app_'):
            raise ValueError('Expected a host app ID, not credentials or a new connection')
        interface = portable['extensions']['com.openai']['interface'].copy()
        # Keep the existing recognizable name; do not create a second plugin.
        interface['displayName'] = manifest['interface']['displayName']
        manifest = {key: manifest[key] for key in ('name', 'apps', 'author', 'homepage') if key in manifest}
        manifest.update(version=portable['version'], skills='./skills/',
                        description=portable['description'], interface=interface,
                        extensions={'com.openai': {
                            'onboardingSkill': './skills/ding-setup/SKILL.md',
                            'publication': publication,
                        }})
        files = {}
        for item in new.infolist():
            path = Path(item.filename)
            if path.is_absolute() or '..' in path.parts:
                raise ValueError('Unsafe archive path')
            if item.filename.startswith(('skills/', 'assets/')) or item.filename in ('README.md', 'privacy.md', 'LICENSE'):
                if item.file_size > 2 * 1024 * 1024:
                    raise ValueError('Unexpectedly large skill or asset')
                if item.filename in files:
                    raise ValueError('Duplicate archive member')
                files[item.filename] = new.read(item)
        for required in ('skills/ding-setup/SKILL.md', 'skills/ding-watch/SKILL.md'):
            if required not in files:
                raise ValueError('Missing journey skill')
        files['.codex-plugin/plugin.json'] = (json.dumps(manifest, indent=2) + '\n').encode()
        files['.app.json'] = (json.dumps(apps, indent=2) + '\n').encode()
        # No mcp.json: the preserved app binding owns the already-approved MCP
        # connection. Packaging a second server would create another login path.
        with zipfile.ZipFile(output, 'x', zipfile.ZIP_DEFLATED) as archive:
            for name, data in sorted(files.items()):
                archive.writestr(name, data)
    return hashlib.sha256(output.read_bytes()).hexdigest()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--existing', required=True, type=Path)
    parser.add_argument('--journey', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    print(build(args.existing, args.journey, args.output), args.output)
