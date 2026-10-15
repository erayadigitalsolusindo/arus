// Pewarna sintaks ringan (tanpa dependensi). Mengembalikan HTML yang SUDAH di-escape; aman untuk {@html}.

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const span = (cls: string, s: string) => `<span class="tk-${cls}">${esc(s)}</span>`;

const KEYWORDS =
  /\b(?:import|from|const|let|var|function|async|await|return|if|else|for|foreach|while|def|class|package|func|type|struct|new|public|private|use|echo|print|fn|try|catch|except|finally|export|default|true|false|null|nil|None|True|False|in|of|as|with|range|defer|var|string|int|bool)\b/;

const TOKEN = new RegExp(
  [
    String.raw`(?<com>(?:^|\s)(?:#|\/\/)[^\n]*|\/\*[\s\S]*?\*\/)`,
    String.raw`(?<str>"(?:\\.|[^"\\\n])*"|'(?:\\.|[^'\\\n])*'|` + '`(?:\\\\.|[^`\\\\])*`)',
    String.raw`(?<num>\b-?\d+(?:\.\d+)?\b)`,
    `(?<kw>${KEYWORDS.source})`
  ].join('|'),
  'g'
);

export function highlight(code: string, lang: string): string {
  if (lang === 'json') return highlightJson(code);
  let out = '';
  let last = 0;
  for (const m of code.matchAll(TOKEN)) {
    const idx = m.index ?? 0;
    out += esc(code.slice(last, idx));
    const g = m.groups ?? {};
    if (g.com !== undefined) {
      // komentar bisa diawali spasi (pola `\s#`): spasi dipertahankan di luar span
      const lead = g.com.match(/^\s*/)?.[0] ?? '';
      out += esc(lead) + span('com', g.com.slice(lead.length));
    } else if (g.str !== undefined) out += span('str', g.str);
    else if (g.num !== undefined) out += span('num', g.num);
    else out += span('kw', g.kw ?? m[0]);
    last = idx + m[0].length;
  }
  return out + esc(code.slice(last));
}

const JSON_TOKEN = /("(?:\\.|[^"\\])*")(\s*:)?|\b(true|false|null)\b|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)/g;

function highlightJson(code: string): string {
  let out = '';
  let last = 0;
  for (const m of code.matchAll(JSON_TOKEN)) {
    const idx = m.index ?? 0;
    out += esc(code.slice(last, idx));
    if (m[1] !== undefined) out += m[2] ? span('key', m[1]) + esc(m[2]) : span('str', m[1]);
    else if (m[3] !== undefined) out += span('kw', m[3]);
    else out += span('num', m[4]);
    last = idx + m[0].length;
  }
  return out + esc(code.slice(last));
}
