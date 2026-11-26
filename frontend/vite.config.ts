import { defineConfig, loadEnv, type Plugin } from 'vite'
import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import Components from 'unplugin-vue-components/vite'
import { PrimeVueResolver } from '@primevue/auto-import-resolver'

// Хакатонная сборка не должна индексироваться ни в каком окружении (вместе с public/robots.txt).
function noindexPlugin(): Plugin {
  return {
    name: 'kuznica-noindex-meta',
    transformIndexHtml(html) {
      const meta = `    <meta name="robots" content="noindex, nofollow" />\n`
      return html.replace(/<head>\s*/i, (match) => `${match}${meta}`)
    },
  }
}

// Предзагрузка кириллической Golos 400 (основной текст): без неё шрифт запрашивается только после
// разбора CSS. Имя файла с хешем известно лишь в бандле, поэтому тег добавляется на этапе сборки.
function fontPreloadPlugin(): Plugin {
  let base = '/'
  return {
    name: 'kuznica-font-preload',
    apply: 'build',
    configResolved(config) {
      base = config.base
    },
    transformIndexHtml: {
      order: 'post',
      handler(_html, ctx) {
        const font = Object.keys(ctx.bundle ?? {}).find((f) =>
          /golos-text-cyrillic-400-normal-[\w-]+\.woff2$/.test(f),
        )
        if (!font) return []
        return [
          {
            tag: 'link',
            attrs: {
              rel: 'preload',
              as: 'font',
              type: 'font/woff2',
              href: `${base}${font}`,
              crossorigin: '',
            },
            injectTo: 'head',
          },
        ]
      },
    },
  }
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiTarget = env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8088'

  return {
    plugins: [
      vue(),
      tailwindcss(),
      // Компоненты PrimeVue (<Button>, <InputText>…) импортируются автоматически.
      Components({ dts: 'src/components.d.ts', resolvers: [PrimeVueResolver()] }),
      noindexPlugin(),
      fontPreloadPlugin(),
    ],
    resolve: {
      alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
    },
    server: {
      port: 5177,
      strictPort: true,
      proxy: { '/api': { target: apiTarget } },
    },
  }
})
