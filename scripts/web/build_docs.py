#!/usr/bin/env python3
"""Rebuild all public documentation channels from source, including frozen tags."""
import argparse
import json
import os
import re
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import yaml

ROOT = Path(__file__).resolve().parents[2]
PRODUCT = json.loads((ROOT / 'content/product.json').read_text())
OUT = ROOT / 'workers/docs/site'


def run(args, cwd=ROOT, env=None):
    subprocess.run(args, cwd=cwd, env=env, check=True)


def build_channel(source, destination, channel, ref, workspace, url_path=''):
    stage = workspace / ('stage-' + channel + '-' + ref.replace('/', '-'))
    shutil.copytree(source / 'docs', stage / 'docs', ignore=shutil.ignore_patterns('cli', 'requirements.txt', 'overrides'))
    run(['go', 'run', './cmd/docgen', str(stage / 'docs/cli')], cwd=source)
    assets = stage / 'docs/assets'
    assets.mkdir(exist_ok=True)
    shutil.copytree(ROOT / 'design/assets', assets, dirs_exist_ok=True)
    (stage / 'docs/stylesheets').mkdir(exist_ok=True)
    shutil.copy2(ROOT / 'design/tokens.css', stage / 'docs/stylesheets/tokens.css')
    shutil.copy2(ROOT / 'docs/stylesheets/extra.css', stage / 'docs/stylesheets/extra.css')
    if channel != 'legacy':
        shutil.copytree(source / 'examples/watches', assets / 'examples', dirs_exist_ok=True)
        shutil.copy2(source / 'ding.yaml.example', assets / 'examples/ding.yaml.example')
        shutil.copy2(source / 'schemas/watch-v1alpha1.json', assets / 'watch-v1alpha1.json')
    config = yaml.safe_load((source / 'mkdocs.yml').read_text())
    config.update(docs_dir=str(stage / 'docs'), site_dir=str(destination), site_url=PRODUCT['docsUrl'] + '/' + url_path,
                  strict=True, hooks=[str(ROOT / 'scripts/web/docs_hooks.py')])
    current = yaml.safe_load((ROOT / 'mkdocs.yml').read_text())
    for key in ('theme', 'extra_css', 'validation'):
        config[key] = current[key]
    config['theme']['custom_dir'] = str(ROOT / 'docs/overrides')
    config['site_name'] = 'Ding'
    config['nav'] = [entry for entry in config['nav'] if 'CLI Reference' not in entry]
    config['nav'].append({'CLI Reference': [{p.stem.replace('ding_', 'ding ').replace('_', ' '): 'cli/' + p.name} for p in sorted((stage/'docs/cli').glob('*.md'))]})
    config_file = stage / 'mkdocs.yml'
    config_file.write_text(yaml.safe_dump(config, sort_keys=False))
    env = dict(os.environ, DING_DOCS_SOURCE=str(source), DING_PRODUCT_FILE=str(ROOT/'content/product.json'), DING_DOCS_CHANNEL=channel, DING_DOCS_REF=ref)
    run([sys.executable, '-m', 'mkdocs', 'build', '--strict', '--config-file', str(config_file)], env=env)
    (destination / 'build-info.json').write_text(json.dumps({'source': ref, 'channel': channel, 'runtime': PRODUCT['runtime']}, indent=2)+'\n')
    if channel in ('preview','legacy'):
        (destination/'sitemap.xml').unlink(missing_ok=True)
        (destination/'sitemap.xml.gz').unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--current-only', action='store_true', help='Local iteration; never deploy this incomplete artifact')
    args = parser.parse_args()
    run(['node', '--input-type=module', '-e', "import {loadProduct} from './scripts/web/product.mjs'; loadProduct(new URL('./', 'file://' + process.cwd() + '/')); "])
    ref = subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip()
    with tempfile.TemporaryDirectory(prefix='ding-docs-') as temp:
        work = Path(temp)
        output = work / 'output'
        build_channel(ROOT, output, 'current', ref, work)
        if not args.current_only:
            build_channel(ROOT, output / 'preview', 'preview', ref, work, 'preview/')
            for archive in json.loads((ROOT/'content/docs-versions.json').read_text())['archives']:
                if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:[-.][A-Za-z0-9.-]+)?', archive['ref']):
                    raise ValueError('Archives must name a pinned release tag')
                path = Path(archive['path'])
                if path.is_absolute() or '..' in path.parts or path.parts[0] not in ('legacy','v'):
                    raise ValueError('Unsafe archive destination')
                # Tags are source snapshots. Extraction uses Git itself, not untrusted tar paths.
                archive_source = work / ('source-' + archive['ref'])
                archive_source.mkdir()
                run(['git','--work-tree='+str(archive_source),'restore','--source='+archive['ref'],'--worktree','--','.'])
                build_channel(archive_source, output/path, archive['channel'], archive['ref'], work, archive['path']+'/')
        (output/'robots.txt').write_text(f"User-agent: *\nAllow: /\nDisallow: /preview/\nDisallow: /legacy/v0.14.0/\nDisallow: /development/\nDisallow: /console/\nSitemap: {PRODUCT['docsUrl']}/sitemap.xml\n")
        (output/'_headers').write_text("/*\n  X-Content-Type-Options: nosniff\n  Referrer-Policy: strict-origin-when-cross-origin\n  X-Frame-Options: DENY\n/preview/*\n  X-Robots-Tag: noindex, follow\n/legacy/*\n  X-Robots-Tag: noindex, follow\n")
        (output/'channels.json').write_text(json.dumps({'complete':not args.current_only,'source':ref,'archives':json.loads((ROOT/'content/docs-versions.json').read_text())['archives']},indent=2)+'\n')
        OUT.parent.mkdir(exist_ok=True)
        shutil.rmtree(OUT, ignore_errors=True)
        shutil.copytree(output,OUT)
    print(f'Built docs → {OUT}')

if __name__ == '__main__':
    main()
