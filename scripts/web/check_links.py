#!/usr/bin/env python3
"""Check links and fragments across both generated origins, including archives."""
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import urljoin, urlsplit, unquote
import json
import sys

ROOT=Path(__file__).resolve().parents[2]
ORIGINS={'ding.ing':ROOT/'workers/website/dist','docs.ding.ing':ROOT/'workers/docs/site'}
REDIRECTS=json.loads((ROOT/'content/redirects.json').read_text())
class Page(HTMLParser):
    def __init__(self,path):
        super().__init__();self.links=[];self.ids=set();self.feed(path.read_text())
    def handle_starttag(self,tag,attrs):
        attrs=dict(attrs)
        if 'id' in attrs:self.ids.add(attrs['id'])
        if tag=='a' and 'name' in attrs:self.ids.add(attrs['name'])
        for key in ('href','src'):
            if attrs.get(key):self.links.append(attrs[key])

def resolve(url):
    parsed=urlsplit(url)
    if parsed.hostname not in ORIGINS:return None
    path=unquote(parsed.path)
    if parsed.hostname=='ding.ing' and path.rstrip('/') in REDIRECTS:
        return resolve(REDIRECTS[path.rstrip('/')])
    root=ORIGINS[parsed.hostname]
    target=(root/path.lstrip('/')).resolve()
    if not target.is_relative_to(root.resolve()):return False
    if target.is_dir():target=target/'index.html'
    if not target.exists() and not target.suffix:target=target.with_suffix('.html')
    return target,unquote(parsed.fragment)

def main():
    errors=[];count=0;cache={}
    for host,root in ORIGINS.items():
        if not (root/'index.html').exists():raise SystemExit(f'Build {host} before checking links')
        for file in root.rglob('*.html'):
            page=cache.setdefault(file,Page(file));rel=file.relative_to(root).as_posix()
            base='https://'+host+'/'+(rel[:-10] if rel.endswith('index.html') else rel)
            for link in page.links:
                if link.startswith(('mailto:','tel:','data:','javascript:')):continue
                destination=resolve(urljoin(base,link))
                if destination is None:continue
                count+=1
                if not destination:
                    errors.append(f'{host}/{rel}: unsafe link {link}');continue
                target,fragment=destination
                if not target.is_file():
                    errors.append(f'{host}/{rel}: missing {link}');continue
                if fragment and target.suffix=='.html':
                    linked=cache.setdefault(target,Page(target))
                    if fragment not in linked.ids:errors.append(f'{host}/{rel}: missing fragment {link}')
    if errors:
        print('\n'.join(sorted(set(errors))))
        raise SystemExit(f'{len(errors)} broken links')
    print(f'Checked {count} internal links and fragment targets across both origins')
if __name__=='__main__':main()
