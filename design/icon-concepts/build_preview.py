"""Concept presentation only; no production app resources are modified."""
from pathlib import Path
import sys
import re
sys.dont_write_bytecode = True
sys.path.insert(0, str(Path.home() / '.codex/skills/logo-design/scripts'))
from concept_sheet import embed

ROOT = Path(__file__).resolve().parent
INK, BG, LINE, MUTED, GREEN, DEEP = '#1a211e', '#fbfdfc', '#dfe4e0', '#63706b', '#30a46c', '#193b2d'

def text(x, y, value, size=20, color=INK, weight=400):
    return f'<text x="{x}" y="{y}" font-family="Segoe UI,Microsoft YaHei,sans-serif" font-size="{size}" font-weight="{weight}" fill="{color}">{value}</text>'

def svg(w, h, contents):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">{contents}</svg>'

def tile(file):
    raw = file.read_text(encoding='utf-8')
    body = re.sub(r'^.*?<title>.*?</title>', '', raw).replace('</svg>', '').replace(INK, '#ffffff')
    return svg(256, 256, f'<rect width="256" height="256" rx="58" fill="{GREEN}"/><g transform="translate(28 28) scale(.78125)">{body}</g>')

names = ['弦间', '回旋', '跃拍']
notes = ['一枚拨片，一道起伏的弦。', '把旧唱机收敛成旋转的唱片。', '错落的节拍，轻快向上。']
parts = [f'<rect width="1500" height="920" fill="{BG}"/>', text(60, 76, 'Lilt / 图标概念', 36, weight=650), text(60, 113, '纯图形 · 无字母 · 原生桌面', 18, MUTED)]
for i, letter in enumerate('abc'):
    x = 60 + i * 470
    source = ROOT / f'{letter}-v2.svg'
    tile_path = ROOT / f'{letter}-app-preview.svg'
    tile_path.write_text(tile(source), encoding='utf-8')
    parts += [f'<rect x="{x}" y="150" width="440" height="708" rx="24" fill="#ffffff" stroke="{DEEP if i == 0 else LINE}" stroke-width="{2 if i == 0 else 1}"/>',
              text(x+28, 194, f'{letter.upper()}  {names[i]}', 24, weight=650),
              embed(str(source), letter, x+84, 225, 272, 272),
              text(x+28, 534, notes[i], 18, MUTED)]
    if i == 0:
        parts += [text(x+348, 193, '推荐', 16, DEEP, 600)]
    for j, size in enumerate([64, 32, 16]):
        sx = x + 32 + j * 90
        parts += [embed(str(source), f'{letter}-{size}', sx, 579+(64-size)/2, size, size), text(sx, 671, str(size)+' px', 13, MUTED)]
    parts += [f'<path d="M{x+28} 702H{x+412}" stroke="{LINE}"/>', embed(str(tile_path), letter+'-tile', x+28, 728, 96, 96), text(x+146, 770, '桌面效果预览', 18, weight=600), text(x+146, 801, 'Radix Green / Sage', 15, MUTED)]
parts += [text(60, 896, '先选轮廓，再精修色彩与各平台图标。', 15, MUTED)]
(ROOT/'concepts.svg').write_text(svg(1500, 920, ''.join(parts)), encoding='utf-8')

# Inspection sheet: positive, reversed, tiny, mirrored, and rotated.
proof = [f'<rect width="1200" height="960" fill="{BG}"/>']
for i, letter in enumerate('abc'):
    source = ROOT/f'{letter}-v2.svg'
    y = i*320
    proof += [text(24,y+30,letter.upper(),20,weight=600),embed(str(source),letter+'-proof',24,y+48,224,224)]
    white = ROOT/f'{letter}-proof-white.svg'
    white.write_text(source.read_text(encoding='utf-8').replace(INK,'#ffffff'),encoding='utf-8')
    proof += [f'<rect x="280" y="{y+48}" width="224" height="224" rx="20" fill="{INK}"/>',embed(str(white),letter+'-white',280,y+48,224,224)]
    proof += [f'<g transform="translate(1264 0) scale(-1 1)">{embed(str(source),letter+"-mirror",520,y+48,224,224)}</g>', f'<g transform="rotate(180 900 {y+160})">{embed(str(source),letter+"-rotate",788,y+48,224,224)}</g>']
    for j, size in enumerate([16,24,32,48]):
        proof += [embed(str(source),letter+str(size),1040,y+50+j*60,size,size)]
(ROOT/'proof.svg').write_text(svg(1200,960,''.join(proof)),encoding='utf-8')
print('Wrote concepts.svg, proof.svg, and concept app previews.')
