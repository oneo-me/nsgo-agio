// Retakes the website's light and dark screenshots from website/demo on
// macOS. The demo is a scripted backend, so the images show the real chatview
// without an agent CLI. Run `bun run screenshots` after changing the view's
// look.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync } from 'node:fs';
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

// clickButton presses the first button whose description starts with text,
// such as the turn's fold row and its activity group; their order changes as
// the view gains buttons.
function clickButton(text) {
  osascript([
    'tell application "System Events"',
    '  tell process "agio-demo"',
    '    set btns to every button of window 1',
    '    repeat with b in btns',
    `      if (description of b) starts with ${JSON.stringify(text)} then`,
    '        click b',
    '        return',
    '      end if',
    '    end repeat',
    '  end tell',
    'end tell',
  ]);
}

async function capture({ dark = false, detail = false, output }) {
  const env = { ...process.env };
  if (dark) env.AGIO_DEMO_DARK = '1';
  else delete env.AGIO_DEMO_DARK;
  if (detail) env.AGIO_DEMO_DETAIL = '1';
  else delete env.AGIO_DEMO_DETAIL;

  const child = spawn(binary, { cwd: repo, env, stdio: 'ignore' });

  try {
    let id = '';
    for (let attempt = 0; attempt < 80 && !id; attempt++) {
      await sleep(100);
      if (child.exitCode !== null) throw new Error('The demo exited before its window opened.');
      id = windowID();
    }
    if (!id) throw new Error('The demo window did not appear.');

    if (detail) {
      osascript([
        'tell application "System Events"',
        '  set frontmost of process "agio-demo" to true',
        'end tell',
      ]);
      await sleep(350);
      // A single-turn demo: open the fold row, then its activity group.
      clickButton('Worked for');
      await sleep(450);
      clickButton('Ran ');
      await sleep(700);
    }

    mkdirSync(path.dirname(output), { recursive: true });
    execFileSync('screencapture', ['-x', '-o', `-l${id}`, output], { stdio: 'inherit' });
    console.log(`wrote ${path.relative(website, output)}`);
  } finally {
    child.kill('SIGTERM');
    await Promise.race([
      new Promise((resolve) => child.once('exit', resolve)),
      sleep(2000),
    ]);
  }
}

console.log('Building the demo…');
mkdirSync(scratch, { recursive: true });
mkdirSync(screens, { recursive: true });
execFileSync('go', ['build', '-o', binary, './website/demo'], { cwd: repo, stdio: 'inherit' });

await capture({ output: path.join(screens, 'app-light.png') });
await capture({ dark: true, output: path.join(screens, 'app-dark.png') });
await capture({ detail: true, output: path.join(screens, 'app-detail-light.png') });
await capture({ dark: true, detail: true, output: path.join(screens, 'app-detail-dark.png') });
