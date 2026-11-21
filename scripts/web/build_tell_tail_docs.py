"""Publish Tell-Tail's own guides at immutable source revisions, never floating main."""
import json
from pathlib import Path
import posixpath
import re
import shutil
import subprocess
import sys
from urllib.parse import urlsplit
import yaml

ROOT = Path(__file__).resolve().parents[2]
PRODUCT = json.loads((ROOT / 'content/tell-tail.json').read_text())
PAGES = {
    'README.md': ('usage.md', 'Commands and alerts'),
    'docs/WEB-DASHBOARD.md': ('dashboard.md', 'Browser dashboard'),
    'docs/APPLE-UNIFIED.md': ('apple-unified.md', 'Apple unified logs'),
    'docs/OPERATIONS.md': ('operations.md', 'Operations and troubleshooting'),
    'docs/WEB-SERVER.md': ('web-server.md', 'HTTP and SSE API'),
    'docs/WEB-FRAMES.md': ('web-frames.md', 'Frame reference'),
    'docs/SNAPSHOTS.md': ('snapshots.md', 'Snapshot API'),
    'docs/OUTPUT.md': ('output.md', 'Output and backpressure'),
    'docs/DESKTOP-ACTIVITY.md': ('desktop-activity.md', 'AI Activity'),
    'docs/PLUGIN-TRACING.md': ('plugin-tracing.md', 'Desktop plugin and call inspection'),
    'docs/LOG-SOURCES.md': ('log-sources.md', 'Application log sources'),
    'docs/WORKSPACES.md': ('workspaces.md', 'Logs and AI Activity workspaces'),
    'docs/MCP.md': ('mcp.md', 'MCP diagnostics'),
    'docs/DEMO.md': ('demo.md', 'Built-in demo'),
    'docs/LOG-FORMATS.md': ('log-formats.md', 'Log format recognition'),
    'docs/MONITORING.md': ('monitoring.md', 'Understanding a live log stream'),
}
MEDIA = {
    'docs/design/desktop-activity-implemented.png': 'assets/tell-tail/desktop-activity-implemented.png',
}
PREVIEW_PAGES = {
    'docs/README.md': ('index.md', 'Preview overview'),
}


def rewrite_links(markdown, source_path, pages, ref):
    """Keep published guides local; link other repository files to the pinned source."""
    def link(match):
        target = match[1]
        parsed = urlsplit(target)
        if parsed.scheme or parsed.netloc or not parsed.path or target.startswith('/'):
            return match[0]
        path = posixpath.normpath(posixpath.join(posixpath.dirname(source_path), parsed.path))
        fragment = '#' + parsed.fragment if parsed.fragment else ''
        if path in MEDIA:
            return '](' + MEDIA[path] + fragment + ')'
        if path in pages:
            return '](' + pages[path][0] + fragment + ')'
        return '](' + PRODUCT['repositoryUrl'] + '/blob/' + ref + '/' + path + fragment + ')'
    return re.sub(r'\]\(([^\s)]+)\)', link, markdown)


def build_tell_tail(output, workspace):
    checkout = workspace / 'tell-tail-source'
    subprocess.run(['git', 'init', '-q', str(checkout)], check=True)
    subprocess.run(['git', '-C', str(checkout), 'remote', 'add', 'origin', PRODUCT['repositoryUrl'] + '.git'], check=True)
    for preview, ref in ((False, PRODUCT['docsRef']), (True, PRODUCT['previewRef'])):
        if not re.fullmatch(r'[0-9a-f]{40}', ref):
            raise ValueError('Tell-Tail docs must pin a full commit SHA')
        subprocess.run(['git', '-C', str(checkout), 'fetch', '-q', '--depth=1', 'origin', ref], check=True)
        subprocess.run(['git', '-C', str(checkout), 'checkout', '-q', '--detach', ref], check=True)
        stage = workspace / ('tell-tail-preview' if preview else 'tell-tail-stable')
        docs = stage / 'docs'
        docs.mkdir(parents=True)
        pages = {**PAGES, **(PREVIEW_PAGES if preview else {})}
        for source, (target, _) in pages.items():
            markdown = (checkout / source).read_text()
            # Release snapshots still contain links to the former publishing origin.
            markdown = markdown.replace('[Ding docs](https://docs.ding.ing/tell-tail/', '[Tell-Tail docs](https://docs.ding.ing/tell-tail/')
            markdown = markdown.replace('https://docs.ding.ing/tell-tail/', PRODUCT['websiteUrl'] + '/docs/')
            markdown = markdown.replace('[Ding docs](https://docs.ding.ing/)', '[Tell-Tail home](' + PRODUCT['websiteUrl'] + '/)')
            repository = PRODUCT['repositoryUrl']
            markdown = markdown.replace('https://github.com/ding-labs/tell-tail', repository)
            if not preview:
                markdown = markdown.replace(f'git clone {repository}.git',
                                            f'git clone --branch {PRODUCT["releaseRef"]} {repository}.git')
            (docs / target).write_text(rewrite_links(markdown, source, pages, ref))
        if not preview:
            shutil.copy2(ROOT / 'content/tell-tail/index.md', docs / 'index.md')
        shutil.copytree(ROOT / 'design/assets', docs / 'assets')
        for source, target in MEDIA.items():
            shutil.copy2(checkout / source, docs / target)
        (docs / 'stylesheets').mkdir()
        shutil.copy2(ROOT / 'design/tokens.css', docs / 'stylesheets/tokens.css')
        shutil.copy2(ROOT / 'docs/stylesheets/extra.css', docs / 'stylesheets/extra.css')
        config = yaml.safe_load((ROOT / 'mkdocs.yml').read_text())
        path = 'docs/next/' if preview else 'docs/'
        destination = output / path
        config.update(site_name='Tell-Tail', site_description='Live log throughput and severity in your terminal or browser',
                      site_url=PRODUCT['websiteUrl'] + '/' + path, repo_url=PRODUCT['repositoryUrl'],
                      repo_name=urlsplit(PRODUCT['repositoryUrl']).path.strip('/'),
                      copyright='Tell-Tail · MIT · Created and maintained by <a href="https://github.com/zuchka">Matt Abrams</a>',
                      docs_dir=str(docs), site_dir=str(destination),
                      hooks=[str(ROOT / 'scripts/web/tell_tail_docs_hooks.py')],
                      nav=[{'Start': [{'Overview and install': 'index.md'}, {'Commands and alerts': 'usage.md'}]},
                           {'Guides and reference': [{title: file} for file, title in pages.values() if file not in ('index.md', 'usage.md')]}],
                      extra={'tell_tail_preview': preview, 'tell_tail_ref': ref, 'tell_tail_version': PRODUCT['version']})
        config['theme']['logo'] = config['theme']['favicon'] = 'assets/tell-tail/logo.svg'
        config['theme']['custom_dir'] = str(ROOT / 'docs/overrides')
        config_file = stage / 'mkdocs.yml'
        config_file.write_text(yaml.safe_dump(config, sort_keys=False))
        subprocess.run([sys.executable, '-m', 'mkdocs', 'build', '--strict', '-f', str(config_file)], check=True)
        (destination / 'build-info.json').write_text(json.dumps({'product': 'tell-tail', 'source': ref,
            'version': PRODUCT['version'], 'channel': 'preview' if preview else 'stable'}, indent=2) + '\n')
        if preview:
            (destination / 'sitemap.xml').unlink(missing_ok=True)
            (destination / 'sitemap.xml.gz').unlink(missing_ok=True)
