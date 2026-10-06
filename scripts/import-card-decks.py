"""Rebuild editable, self-contained preference decks from CC0 SVG originals.

Run from the repository root. Sources and attribution: web/public/cards/README.md.
"""
from pathlib import Path
import argparse
import hashlib
import os
import shutil
import sys
import concurrent.futures
import copy
import json
import re
import subprocess
import urllib.request
import xml.etree.ElementTree as ET

NS = 'http://www.w3.org/2000/svg'
ET.register_namespace('', NS)
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--inkscape', default=os.environ.get('INKSCAPE') or shutil.which('inkscape'))
parser.add_argument('--source', type=Path, default=Path('.build/card-sources/atlas.svg'))
parser.add_argument('--output', type=Path, default=Path('web/public/cards'))
args = parser.parse_args()
if not args.inkscape:
    parser.error('Install Inkscape on PATH or pass --inkscape.')
OUT = args.output
SUITS = ['spades', 'clubs', 'diamonds', 'hearts']
RANKS = ['7', '8', '9', '10', 'jack', 'queen', 'king', 'ace']

def svg(**attrs):
    return ET.Element(f'{{{NS}}}svg', {k: str(v) for k, v in attrs.items()})

def write(root, deck, rank, suit):
    folder = OUT / deck
    folder.mkdir(parents=True, exist_ok=True)
    ET.ElementTree(root).write(folder / f'{rank}-{suit}.svg', encoding='utf-8', xml_declaration=True)

if not args.source.exists():
    args.source.parent.mkdir(parents=True, exist_ok=True)
    request = urllib.request.Request('https://upload.wikimedia.org/wikipedia/commons/5/55/Atlasnye_playing_cards_deck.svg', headers={'User-Agent': 'Preferans-card-import/1.0'})
    data = urllib.request.urlopen(request, timeout=60).read()
    if hashlib.sha256(data).hexdigest() != '6a23c43a1e4245a2c8e4aca27db143e04f5330a133890f2a95e33f0823c23c42':
        raise ValueError('Atlas source checksum mismatch; inspect the upstream file before updating the pin.')
    args.source.write_bytes(data)
source = ET.parse(args.source).getroot()
query = subprocess.run([args.inkscape, str(args.source), '--query-all'], capture_output=True, text=True, check=True)
boxes = {line.split(',')[0]: list(map(float, line.split(',')[1:])) for line in query.stdout.splitlines()}
for row, suit in enumerate(['hearts', 'diamonds', 'clubs', 'spades']):
    for col, rank in {0: 'ace', 6: '7', 7: '8', 8: '9', 9: '10', 10: 'jack', 11: 'queen', 12: 'king'}.items():
        left, top = 30+390*col, 30+570*row
        card = svg(viewBox=f'{left} {top} 360 540', width=360, height=540)
        # Two groups in the original sheet span two cards. Cropping by the
        # original grid handles those too, preserving every original path.
        for group in source.findall(f'{{{NS}}}g'):
            x, y, w, h = boxes[group.get('id')]
            if x < left+360 and x+w > left and y < top+540 and y+h > top:
                card.append(copy.deepcopy(group))
        assert len(card), f'Missing {rank}-{suit}'
        write(card, 'atlas', rank, suit)

# The second deck has its own English-pattern figures.
subprocess.run([sys.executable, 'scripts/import-english-deck.py', '--output', str(OUT / 'english')], check=True)

commit = '29f062e550f0b8add3a9ba31f706cc610e370b39'
def download(item):
    s, r = item
    name = ['7','8','9','10','J','Q','K','A'][r] + ['S','C','D','H'][s]
    url = f'https://raw.githubusercontent.com/SONDLecT/woodcut-cards/{commit}/cards/{name}.svg'
    data = urllib.request.urlopen(url, timeout=60).read()
    art = ET.fromstring(data)
    art.set('x', '34'); art.set('y', '26'); art.set('width', '182'); art.set('height', '298')
    root = svg(viewBox='0 0 250 350', width=250, height=350)
    ET.SubElement(root, f'{{{NS}}}rect', x='1', y='1', width='248', height='348', rx='12', fill='#f7f5ee', stroke='#a58c55', **{'stroke-width': '2'})
    root.append(art)
    for rotation in ['', 'rotate(180 125 175)']:
        g = ET.SubElement(root, f'{{{NS}}}g', fill='#25292c' if s < 2 else '#a82424', **{'font-family': 'DejaVu Sans, Arial, sans-serif', 'font-weight': 'bold', 'text-anchor': 'middle', 'transform': rotation})
        ET.SubElement(g, f'{{{NS}}}text', x='20', y='39', **{'font-size': '30' if r != 3 else '26'}).text = ['7','8','9','10','В','Д','К','Т'][r]
        ET.SubElement(g, f'{{{NS}}}text', x='20', y='69', **{'font-size': '30'}).text = ['♠','♣','♦','♥'][s]
    write(root, 'woodcut', RANKS[r], SUITS[s])
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
    list(pool.map(download, [(s, r) for s in range(4) for r in range(8)]))
print(f'Created 96 SVG cards. Woodcut revision: {commit}')
subprocess.run([sys.executable, 'scripts/card-frames.py', '--cards', str(OUT)], check=True)
