// Retakes the website's screenshots from Agio's demo build on macOS: the
// real app showing scripted tasks (cmd/agio/demo.go), so the images need no
// agent CLI. Each language and appearance gets a task, its turn opened and
// New task. Run `bun run screenshots` after changing the app's look.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

if (process.platform !== 'darwin') {
  console.error('The screenshots need macOS: they capture a native NSGo window.');
  process.exit(1);
}

const scripts = path.dirname(fileURLToPath(import.meta.url));
const website = path.resolve(scripts, '..');
const repo = path.resolve(website, '..');
const scratch = path.join(website, '.screenshots');
const binary = path.join(scratch, 'agio-demo');
const screens = path.join(website, 'src', 'assets', 'screens');

// The color schemes shown, from cmd/agio/themes.go.
const schemes = { light: 'ocean', dark: 'ink' };
// The words the clicks look for, from chatview/locales.
const words = {
  en: { worked: 'Worked for', ran: 'Ran ', latest: 'Jump to latest' },
  zh: { worked: '用时', ran: '已执行', latest: '跳到最新' },
};

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function windowID() {
  try {
    return execFileSync('xcrun', ['swift', path.join(scripts, 'window-id.swift'), 'agio-demo'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim();
  } catch {
    return '';
  }
}

function osascript(lines) {
  execFileSync('osascript', lines.flatMap((line) => ['-e', line]), { stdio: ['ignore', 'ignore', 'inherit'] });
}

// clickButton presses the first button anywhere in the window whose
// description starts with text, such as the turn's fold row and its activity
// group; their order changes as the view gains buttons.
function clickButton(text) {
  osascript([
    'tell application "System Events"',
    '  tell process "agio-demo"',
    '    set els to entire contents of window 1',
    '    repeat with i from 1 to count of els',
    '      set e to item i of els',
    '      try',
    `        if role of e is "AXButton" and (description of e) starts with ${JSON.stringify(text)} then`,
    '          click e',
    '          return',
    '        end if',
    '      end try',
    '    end repeat',
    '  end tell',
    'end tell',
  ]);
}

// home writes a HOME of the demo's own, whose Agio settings choose the
// language and appearance and whose state sizes the window, so the user's
// own settings are neither read nor changed.
function home(lang, appearance) {
  const dir = path.join(scratch, 'home');
  rmSync(dir, { recursive: true, force: true });
  const agio = path.join(dir, 'Library', 'Application Support', 'Agio');
  mkdirSync(agio, { recursive: true });
  writeFileSync(
    path.join(agio, 'settings.yaml'),
    [
      `language: ${lang === 'zh' ? 'zh-Hans' : 'en'}`,
      `appearance: ${appearance}`,
      `light_scheme: ${schemes.light}`,
      `dark_scheme: ${schemes.dark}`,
      'stats_off: true',
      '',
    ].join('\n'),
  );
  writeFileSync(
    path.join(agio, 'state.yaml'),
    ['window:', '  width: 1180', '  height: 780', 'projects: [~/src/agio, ~/src/neorio, ~/src/nsgo]', ''].join('\n'),
  );
  return dir;
}

async function capture({ lang, appearance, shot }) {
  const { AGIO_DEMO_DETAIL, AGIO_DEMO_NEW, ...inherited } = process.env;
  const env = {
    ...inherited,
    HOME: home(lang, appearance),
    AGIO_DEMO_LANG: lang,
    ...(shot === 'detail' && { AGIO_DEMO_DETAIL: '1' }),
    ...(shot === 'new' && { AGIO_DEMO_NEW: '1' }),
  };
  const output = path.join(screens, lang, `${shot}-${appearance}.png`);

  const child = spawn(binary, { cwd: repo, env, stdio: 'ignore' });

  try {
    let id = '';
    for (let attempt = 0; attempt < 80 && !id; attempt++) {
      await sleep(100);
      if (child.exitCode !== null) throw new Error('The demo exited before its window opened.');
      id = windowID();
    }
    if (!id) throw new Error('The demo window did not appear.');

    osascript(['tell application "System Events"', '  set frontmost of process "agio-demo" to true', 'end tell']);
    await sleep(600);
    if (shot === 'detail') {
      // A single-turn task: open the fold row, then its activity group.
      clickButton(words[lang].worked);
      await sleep(450);
      clickButton(words[lang].ran);
      await sleep(700);
      // Opening the steps can leave the jump to the latest turn showing.
      clickButton(words[lang].latest);
      await sleep(500);
    }

    mkdirSync(path.dirname(output), { recursive: true });
    execFileSync('screencapture', ['-x', '-o', `-l${id}`, output], { stdio: 'inherit' });
    console.log(`wrote ${path.relative(website, output)}`);
  } finally {
    child.kill('SIGTERM');
    await Promise.race([new Promise((resolve) => child.once('exit', resolve)), sleep(2000)]);
  }
}

console.log('Building the demo…');
mkdirSync(scratch, { recursive: true });
execFileSync('go', ['build', '-tags', 'agiodemo', '-o', binary, './cmd/agio'], { cwd: repo, stdio: 'inherit' });

for (const lang of ['en', 'zh']) {
  for (const appearance of ['light', 'dark']) {
    for (const shot of ['app', 'detail', 'new']) {
      await capture({ lang, appearance, shot });
    }
  }
}
