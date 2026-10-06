import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [
    tailwindcss(),
    sveltekit({
      // Mode SPA: semua route di-render di browser, fallback ke index.html.
      adapter: adapter({ fallback: 'index.html' })
    })
  ],
  server: { port: 5173, strictPort: true }
});
