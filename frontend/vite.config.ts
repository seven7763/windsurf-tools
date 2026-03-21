import type {Plugin} from 'vite'
import {readFileSync} from 'fs'
import {resolve, dirname} from 'path'
import {fileURLToPath} from 'url'
import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

const __dirname = dirname(fileURLToPath(import.meta.url))
const pkg = JSON.parse(readFileSync(resolve(__dirname, 'package.json'), 'utf-8')) as {version: string}

/** Wails/WebView2：去掉 module 脚本的 crossorigin，避免嵌入协议下脚本不执行（只剩 CSS/背景网格） */
function stripModuleScriptCrossorigin(): Plugin {
  return {
    name: 'strip-module-script-crossorigin',
    apply: 'build',
    transformIndexHtml(html) {
      return html.replace(/\s+crossorigin(?:="anonymous")?/g, '')
    },
  }
}

export default defineConfig({
  // Wails 嵌入资源必须用相对路径，否则生产环境 /assets/* 会 404，页面空白
  base: './',
  define: {
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(pkg.version),
  },
  plugins: [vue(), stripModuleScriptCrossorigin()],
  server: {
    host: '127.0.0.1',
    port: 3457,
    strictPort: true,
  },
  build: {
    // WebView2 下多 chunk + modulepreload/fetch 偶发导致主脚本不执行，只剩 body 背景（看似黑屏）
    target: 'chrome100',
    modulePreload: false,
    rollupOptions: {
      output: {
        // 单文件避免 ./vendor-*.js 二次请求在嵌入资源环境下失败
        inlineDynamicImports: true,
      },
    },
  },
})
