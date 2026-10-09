"""Build-time substitutions; examples stay owned by the runtime repository."""
import json
import os
from pathlib import Path
import re
from urllib.parse import quote

ROOT = Path(os.environ['DING_DOCS_SOURCE']).resolve()
PRODUCT = json.loads(Path(os.environ['DING_PRODUCT_FILE']).read_text())
CHANNEL = os.environ.get('DING_DOCS_CHANNEL', 'current')
REF = os.environ.get('DING_DOCS_REF', PRODUCT['runtime']['sourceRef'])


def on_config(config):
    label = f"Watch source preview · {PRODUCT['runtime']['sourceRef']} · source build required"
    if PRODUCT['runtime']['channel'] == 'stable':
        label = f"Watch runtime · {PRODUCT['runtime']['version']}"
    if CHANNEL == 'legacy':
        label = f'Legacy runtime · {REF} · commands differ from persistent watches'
    elif CHANNEL == 'preview':
        label = f'Development preview · {REF} · not a released version'
    elif CHANNEL == 'version':
        label = f'Watch release · {REF}'
    config.extra.update(channel_label=label, noindex=CHANNEL in ('preview', 'legacy'),
                        public_docs_url=PRODUCT['docsUrl'] + '/')
    return config


def on_page_markdown(markdown, page, config, files):
    def snippet(match):
        name = match.group(1)
        path = (ROOT / name).resolve()
        if not path.is_relative_to(ROOT) or not (name == 'ding.yaml.example' or name.startswith('examples/watches/')):
            raise ValueError(f'Unsupported snippet {name}')
        return path.read_text().strip()
    def source_link(match):
        target = match.group(1)
        raw, _, line = target.partition(':')
        resolved = (ROOT / 'docs' / Path(page.file.src_uri).parent / raw).resolve()
        if resolved.is_relative_to(ROOT) and not resolved.is_relative_to(ROOT / 'docs'):
            path = quote(str(resolved.relative_to(ROOT)))
            anchor = '#L' + line if line.isdigit() else ''
            return '(' + PRODUCT['repositoryUrl'] + '/blob/' + REF + '/' + path + anchor + ')'
        return match.group(0)
    markdown = re.sub(r'\((\.\./\.\./[^)]+)\)', source_link, markdown)
    markdown = re.sub(r'\{\{\s*snippet:([^}\s]+)\s*\}\}', snippet, markdown)
    availability = ('The watch runtime is a **source preview**. Published v0.14.0 artifacts use the legacy runtime. '
                    'The Console is not released in this documented snapshot.')
    if PRODUCT['runtime']['channel'] == 'stable':
        availability = f"These instructions document **{PRODUCT['runtime']['version']}**."
    markdown = markdown.replace('{{ availability }}', availability).replace('{{ runtime_ref }}', PRODUCT['runtime']['sourceRef'])
    if '{{ snippet:' in markdown:
        raise ValueError(f'Unexpanded snippet in {page.file.src_uri}')
    return markdown


def on_post_page(output, page, config):
    # Material 9.6.22 supplies the dialog role but no accessible name.
    return output.replace('data-md-component="search" role="dialog"',
                          'data-md-component="search" role="dialog" aria-label="Search documentation"')
