# Agio website

The site at <https://oneo-me.github.io/nsgo-agio/>: the product page in English
and Simplified Chinese, in light and dark appearances. It is built with Astro,
Tailwind and TypeScript; the page ships no UI framework and a small script
for the appearance, language choice, copy buttons, tabs and the mobile menu.

```sh
bun install
bun run dev          # http://localhost:4321/
bun run build        # typecheck, then build dist/
bun run preview      # serve dist/ at /nsgo-agio/
bun run screenshots  # retake src/assets/screens/ from website/demo (macOS)
bun run og           # redraw public/og*.png and the touch icons (macOS)
```

## Language and appearance

English is at the base and Chinese under `zh/`. A first visit from a Chinese
browser goes to `zh/`; a choice made with the switch is kept in `agio-lang`.
The appearance follows the system until the header's switch chooses light,
dark or system; the choice is kept in `agio-theme`. Both are applied by an
inline head script before the first paint, so a stored choice does not flash
the other one.

All of the page's words are in `src/copy.ts`, with English and Chinese side by
side. Add both together; Chinese headings, titles and kickers omit the trailing
。 and body sentences keep it. `src/site.ts` maps a language to its path and
holds the outside links.

## Base path

The site is published under the repository's GitHub Pages project URL, so a
production build uses `/nsgo-agio/`; the dev server uses `/` for convenience.
CI sets `SITE_URL` and `BASE_PATH` from the repository, so a fork deploys at
its own address. A custom domain sets both explicitly:

```sh
SITE_URL=https://agio.example BASE_PATH=/ bun run build
```

## Screenshots

`src/assets/screens/` holds the app in both appearances:
`app-light.png`, `app-dark.png`, `app-detail-light.png` and
`app-detail-dark.png`. They come from `website/demo`, a Go program with a
scripted backend and no agent CLI. `bun run screenshots` builds it, opens a
native window, captures both appearances, and opens the turn's steps for the
detail images. The script needs macOS, a screen, and permission for Terminal
or the agent to control `System Events`; approve the prompt once, or the clicks
that open the steps do nothing.

The site imports the PNGs and lets Astro make AVIF and WebP versions at
several widths; CSS shows the image that matches the chosen appearance.

## Social cards and icons

`bun run og` renders `public/og.png` (English), `public/og-zh.png` (Chinese),
`public/icon.png` and `public/apple-touch-icon.png` with headless Chrome and
`sips`. The generated files are committed; a build needs neither.

## Deployment

The [Website workflow](../.github/workflows/website.yml) builds `website/dist`
and publishes it to GitHub Pages on pushes to `main` that touch `website/`.
Enable Pages for the repository with GitHub Actions as the source; a private
repository needs a plan that includes Pages.

The page counts visits in the workspace's Umami site, tagged `agio-website`;
add the Pages domain to that site in Umami for the script to run there.
