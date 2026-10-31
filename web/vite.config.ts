import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// The policy Caddy will send in production. `vite preview` sends it too, so the
// e2e suite fails on any violation long before deploy.
const CSP = [
  "default-src 'none'",
  "script-src 'self'",
  "style-src 'self'",
  "img-src 'self'",
  "font-src 'self'",
  "connect-src 'self'",
  "base-uri 'none'",
  "form-action 'self'",
  "frame-ancestors 'none'",
  "require-trusted-types-for 'script'",
].join("; ");

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: { "/api": "http://127.0.0.1:8080" },
  },
  preview: {
    headers: {
      "Content-Security-Policy": CSP,
      "X-Content-Type-Options": "nosniff",
      "Referrer-Policy": "strict-origin",
    },
  },
  build: { sourcemap: false },
  test: { include: ["src/**/*.test.tsx"], environment: "node" },
});
