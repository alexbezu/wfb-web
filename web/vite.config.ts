import { defineConfig } from "vite";

export default defineConfig({
  base: "/app/",
  build: {
    outDir: "../internal/frontend/dist",
    emptyOutDir: true
  }
});
