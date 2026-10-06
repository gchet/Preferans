"""Make SVG outer outlines optional via #no-frame without duplicating artwork.

Run after importing or replacing decks. Original internal illustration is retained.
"""
from pathlib import Path
import argparse
import re
import xml.etree.ElementTree as ET

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--cards', type=Path, default=Path('web/public/cards'))
args = parser.parse_args()
sizes = {'atlas': (359, 539), 'english': (209, 314), 'woodcut': (248, 348)}
for deck, (width, height) in sizes.items():
    for path in (args.cards / deck).glob('*.svg'):
        text = path.read_text(encoding='utf-8')
        if 'id="no-frame"' in text:
            continue
        count = [0]
        def mark(match):
            tag = match.group()
            rect = ET.fromstring(tag)
            if abs(float(rect.get('width', '0')) - width) < 1 and abs(float(rect.get('height', '0')) - height) < 1:
                count[0] += 1
                return tag.replace('<rect', '<rect class="card-outline"', 1)
            return tag
        text = re.sub(r'<rect\b[^>]*?/\s*>', mark, text)
        assert count[0], f'No outer card outline found: {path}'
        style = '<g id="no-frame"/><style>#no-frame:target ~ .card-outline, #no-frame:target ~ * .card-outline { stroke: none !important; }</style>'
        text = re.sub(r'<svg\b[^>]*>', lambda m: m.group() + style, text, count=1)
        path.write_text(text, encoding='utf-8')
print('Optional outer frames prepared for all three decks.')
