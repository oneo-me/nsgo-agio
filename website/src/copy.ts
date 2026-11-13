// The page's words, in English and Simplified Chinese. Short, specific and
// calm; the product is a library, so every claim should be something the
// code does. Chinese headings, titles and kickers omit the trailing 。;
// body sentences keep it.

import type { Lang } from './site';
import type { Led, Tone } from './template';

type Feature = { led: Led; title: string; body: string };
type Note = { led: Led; title: string; body: string };
type Column = { value: string; title: string; body: string };
type Tab = { id: string; label: string; title: string; body: string; command: string; code: string };

const en = {
  meta: {
    title: 'Agio — coding agents, in one calm stream',
    description:
      'The agent core of Neorio: run pi, Codex, Claude Code and CodeBuddy Code behind one neutral API, and show their conversations with NSGo — in one process, or as a server and its clients.',
    ogAlt: 'Agio — coding agents, in one calm stream',
  },

  nav: {
    skip: 'Skip to content',
    primary: 'Primary',
    home: 'Agio home',
    links: [
      { label: 'Features', href: '#features' },
      { label: 'How it works', href: '#how' },
      { label: 'Under the hood', href: '#under' },
    ],
    github: 'GitHub',
    cta: 'Get started',
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
    badge: { tag: 'New', text: 'Serve agents over TCP, a pipe or SSH' },
    title: 'Coding agents.\nIn one calm stream.',
    lede: 'Agio runs pi, Codex, Claude Code and CodeBuddy Code behind one neutral API, and shows their conversations in a native window — here, or across the wire.',
    primary: 'Get started',
    secondary: 'View on GitHub',
    fine: 'Free and open source · MIT · Go 1.27',
    alt: 'Agio’s window: a conversation with two turns beside a list of conversations',
    caption: 'Every conversation, its turns, and where they run.',
    tone: 'blue' as Tone,
  },

  agents: {
    title: 'Works with the agents you already use',
    names: ['pi', 'Codex', 'Claude Code', 'CodeBuddy Code'],
    note: 'and any Agent Client Protocol CLI',
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
        body: 'host.Host runs agents in this process. wire serves the same backend over TCP, a pipe or SSH, and the view cannot tell the difference.',
      },
      {
        led: 'amber',
        title: 'The window is included',
        body: 'chatview.App brings conversations, a sidebar, a composer, model and reasoning choices, settings and each account’s limits.',
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
    caption: 'Worked for 4s, opened: thinking, an edit, a command, and the answer.',
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
    title: 'A working window in a few lines.',
    lead: 'Install the module, pick where agents run, and hand the backend to a view.',
    requirements: 'Needs Go 1.27 and the agent CLIs you want on the login shell’s PATH. They run with your own sign-ins.',
    readme: 'Read the README',
    examples: 'Run the examples',
  },

  built: {
    title: 'Built with NSGo',
    body: 'Agio’s views are NSGo components: native Go UI, written once and shown in English and Chinese. The core below them is plain, headless Go.',
    nsgo: 'nsgo.dev',
    nsgoRepo: 'GitHub',
  },

  footer: {
    note: 'The agent core of Neorio.',
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
    title: 'Agio — 编程智能体，汇入一条平静的流',
    description:
      'Neorio 的智能体核心：pi、Codex、Claude Code 和 CodeBuddy Code 共用一套中立的 API，用 NSGo 展示它们的对话——在同一个进程里，或作为服务端和它的客户端。',
    ogAlt: 'Agio — 编程智能体，汇入一条平静的流',
  },

  nav: {
    skip: '跳到内容',
    primary: '主导航',
    home: 'Agio 首页',
    links: [
      { label: '功能', href: '#features' },
      { label: '工作方式', href: '#how' },
      { label: '技术细节', href: '#under' },
    ],
    github: 'GitHub',
    cta: '开始使用',
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
    badge: { tag: '新', text: '通过 TCP、管道或 SSH 提供智能体' },
    title: '编程智能体\n汇入一条平静的流',
    lede: 'Agio 用一套中立的 API 运行 pi、Codex、Claude Code 和 CodeBuddy Code，并在原生窗口里展示它们的对话——就在本机，或隔着一条线。',
    primary: '开始使用',
    secondary: '在 GitHub 查看',
    fine: '免费开源 · MIT · Go 1.27',
    alt: 'Agio 的窗口：一个有两个轮次的对话，旁边是对话列表',
    caption: '每个对话、它的轮次，以及它们在哪里运行。',
    tone: 'blue',
  },

  agents: {
    title: '接上你已经在用的智能体',
    names: ['pi', 'Codex', 'Claude Code', 'CodeBuddy Code'],
    note: '以及任何 Agent Client Protocol CLI',
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
        body: 'host.Host 在本进程运行智能体。wire 通过 TCP、管道或 SSH 提供同一个后端，视图看不出区别。',
      },
      {
        led: 'amber',
        title: '窗口也是现成的',
        body: 'chatview.App 带来对话、侧边栏、输入框、模型与推理档位、设置，以及每个账户的限额。',
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
    caption: '工作了 4 秒，已展开：思考、一次编辑、一条命令，以及回答。',
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
    title: '几行代码，一个能用的窗口',
    lead: '安装模块，选择智能体在哪里运行，然后把后端交给视图。',
    requirements: '需要 Go 1.27，以及登录 shell 的 PATH 中你想用的智能体 CLI。它们使用你自己的登录状态运行。',
    readme: '阅读 README',
    examples: '运行示例',
  },

  built: {
    title: '用 NSGo 构建',
    body: 'Agio 的视图是 NSGo 组件：原生的 Go 界面，写一次，以英文和中文展示。它们之下的核心是纯粹的无头 Go。',
    nsgo: 'nsgo.dev',
    nsgoRepo: 'GitHub',
  },

  footer: {
    note: 'Neorio 的智能体核心。',
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
