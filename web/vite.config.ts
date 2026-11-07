import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import tsconfigPaths from "vite-tsconfig-paths";
import { VitePWA } from "vite-plugin-pwa";

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    tsconfigPaths(),
    VitePWA({
      registerType: "autoUpdate",
      // registro manual en main.tsx (scope = BASE_URL, raíz)
      injectRegister: false,
      manifest: {
        name: "rkt dashboard",
        short_name: "rkt",
        description: "Opera tu PaaS Windows: apps, deploys, logs.",
        lang: "es",
        theme_color: "#0c0e12",
        background_color: "#0c0e12",
        display: "standalone",
        start_url: "./",
        scope: "./",
        icons: [
          { src: "icons/icon-192.png", sizes: "192x192", type: "image/png" },
          { src: "icons/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
          { src: "icons/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
        ],
      },
      workbox: {
        // SPA: shell offline; api siempre red (con offline en UI).
        navigateFallback: "index.html",
        skipWaiting: true,
        clientsClaim: true,
        cleanupOutdatedCaches: true,
        runtimeCaching: [
          {
            urlPattern: /^https?:.*\/api\/v1\/.*/i,
            handler: "NetworkOnly",
          },
        ],
      },
    }),
  ],
  base: "/",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        // Vendor estable en chunks propios: mejor caché entre deploys.
        manualChunks: {
          react: ["react", "react-dom", "react-router"],
          ui: ["@base-ui/react", "lucide-react", "sonner", "zustand"],
        },
      },
    },
  },
  server: {
    host: "127.0.0.1",
    proxy: {
      // Backend local: `rkt dev` expone API+dashboard en healthz (8090).
      "/api": { target: "http://127.0.0.1:8090", changeOrigin: true },
    },
  },
});
