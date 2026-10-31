"""Menyalin tutorial_out ke source/web/static/tutorial (dilayani di /tutorial/) dengan gambar WebP.
Pakai: python publish.py   (jalankan node build.mjs dulu). Hanya gambar yang dirujuk index.html yang disalin."""
import re, shutil, sys
from pathlib import Path
from PIL import Image

root = Path(__file__).resolve().parents[2]
src = root / 'tutorial_out'
dst = root / 'source' / 'web' / 'static' / 'tutorial'
html = (src / 'index.html').read_text(encoding='utf8')

pat = re.compile(r'((?:guides|desktop|mobile)/[A-Za-z0-9_./\-]+)\.png')
files = sorted(set(m.group(1) for m in pat.finditer(html)))
if dst.exists():
    shutil.rmtree(dst)
dst.mkdir(parents=True)
before = after = 0
for f in files:
    s = src / (f + '.png')
    if not s.exists():
        print('hilang:', f)
        continue
    o = dst / (f + '.webp')
    o.parent.mkdir(parents=True, exist_ok=True)
    Image.open(s).convert('RGB').save(o, 'WEBP', quality=72, method=6)
    before += s.stat().st_size
    after += o.stat().st_size
(dst / 'index.html').write_text(pat.sub(r'\1.webp', html), encoding='utf8')
print(f'{len(files)} gambar, {before/1e6:.1f} MB -> {after/1e6:.1f} MB, ke {dst}')
