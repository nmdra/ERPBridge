import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react()],
  build: {
    emptyOutDir: false,
    outDir: "../internal/web/prebuilt/build",
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
  },
});
