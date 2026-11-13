# Agio website

The site at <https://oneo-me.github.io/nsgo-agio/>: the product page in English
and Simplified Chinese, in light and dark appearances. It says what Agio is,
the agent core of NSGo apps and an app of its own, and links the app's
download to the latest GitHub release. It is built with Astro,
Tailwind and TypeScript; the page ships no UI framework and a small script
for the appearance, language choice, copy buttons, tabs, the screenshot
lightbox and the mobile menu.

```sh
bun install
bun run dev          # http://localhost:4321/
bun run build        # typecheck, then build dist/
bun run preview      # serve dist/ at /nsgo-agio/
bun run screenshots  # retake src/assets/screens/ from Agio's demo build (macOS)
bun run og           # redraw public/og*.png and the touch icons (macOS)
```

## As a template

The site doubles as the template for the workspace's product sites. It
reads like an app's own page: a centered hero, the app's window on a softly
tinted stage, features marked by one colored LED each, screenshots that open
in a lightbox, and a quiet footer. To start another app's site, copy
`website/` and replace:

- `src/copy.ts` and `src/site.ts`: the words in both languages, the links and
  the install command;
- `src/assets/screens/` and the `shots` in `src/layouts/Home.astro`;
- `src/components/Mark.astro`, `public/favicon.svg` and the social cards;
- the sections in `src/layouts/Home.astro` the app does not need;
- `agio-` in the storage keys and the Umami website ID and domains in
  `src/layouts/Layout.astro`.

`Stage` takes a tone and features and notes take an LED, named in
`src/template.ts` and drawn in `src/styles/global.css` for light and dark.

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

`src/assets/screens/en/` and `zh/` hold the app in each language's words and
both appearances: `app` a task, `detail` a turn with its steps open and `new`
New task, each as `-light.png` and `-dark.png`. They come from Agio's demo
build (`cmd/agio/demo.go`), the real app showing scripted tasks without an
agent CLI. `bun run screenshots` builds it, runs it with a HOME of its own
whose settings choose the language, appearance and color schemes, and
captures each window. The script needs macOS, a screen, and permission for
Terminal or the agent to control `System Events`; approve the prompt once, or
the clicks that open the steps do nothing. Quit any demo still open first, as
the script captures the first window it finds. The READMEs show `app` too.

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

The page counts visits in Agio's Umami site, tagged `website`, only on the
domains listed in the script's `data-domains`.
