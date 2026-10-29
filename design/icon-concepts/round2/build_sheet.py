from pathlib import Path
import sys
sys.dont_write_bytecode = True
sys.path.insert(0, str(Path.home()/'.codex/skills/logo-design/scripts'))
from concept_sheet import embed
ROOT=Path(__file__).resolve().parent
def text(x,y,value,size=20,color='#202020',weight=400):
    return f'<text x="{x}" y="{y}" font-family="Segoe UI,Microsoft YaHei,sans-serif" font-size="{size}" font-weight="{weight}" fill="{color}">{value}</text>'
parts=['<rect width="1500" height="880" fill="#f9f9f9"/>', text(64,76,'Lilt / 新一轮图标',36,weight=650),text(64,115,'让音乐一眼可辨',20,'#646464')]
files=['a-flow-note.svg','b-turntable.svg','c-headphones.svg']
names=['A  流音','B  金唱针','C  随身听']
notes=['蓝青色流动双音符','薄荷绿唱机 · 金色唱针','紫橙耳机 · 中央音符']
for i,f in enumerate(files):
    x=60+i*470
    parts += [f'<rect x="{x}" y="157" width="440" height="665" rx="24" fill="#ffffff"/>',text(x+30,207,names[i],26,weight=650),embed(str(ROOT/f),f'large{i}',x+64,238,312,312),text(x+30,604,notes[i],20,'#646464')]
    for j,s in enumerate([64,32,24,16]):
        px=x+30+j*98
        parts += [embed(str(ROOT/f),f'small{i}-{s}',px,662+(64-s)/2,s,s),text(px,758,str(s)+' px',14,'#646464')]
parts += [text(64,858,'概念预览 · 图标内不含字母',15,'#646464')]
(ROOT/'concepts.svg').write_text('<svg xmlns="http://www.w3.org/2000/svg" width="1500" height="880" viewBox="0 0 1500 880">'+''.join(parts)+'</svg>',encoding='utf-8')
print('Wrote round2/concepts.svg')
