// Render markdown keterangan item ke HTML aman. Satu-satunya tempat markdown dirender; hasilnya boleh dipakai
// dengan {@html} (pengecualian dari aturan "jangan {@html} untuk data pengguna") karena:
//   - `html: false` → tag HTML mentah di sumber di-escape, bukan diteruskan;
//   - markdown-it menolak URL berbahaya (javascript:, vbscript:, file:, data:) pada tautan;
//   - gambar dimatikan (keterangan tidak boleh memuat sumber luar / pelacak);
//   - tautan dipaksa rel="noopener noreferrer nofollow" + target=_blank.
import MarkdownIt from 'markdown-it';

const md = new MarkdownIt({ html: false, linkify: false, breaks: true, typographer: false });
md.disable(['image']);

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
