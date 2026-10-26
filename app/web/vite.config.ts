import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { existsSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const environmentFile = fileURLToPath(new URL('../.local/environment.json', import.meta.url));
const local = existsSync(environmentFile) ? JSON.parse(readFileSync(environmentFile, 'utf8')) as Record<string, string> : {};
const target = process.env.CP_DEV_API_ORIGIN ?? (local.CP_HTTP_ADDR ? 'http://' + local.CP_HTTP_ADDR : 'http://127.0.0.1:8080');
export default defineConfig({
  plugins: [react()],
  server: { proxy: { '/v1': { target, changeOrigin: false }, '/health': { target } } },
  build: { sourcemap: false, target: 'es2022' },
});
