import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "./",
  server: {
    // Dev preview only: read the local daemon without CORS. The Wails shell talks to it from Go.
    proxy: {
      "/__portly": {
        target: `http://127.0.0.1:${process.env.PORTLY_API_PORT ?? 7737}`,
        rewrite: (path) => path.replace(/^\/__portly/, ""),
      },
    },
  },
});
