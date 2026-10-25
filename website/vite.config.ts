import { defineConfig } from 'vite'
import { tanstackStart } from '@tanstack/solid-start/plugin/vite'
import solid from 'vite-plugin-solid'

export default defineConfig({
  plugins: [
    // Every page is rendered at build time, so the site is plain files
    // that Cloudflare serves as static assets.
    tanstackStart({
      prerender: {
        enabled: true,
        crawlLinks: true,
        failOnError: true,
        // Links to a section of a page are that page.
        filter: ({ path }) => !path.includes('#'),
      },
      // Cloudflare serves 404.html for paths that have no file.
      pages: [{ path: '/404', prerender: { autoSubfolderIndex: false } }],
    }),
    solid({ ssr: true }),
  ],
})
