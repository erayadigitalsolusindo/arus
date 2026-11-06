// Render markdown keterangan item ke HTML aman. Satu-satunya tempat markdown dirender; hasilnya boleh dipakai
// dengan {@html} (pengecualian dari aturan "jangan {@html} untuk data pengguna") karena:
//   - `html: false` → tag HTML mentah di sumber di-escape, bukan diteruskan;
//   - markdown-it menolak URL berbahaya (javascript:, vbscript:, file:, data:) pada tautan;
//   - gambar dimatikan (keterangan tidak boleh memuat sumber luar / pelacak);
//   - tautan dipaksa rel="noopener noreferrer nofollow" + target=_blank.
import MarkdownIt from 'markdown-it';

const md = new MarkdownIt({ html: false, linkify: true, breaks: true, typographer: false });
md.disable(['image']);

// Daftar tugas ala GitHub: "- [ ] belum" / "- [x] selesai" menjadi kotak centang baca-saja. Satu-satunya HTML mentah yang
// dihasilkan di sini adalah konstanta di bawah (bukan dari sumber pengguna), jadi aturan `html: false` tetap utuh.
md.core.ruler.after('inline', 'task-lists', (state) => {
  const tk = state.tokens;
  for (let i = 2; i < tk.length; i++) {
    if (tk[i].type !== 'inline' || tk[i - 1].type !== 'paragraph_open' || tk[i - 2].type !== 'list_item_open') continue;
    const first = tk[i].children?.[0];
    const m = first?.type === 'text' ? /^\[([ xX])\]\s+/.exec(first.content) : null;
    if (!first || !m) continue;
    first.content = first.content.slice(m[0].length);
    const box = new state.Token('html_inline', '', 0);
    box.content = `<input type="checkbox" disabled${m[1] === ' ' ? '' : ' checked'} class="md-task"> `;
    tk[i].children!.unshift(box);
    tk[i - 2].attrJoin('class', 'md-task-item');
  }
});

const defaultLinkOpen = md.renderer.rules.link_open ?? ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));
md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  const t = tokens[idx];
  t.attrSet('rel', 'noopener noreferrer nofollow');
  t.attrSet('target', '_blank');
  return defaultLinkOpen(tokens, idx, options, env, self);
};

export function renderMarkdown(src: string): string {
  return md.render(src);
}
