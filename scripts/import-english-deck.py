"""Import the public-domain English-pattern deck with vertical indices."""
from pathlib import Path
import argparse
import sys
from concurrent.futures import ThreadPoolExecutor
from urllib.request import urlopen
import xml.etree.ElementTree as ET

REV = 'f8df28774736ea2545fc8e7fb693eb55ce031945'
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', type=Path, default=Path('web/public/cards/english'))
args = parser.parse_args()
OUT = args.output
OUT.mkdir(parents=True, exist_ok=True)
def download(card):
    suit, rank = card
    source_suit = dict(spades='spade', clubs='club', diamonds='diamond', hearts='heart')[suit]
    source_rank = rank.title() if rank.isalpha() else rank
    url = f'https://raw.githubusercontent.com/saulspatz/SVGCards/{REV}/Decks/Vertical2/svgs/{source_suit}{source_rank}.svg'
    data = urlopen(url, timeout=60).read()
    root = ET.fromstring(data)
    assert not list(root.iter('{http://www.w3.org/2000/svg}image')), 'Expected vector artwork'
    (OUT / f'{rank}-{suit}.svg').write_bytes(data)
with ThreadPoolExecutor(max_workers=6) as pool:
    list(pool.map(download, [(s,r) for s in ['spades','clubs','diamonds','hearts'] for r in ['7','8','9','10','jack','queen','king','ace']]))
print('Imported 32 English-pattern SVG cards')
import subprocess
subprocess.run([sys.executable, 'scripts/card-frames.py', '--cards', str(OUT.parent)], check=True)
