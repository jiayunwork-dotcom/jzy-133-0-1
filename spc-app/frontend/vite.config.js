import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 构建产物输出到 dist，由 nginx:1.27-alpine 托管。
// 开发时 /api 代理到本机 Go 后端；容器内走 nginx 反代（见 nginx.conf）。
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080'
    }
  }
})
