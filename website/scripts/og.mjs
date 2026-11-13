// Draws the English and Chinese social cards and the touch icons from the
// site's own pieces.
// Run `bun run og` after changing the look; the files in public/ are
// committed, so a build does not need Chrome or sips.
import { execFileSync } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const scripts = path.dirname(fileURLToPath(import.meta.url));
const website = path.resolve(scripts, '..');
const scratch = path.join(website, '.screenshots');
const chrome = process.env.CHROME ?? '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const mark = readFileSync(path.join(website, 'public', 'favicon.svg'), 'utf8');
// The app in a language's words, from `bun run screenshots`.
const shot = (lang) =>
  readFileSync(path.join(website, 'src', 'assets', 'screens', lang, 'app-dark.png')).toString('base64');

const font =
  "-apple-system, BlinkMacSystemFont, 'SF Pro Text', 'Segoe UI', 'PingFang SC', 'Noto Sans SC', sans-serif";
const mono = "ui-monospace, 'SF Mono', 'JetBrains Mono', Menlo, monospace";

mkdirSync(scratch, { recursive: true });

function page(name, width, height, body, style = '') {
  const file = path.join(scratch, `${name}.html`);
  writeFileSync(
    file,
    `<!doctype html><html><head><meta charset="utf-8"><style>
      * { box-sizing: border-box; }
      html, body { margin: 0; width: ${width}px; height: ${height}px; overflow: hidden; }
      body { background: #08090c; color: #f1f3f7; font-family: ${font}; }
      ${style}
    </style></head><body>${body}</body></html>`,
  );
  return file;
}

function render(name, width, height, file) {
  execFileSync(
    chrome,
    [
      '--headless=new',
      '--disable-gpu',
      '--hide-scrollbars',
      '--force-device-scale-factor=1',
      `--window-size=${width},${height}`,
      `--screenshot=${path.join(website, 'public', name)}`,
      `file://${file}`,
    ],
    { stdio: 'ignore' },
  );
}

function socialCard({ name, lang, title, lede, titleStyle }) {
  const file = page(
    name,
    1200,
    630,
    `<div class="glow"></div>
    <div class="left">
      <div class="brand">${mark}<span>Agio</span></div>
      <h1>${title}</h1>
      <p>${lede}</p>
      <div class="command">$ go get github.com/oneo-me/nsgo-agio</div>
    </div>
    <div class="shot"><img src="data:image/png;base64,${shot(lang)}" alt=""></div>
    <div class="foot">oneo-me/nsgo-agio · MIT</div>`,
    `
    .glow { position: absolute; left: 340px; top: -260px; width: 1000px; height: 900px;
      background: radial-gradient(closest-side, rgb(0 122 255 / 22%), transparent 70%); }
    .left { position: absolute; left: 64px; top: 58px; width: 520px; }
    .brand { display: flex; align-items: center; gap: 14px; }
    .brand svg { width: 52px; height: 52px; filter: drop-shadow(0 10px 24px rgb(0 122 255 / 35%)); }
    .brand span { font-size: 34px; font-weight: 700; letter-spacing: -.04em; }
    h1 { margin: 52px 0 0; font-size: 52px; line-height: 1.05; letter-spacing: -.045em; ${titleStyle} }
    p { margin: 26px 0 0; width: 460px; color: #9aa1ad; font-size: 17px; line-height: 1.55; }
    .command { margin-top: 30px; display: inline-block; padding: 12px 16px; border: 1px solid #22262f;
      border-radius: 9px; background: #101319; color: #bfe0ff; font-family: ${mono}; font-size: 13px; }
    .shot { position: absolute; left: 618px; top: 82px; width: 690px; height: 510px;
      overflow: hidden; border: 1px solid #303643; border-radius: 16px; background: #101319;
      box-shadow: 0 40px 100px -30px #000; transform: rotate(-1.2deg); }
    .shot img { width: 100%; height: 100%; object-fit: cover; object-position: left top; }
    .foot { position: absolute; left: 64px; bottom: 46px; color: #7f8794; font-family: ${mono}; font-size: 12px; }
    `,
  );
  render(`${name}.png`, 1200, 630, file);
}

socialCard({
  name: 'og',
  lang: 'en',
  title: 'Coding agents,<br>inside your app.',
  lede: 'The agent core of NSGo apps, for phones and desktops — and Agio, the app, for macOS, Windows and Linux.',
  titleStyle: '',
});

socialCard({
  name: 'og-zh',
  lang: 'zh',
  title: '编程智能体，<br>放进你的应用。',
  lede: 'NSGo 应用的智能体核心，适配手机与桌面——以及支持 macOS、Windows 和 Linux 的 Agio 应用。',
  titleStyle: 'font-size: 48px; letter-spacing: -.02em; line-height: 1.16;',
});

// The touch icon is square: iOS masks the corners itself, and a rounded
// mark inside a square would show two corner shapes.
const squareMark = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" fill="none">
  <rect width="1024" height="1024" fill="#f5f6f8"/>
  <path fill="#0a7cff" d="M362 232H662A150 150 0 0 1 812 382V566A150 150 0 0 1 662 716H436L300 812C284 823 264 810 268 791L288 704A150 150 0 0 1 212 566V382A150 150 0 0 1 362 232Z"/>
  <path d="M382 384 476 474 382 564" stroke="#fff" stroke-width="68" stroke-linecap="round" stroke-linejoin="round"/>
  <rect x="532" y="530" width="138" height="68" rx="34" fill="#fff"/>
</svg>`;
const icon = page(
  'icon',
  512,
  512,
  squareMark,
  `
  svg { display: block; width: 512px; height: 512px; }
  `,
);
render('icon.png', 512, 512, icon);
execFileSync('sips', ['-z', '180', '180', path.join(website, 'public', 'icon.png'), '--out', path.join(website, 'public', 'apple-touch-icon.png')], {
  stdio: 'ignore',
});
console.log('wrote public/og.png, public/og-zh.png and the icons');
