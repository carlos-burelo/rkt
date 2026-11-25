import { defineConfig } from "astro/config";

// Site estático (landing + docs públicas). NO se embebe en rkt.exe.
// Deploy: GitHub Pages / Cloudflare. Fuente pública: site/docs/.
export default defineConfig({
  outDir: "dist",
  site: "https://rkt.example.com",
  markdown: {
    shikiConfig: { theme: "github-dark" },
  },
});
