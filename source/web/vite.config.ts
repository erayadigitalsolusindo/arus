import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

// Dev server tidak melayani index.html di dalam folder static/; nginx produksi sudah melakukannya.
const landingDir = {
  name: 'landing-dir-index',
  configureServer(server: import('vite').ViteDevServer) {
    server.middlewares.use((req, _res, next) => {
      if (req.url === '/landing' || req.url === '/landing/') req.url = '/landing/index.html';
      next();
    });
  }
};

export default defineConfig({
  plugins: [
    landingDir,
    tailwindcss(),
    sveltekit({
      // Mode SPA: semua route di-render di browser, fallback ke index.html.
      adapter: adapter({ fallback: 'index.html' })
    })
  ],
  server: { port: 5173, strictPort: true }
});
