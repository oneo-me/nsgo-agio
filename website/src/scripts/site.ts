// Appearance, language choice, copy buttons and tabs: the page's only
// client-side behavior. The head script has already applied the stored
// appearance and sent a first visit to its language.

const themes = ['light', 'dark', 'system'] as const;
type Theme = (typeof themes)[number];

const themeButtons = [...document.querySelectorAll<HTMLButtonElement>('[data-theme-value]')];
const darkMedia = window.matchMedia('(prefers-color-scheme: dark)');

function storedTheme(): Theme {
  try {
    const value = localStorage.getItem('agio-theme');
    if (value === 'light' || value === 'dark' || value === 'system') return value;
  } catch {
    // Storage can be unavailable; the system still works.
  }
  return 'system';
}

function applyTheme(theme: Theme, persist = true) {
  const dark = theme === 'dark' || (theme === 'system' && darkMedia.matches);
  const root = document.documentElement;
  root.dataset.theme = dark ? 'dark' : 'light';
  root.dataset.themePreference = theme;
  root.style.colorScheme = dark ? 'dark' : 'light';

  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', dark ? '#08090c' : '#fbfbfd');
  for (const button of themeButtons) {
    button.setAttribute('aria-pressed', String(button.dataset.themeValue === theme));
  }
  if (persist) {
    try {
      localStorage.setItem('agio-theme', theme);
    } catch {
      // The choice lasts this page.
    }
  }
}

applyTheme(storedTheme(), false);
for (const button of themeButtons) {
  button.addEventListener('click', () => {
    const value = button.dataset.themeValue;
    applyTheme(themes.includes(value as Theme) ? (value as Theme) : 'system');
  });
}
darkMedia.addEventListener('change', () => {
  if (storedTheme() === 'system') applyTheme('system', false);
});

for (const link of document.querySelectorAll<HTMLAnchorElement>('[data-lang-switch]')) {
  link.addEventListener('click', () => {
    try {
      localStorage.setItem('agio-lang', link.dataset.langSwitch ?? '');
    } catch {
      // The switch still navigates.
    }
  });
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const area = document.createElement('textarea');
    area.value = text;
    area.setAttribute('readonly', '');
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.append(area);
    area.select();
    const copied = document.execCommand('copy');
    area.remove();
    return copied;
  }
}

document.addEventListener('click', async (event) => {
  const target = event.target instanceof Element ? event.target : null;
  const button = target?.closest<HTMLButtonElement>('[data-copy]');
  if (!button) return;

  const text = button.dataset.copy ?? '';
  if (!(await copyText(text))) return;

  const label = button.getAttribute('aria-label');
  button.dataset.copied = '';
  button.setAttribute('aria-label', document.documentElement.lang.startsWith('zh') ? '已复制' : 'Copied');
  window.setTimeout(() => {
    delete button.dataset.copied;
    if (label) button.setAttribute('aria-label', label);
  }, 1600);
});

for (const root of document.querySelectorAll<HTMLElement>('[data-tabs]')) {
  const tabs = [...root.querySelectorAll<HTMLButtonElement>('[role="tab"]')];
  const panels = [...root.querySelectorAll<HTMLElement>('[role="tabpanel"]')];

  const select = (tab: HTMLButtonElement, focus = false) => {
    for (const other of tabs) {
      const current = other === tab;
      other.setAttribute('aria-selected', String(current));
      other.tabIndex = current ? 0 : -1;
      if (current && focus) other.focus();
    }
    for (const panel of panels) {
      panel.hidden = panel.id !== tab.getAttribute('aria-controls');
    }
  };

  tabs.forEach((tab, index) => {
    tab.addEventListener('click', () => select(tab));
    tab.addEventListener('keydown', (event) => {
      let next = index;
      if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
      else if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length;
      else if (event.key === 'Home') next = 0;
      else if (event.key === 'End') next = tabs.length - 1;
      else return;
      event.preventDefault();
      select(tabs[next], true);
    });
  });
}
