// The page's words, in English and Simplified Chinese. Short, specific and
// calm; Agio is a library first and an app second, and every claim should
// be something the code does. Chinese headings, titles and kickers omit the trailing 。;
// body sentences keep it.

import type { IconName } from './icons';
import type { Lang } from './site';
import type { Led, Tone } from './template';

type Feature = { led: Led; title: string; body: string };
type Card = { icon: IconName; kicker: string; title: string; body: string; link: string; href: string };
type Note = { led: Led; title: string; body: string };
type Column = { value: string; title: string; body: string };
type Tab = { id: string; label: string; title: string; body: string; command: string; code: string };

const en = {
  meta: {
    title: 'Agio — coding agents, inside your app',
    description:
      'Agio is the agent core of NSGo apps: it runs pi, Codex, Claude Code and CodeBuddy Code and shows their work on phones and desktops. Agio, the app, is that core on its own, for macOS, Windows and Linux.',
    ogAlt: 'Agio — coding agents, inside your app',
  },

  nav: {
    skip: 'Skip to content',
    primary: 'Primary',
    home: 'Agio home',
    links: [
      { label: 'What it is', href: '#what' },
      { label: 'The app', href: '#app' },
      { label: 'The library', href: '#features' },
      { label: 'How it works', href: '#how' },
    ],
    github: 'GitHub',
    cta: 'Download',
    menu: 'Open menu',
    close: 'Close menu',
    menuLabel: 'Menu',
  },

  theme: { label: 'Appearance', light: 'Light', dark: 'Dark', system: 'System' },
  language: { label: 'Language', en: 'English', zh: '中文' },
  code: { copy: 'Copy code', command: 'Copy command' },

  lightbox: {
    label: 'Screenshot',
    enlarge: 'Enlarge',
    close: 'Close',
    previous: 'Previous',
    next: 'Next',
  },

  hero: {
    badge: { tag: 'New', text: 'Agio for macOS, Windows and Linux' },
    title: 'Coding agents.\nInside your app.',
    lede: 'Agio is the agent core of NSGo apps: it runs pi, Codex, Claude Code and CodeBuddy Code, and shows their work on phones and desktops. Agio, the app, is that core on its own — to try, to test, or simply to use.',
    primary: 'Download Agio',
    secondary: 'View on GitHub',
    fine: 'Free and open source · MIT · Updates itself',
    alt: 'Agio’s window: a task with two turns beside the sidebar’s tasks, running ones first',
    caption: 'Tasks, their turns, and the project each works in.',
    tone: 'blue' as Tone,
  },

  agents: {
    title: 'Works with the agents you already use',
    names: ['pi', 'Codex', 'Claude Code', 'CodeBuddy Code'],
    note: 'and any Agent Client Protocol CLI',
  },

  what: {
    title: 'One core. Two ways to use it.',
    lead: 'Most of Agio is a Go module other NSGo apps build their development environments on. The rest is an app that shows what it can do.',
    cards: [
      {
        icon: 'layers',
        kicker: 'Library',
        title: 'The agent core of NSGo apps',
        body: 'Bring the agents you use into your own app: drivers, a host, a wire protocol and ready views behind one neutral API, with screens for phones and desktops and agents on every desktop system.',
        link: 'Build with it',
        href: '#features',
      },
      {
        icon: 'window',
        kicker: 'App',
        title: 'Agio, the app',
        body: 'The same core in a window of its own: a demo of what Agio does, a place to test it, and a simple tool for working with agents. Released on GitHub, and it updates itself.',
        link: 'Download',
        href: '#download',
      },
    ] satisfies Card[],
  },

  app: {
    title: 'Agio, the app.',
    lead: 'Start a task, pick the project and the model, and follow the agent as it works. Everything here comes from the library; the app adds the rest of a desktop tool.',
    alt: 'Agio’s New task: the app’s icon over “What should we work on in neorio?” and a centered composer',
    caption: 'New task asks which project to work in.',
    tone: 'green' as Tone,
    items: [
      {
        led: 'blue',
        title: 'Tasks, not chats',
        body: 'New task asks which project to work in. Running tasks lead the sidebar, the rest follow by when they were last active, and all of them come back after a restart.',
      },
      {
        led: 'violet',
        title: 'The agents you already have',
        body: 'Agio finds the agent CLIs on your login shell’s PATH and runs them with your own sign-ins. Turn agents and models on and off in the settings.',
      },
      {
        led: 'green',
        title: 'Your project, one click away',
        body: 'The title bar opens a task’s folder in Finder, your terminal, your editor or your Git client.',
      },
      {
        led: 'amber',
        title: 'Made to look like yours',
        body: 'Color schemes for light and dark over a translucent window, an optional grain, in English or Chinese.',
      },
      {
        led: 'teal',
        title: 'It tells you when it’s done',
        body: 'A sound and a notification when an agent stops, and the computer kept awake while agents work.',
      },
      {
        led: 'pink',
        title: 'Up to date by itself',
        body: 'Signed releases on GitHub. Agio installs them in place on macOS and Windows, and opens the release on Linux.',
      },
    ] satisfies Feature[],
  },

  features: {
    title: 'The parts between a CLI and your window.',
    lead: 'Drivers translate, a host folds, and a view shows. Each layer only knows the one below it.',
    items: [
      {
        led: 'blue',
        title: 'Four drivers, one vocabulary',
        body: 'Each agent speaks its own protocol. Agio turns them into the same events, turns and statuses; nothing above the drivers knows which one runs.',
      },
      {
        led: 'green',
        title: 'Turns you can read',
        body: 'Thinking, tool calls, streamed markdown and the time a turn took fold into a conversation. A running turn stays open; a finished one steps aside.',
      },
      {
        led: 'violet',
        title: 'Here, or across the wire',
        body: 'host.Host runs agents on macOS, Windows or Linux. wire serves the same backend over TCP, a pipe or SSH, so a phone can show the agents a computer runs.',
      },
      {
        led: 'amber',
        title: 'Views for phones and desktops',
        body: 'chatview.App brings tasks, a sidebar, a composer, model and reasoning choices, settings and each account’s limits, built with NSGo.',
      },
      {
        led: 'teal',
        title: 'A server without a UI',
        body: 'Everything below chatview is headless. A server builds without NSGo, and views reach agents only through agio.Backend.',
      },
      {
        led: 'pink',
        title: 'Nothing kept behind your back',
        body: 'Agio persists nothing; your app keeps a conversation’s cursor, folder and model. Keep a server private and give it a token.',
      },
    ] satisfies Feature[],
  },

  showcase: {
    title: 'A transcript, not a log.',
    lead: 'Open a finished turn and read what happened: the reasoning, the files it opened, the commands it ran, the answer that streamed in.',
    alt: 'An Agio turn opened: thinking, an edit, a command, and a markdown answer',
    caption: 'Worked for 4s, opened: thinking, an edit and a command, then the answer.',
    tone: 'violet' as Tone,
    notes: [
      { led: 'violet', title: 'Thinking', body: 'Reasoning stays a step, not a wall of text.' },
      { led: 'amber', title: 'Tools', body: 'Each call keeps its target and its output.' },
      { led: 'green', title: 'Answer', body: 'The reply streams in as markdown.' },
    ] satisfies Note[],
  },

  how: {
    title: 'One contract. Two places to run.',
    lead: 'A view shows an agio.Backend. A host is one; a wire client is another. Swap it, and the same window shows local or remote agents.',
    tabsLabel: 'Where agents run',
    tabs: [
      {
        id: 'single',
        label: 'In this process',
        title: 'main.go',
        body: 'host.Host starts the agent CLIs on this machine and hands their conversations straight to the view.',
        command: 'go tool nsgo dev cmd/agio',
        code: `h := host.New(host.Options{Dir: workDir})

nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, h)}
}, nsgo.WindowSize(1040, 720))

h.Close()`,
      },
      {
        id: 'server',
        label: 'As a server',
        title: 'server.go',
        body: 'wire serves the same host over TCP with a token. It builds without NSGo, so it runs anywhere Go does.',
        command: 'go run ./examples/server -token secret',
        code: `h := host.New(host.Options{Dir: workDir})
defer h.Close()

log.Fatal(wire.ListenAndServe(h, "127.0.0.1:7788", token))`,
      },
      {
        id: 'client',
        label: 'As a client',
        title: 'client.go',
        body: 'wire.Client is a backend too. Hand it to the same chatview.App and the agents run somewhere else.',
        command: 'AGIO_TOKEN=secret go tool nsgo dev examples/client',
        code: `c, err := wire.Dial("127.0.0.1:7788", token)
if err != nil {
	log.Fatal(err)
}
defer c.Close()

nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, c)}
})`,
      },
    ] satisfies Tab[],
    link: 'Read the architecture',
  },

  under: {
    title: 'Built for the wire.',
    lead: 'Remote conversations should read live and cost little.',
    items: [
      {
        value: '30/s',
        title: 'Published, not pushed',
        body: 'A served session coalesces changes and publishes at most about 30 times a second, so a reply reads live without flooding a slow link.',
      },
      {
        value: 'patches',
        title: 'Only what changed',
        body: 'Turns travel as transcript patches. Streamed text sends only what arrived, not the whole conversation again.',
      },
      {
        value: 'groups',
        title: 'Only its own processes',
        body: 'Agents start in their own process group. Stopping one stops what it started, and a host signals only what it launched.',
      },
    ] satisfies Column[],
  },

  start: {
    title: 'Build your own on Agio.',
    lead: 'Install the module, pick where agents run, and hand the backend to a view.',
    requirements: 'Needs Go 1.27 and the agent CLIs you want on the login shell’s PATH. They run with your own sign-ins.',
    readme: 'Read the README',
    examples: 'Run the examples',
  },

  download: {
    title: 'Download Agio.',
    lead: 'For macOS, Windows and Linux, on arm64 and x64. Agio checks GitHub for new releases and updates itself.',
    button: 'Download from GitHub',
    notes: 'Release notes',
    platforms: ['macOS', 'Windows', 'Linux'],
    fine: 'Agio runs the agent CLIs on your login shell’s PATH with your own sign-ins. It sends anonymous usage statistics, never prompts or folders, and its settings turn them off.',
  },

  built: {
    title: 'Built with NSGo',
    body: 'Agio’s views are NSGo components: native Go UI for phones and desktops, written once and shown in English and Chinese. The core below them is plain, headless Go.',
    nsgo: 'nsgo.dev',
    nsgoRepo: 'GitHub',
  },

  footer: {
    note: 'The agent core of NSGo apps.',
    links: 'Project',
    readme: 'README',
    changelog: 'Changelog',
    issues: 'Issues',
    license: 'MIT License',
  },

  notFound: {
    title: 'Page not found — Agio',
    heading: 'This page wandered off.',
    body: 'The address is gone, or it never existed. Agio lives on the home page.',
    back: 'Back to Agio',
  },
};

const zh: typeof en = {
  meta: {
    title: 'Agio — 把编程智能体放进你的应用',
    description:
      'Agio 是 NSGo 应用的智能体核心：运行 pi、Codex、Claude Code 和 CodeBuddy Code，并在手机与桌面上展示它们的工作。Agio 应用就是这个核心本身，支持 macOS、Windows 和 Linux。',
    ogAlt: 'Agio — 把编程智能体放进你的应用',
  },

  nav: {
    skip: '跳到内容',
    primary: '主导航',
    home: 'Agio 首页',
    links: [
      { label: '是什么', href: '#what' },
      { label: '应用', href: '#app' },
      { label: '库', href: '#features' },
      { label: '工作方式', href: '#how' },
    ],
    github: 'GitHub',
    cta: '下载',
    menu: '打开菜单',
    close: '关闭菜单',
    menuLabel: '菜单',
  },

  theme: { label: '外观', light: '浅色', dark: '深色', system: '跟随系统' },
  language: { label: '语言', en: 'English', zh: '中文' },
  code: { copy: '复制代码', command: '复制命令' },

  lightbox: {
    label: '截图',
    enlarge: '放大',
    close: '关闭',
    previous: '上一张',
    next: '下一张',
  },

  hero: {
    badge: { tag: '新', text: 'Agio 支持 macOS、Windows 和 Linux' },
    title: '编程智能体\n放进你的应用',
    lede: 'Agio 是 NSGo 应用的智能体核心：运行 pi、Codex、Claude Code 和 CodeBuddy Code，并在手机与桌面上展示它们的工作。Agio 应用就是这个核心本身——拿来试用、测试，或者直接当工具用。',
    primary: '下载 Agio',
    secondary: '在 GitHub 查看',
    fine: '免费开源 · MIT · 自动更新',
    alt: 'Agio 的窗口：一个有两个轮次的任务，旁边是侧边栏的任务，进行中的排在最前',
    caption: '任务、它们的轮次，以及各自所在的项目。',
    tone: 'blue',
  },

  agents: {
    title: '接上你已经在用的智能体',
    names: ['pi', 'Codex', 'Claude Code', 'CodeBuddy Code'],
    note: '以及任何 Agent Client Protocol CLI',
  },

  what: {
    title: '一个核心，两种用法',
    lead: 'Agio 的主体是一个 Go 模块，其他 NSGo 应用在它之上搭建自己的开发环境；其余部分是一个展示它能做什么的应用。',
    cards: [
      {
        icon: 'layers',
        kicker: '库',
        title: 'NSGo 应用的智能体核心',
        body: '把你在用的智能体接进自己的应用：驱动、host、wire 协议和现成的视图共用一套中立的 API，界面适配手机与桌面，智能体运行在各大桌面系统上。',
        link: '用它构建',
        href: '#features',
      },
      {
        icon: 'window',
        kicker: '应用',
        title: 'Agio 应用',
        body: '同一个核心，装进它自己的窗口：用来演示 Agio 能做什么、测试它的功能，也是一个与智能体协作的简单工具。在 GitHub 发布，并会自动更新。',
        link: '下载',
        href: '#download',
      },
    ],
  },

  app: {
    title: 'Agio 应用',
    lead: '新建任务，选好项目和模型，看着智能体一步步完成。这里的一切都来自库，应用补上了桌面工具的其余部分。',
    alt: 'Agio 的新建任务：应用图标下是“要在 neorio 中处理什么任务？”和居中的输入框',
    caption: '新建任务时先问在哪个项目里做。',
    tone: 'green',
    items: [
      {
        led: 'blue',
        title: '是任务，不只是聊天',
        body: '新建任务时先选项目。进行中的任务排在侧边栏最前，其余按最近活动排列，重启后全部恢复。',
      },
      {
        led: 'violet',
        title: '用你已有的智能体',
        body: 'Agio 在登录 shell 的 PATH 中找到智能体 CLI，用你自己的登录状态运行。在设置中开关智能体和模型。',
      },
      {
        led: 'green',
        title: '一键打开项目',
        body: '标题栏可以在访达、终端、编辑器或 Git 客户端中打开任务所在的文件夹。',
      },
      {
        led: 'amber',
        title: '照你的喜好来',
        body: '浅色与深色各有配色方案，铺在半透明的窗口上，可选颗粒质感，界面支持中文和英文。',
      },
      {
        led: 'teal',
        title: '做完了会告诉你',
        body: '智能体停下时播放声音并发送通知，智能体工作时保持电脑唤醒。',
      },
      {
        led: 'pink',
        title: '自己保持最新',
        body: '在 GitHub 发布签名的版本。macOS 和 Windows 上就地安装更新，Linux 上打开发布页面。',
      },
    ],
  },

  features: {
    title: 'CLI 与你的窗口之间的那些部分',
    lead: '驱动负责翻译，host 负责折叠，视图负责展示。每一层只认识下一层。',
    items: [
      {
        led: 'blue',
        title: '四个驱动，一套词汇',
        body: '每个智能体各说各的协议。Agio 把它们变成同一套事件、轮次和状态；驱动之上，没有人知道正在运行哪一个。',
      },
      {
        led: 'green',
        title: '读得懂的轮次',
        body: '思考、工具调用、流式 Markdown 和一轮所花的时间，折叠成一段对话。运行中的轮次保持展开，结束的轮次让到一边。',
      },
      {
        led: 'violet',
        title: '就在这里，或隔着一条线',
        body: 'host.Host 在 macOS、Windows 或 Linux 上运行智能体。wire 通过 TCP、管道或 SSH 提供同一个后端，手机也能展示电脑上运行的智能体。',
      },
      {
        led: 'amber',
        title: '手机与桌面的视图',
        body: 'chatview.App 带来任务、侧边栏、输入框、模型与推理档位、设置，以及每个账户的限额，用 NSGo 构建。',
      },
      {
        led: 'teal',
        title: '没有 UI 的服务端',
        body: 'chatview 以下都是无头的。服务端不依赖 NSGo 就能构建，视图只通过 agio.Backend 接触智能体。',
      },
      {
        led: 'pink',
        title: '不在背后留下任何东西',
        body: 'Agio 不持久化任何东西；你的应用保存对话的 cursor、目录和模型。把服务端放在私有网络，并给它一个令牌。',
      },
    ],
  },

  showcase: {
    title: '一份记录，不是日志',
    lead: '展开一个结束的轮次，读一读发生了什么：推理、打开的文件、运行的命令、流式写出的回答。',
    alt: '展开的 Agio 轮次：思考、一次编辑、一条命令，以及 Markdown 回答',
    caption: '用时 4 秒，已展开：思考、一次编辑和一条命令，然后是回答。',
    tone: 'violet',
    notes: [
      { led: 'violet', title: '思考', body: '推理是一个步骤，不是一堵文字墙。' },
      { led: 'amber', title: '工具', body: '每次调用都保留目标和输出。' },
      { led: 'green', title: '回答', body: '回复以 Markdown 流式写出。' },
    ],
  },

  how: {
    title: '一个契约，两个运行地点',
    lead: '视图展示 agio.Backend。host 是一个，wire 客户端是另一个。换掉它，同一个窗口就能展示本机或远程的智能体。',
    tabsLabel: '智能体在哪里运行',
    tabs: [
      {
        id: 'single',
        label: '在本进程',
        title: 'main.go',
        body: 'host.Host 在本机启动智能体 CLI，把它们的对话直接交给视图。',
        command: 'go tool nsgo dev cmd/agio',
        code: en.how.tabs[0].code,
      },
      {
        id: 'server',
        label: '作为服务端',
        title: 'server.go',
        body: 'wire 用令牌通过 TCP 提供同一个 host。它不依赖 NSGo，Go 能跑的地方它都能跑。',
        command: 'go run ./examples/server -token secret',
        code: en.how.tabs[1].code,
      },
      {
        id: 'client',
        label: '作为客户端',
        title: 'client.go',
        body: 'wire.Client 也是一个后端。把它交给同一个 chatview.App，智能体就在别处运行。',
        command: 'AGIO_TOKEN=secret go tool nsgo dev examples/client',
        code: en.how.tabs[2].code,
      },
    ],
    link: '阅读架构',
  },

  under: {
    title: '为线路而生',
    lead: '远程对话应该读起来是实时的，而且花费很少。',
    items: [
      {
        value: '30/s',
        title: '发布，而不是推送',
        body: '服务端上的会话会合并变化，每秒最多发布约 30 次，回复读起来是实时的，又不会淹没慢连接。',
      },
      {
        value: 'patches',
        title: '只发送变化',
        body: '轮次以补丁的形式传输。流式文本只发送新到达的部分，而不是整段对话重来。',
      },
      {
        value: 'groups',
        title: '只碰自己的进程',
        body: '智能体在自己的进程组中启动。停掉一个，就停掉它启动的东西；host 只给自己启动的进程发信号。',
      },
    ],
  },

  start: {
    title: '在 Agio 之上构建你的应用',
    lead: '安装模块，选择智能体在哪里运行，然后把后端交给视图。',
    requirements: '需要 Go 1.27，以及登录 shell 的 PATH 中你想用的智能体 CLI。它们使用你自己的登录状态运行。',
    readme: '阅读 README',
    examples: '运行示例',
  },

  download: {
    title: '下载 Agio',
    lead: '支持 macOS、Windows 和 Linux，arm64 与 x64。Agio 会检查 GitHub 上的新版本并自动更新。',
    button: '从 GitHub 下载',
    notes: '更新日志',
    platforms: ['macOS', 'Windows', 'Linux'],
    fine: 'Agio 用你自己的登录状态运行登录 shell 的 PATH 中的智能体 CLI。它会发送匿名使用统计，从不包含提示词或文件夹，可在设置中关闭。',
  },

  built: {
    title: '用 NSGo 构建',
    body: 'Agio 的视图是 NSGo 组件：面向手机与桌面的原生 Go 界面，写一次，以英文和中文展示。它们之下的核心是纯粹的无头 Go。',
    nsgo: 'nsgo.dev',
    nsgoRepo: 'GitHub',
  },

  footer: {
    note: 'NSGo 应用的智能体核心。',
    links: '项目',
    readme: 'README',
    changelog: '更新日志',
    issues: '问题反馈',
    license: 'MIT 许可证',
  },

  notFound: {
    title: '页面未找到 — Agio',
    heading: '这个页面走丢了',
    body: '地址不在了，或者从未存在。Agio 在首页。',
    back: '回到 Agio',
  },
};

export const copies: Record<Lang, typeof en> = { en, zh };

export type Copy = typeof en;
