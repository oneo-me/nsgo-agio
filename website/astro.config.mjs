import { defineConfig } from 'astro/config';
import sitemap from '@astrojs/sitemap';
import tailwindcss from '@tailwindcss/vite';

// The site is static and lives at the root of the repository's GitHub Pages
// project URL. CI passes the owner and repository name, so a fork or a
// rename keeps working; a custom domain overrides both with SITE_URL and
// BASE_PATH. The dev server stays at / for convenience.
const production = process.env.NODE_ENV === 'production';
const site = process.env.SITE_URL ?? 'https://oneo-me.github.io';
const base = process.env.BASE_PATH ?? (production ? '/nsgo-agio/' : '/');

export default defineConfig({
  site,
  base,
  trailingSlash: 'always',
  devToolbar: { enabled: false },
  integrations: [sitemap({ filter: (page) => !/\/404(\/|\.html)?$/.test(page) })],
  vite: { plugins: [tailwindcss()] },
});
