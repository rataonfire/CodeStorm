import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import dns from 'node:dns';
dns.setDefaultResultOrder('ipv4first'); // или 'verbatim'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
   server: {
    host: '0.0.0.0', // или '127.0.0.1', если нужно явно указать IPv4
    port: 5173
  }
})
