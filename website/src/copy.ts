// The page's words, in English and Simplified Chinese. Short, specific and
// calm; the product is a library, so every claim should be something the
// code does. Chinese headings, titles and kickers omit the trailing 。;
// body sentences keep it.

import type { IconName } from './icons';
import type { Lang } from './site';

type FeatureCopy = {
  icon: IconName;
  accent: 'blue' | 'green' | 'violet' | 'amber' | 'cyan' | 'coral';
  title: string;
  body: string;
};

type TabCopy = {
  id: string;
  label: string;
  title: string;
  command: string;
  code: string;
};

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
    features: 'Features',
    how: 'How it works',
    start: 'Quick start',
    github: 'GitHub',
    getStarted: 'Get started',
    preview: 'preview',
    menu: 'Open menu',
    close: 'Close menu',
    menuLabel: 'Menu',
  },

  theme: {
    label: 'Appearance',
    light: 'Light',
    dark: 'Dark',
    system: 'System',
  },

  language: {
    label: 'Language',
    en: 'English',
    zh: '中文',
  },

  code: {
    copy: 'Copy code',
    command: 'Copy command',
  },

  hero: {
    title: 'Agio',
    lede: 'Coding agents, in one calm stream.',
    body: 'The agent core of Neorio: four coding agents behind one neutral API, with NSGo conversations — here or across the wire.',
    primary: 'Get started',
    secondary: 'GitHub',
    installLabel: 'Install with Go',
    fine: ['MIT', 'Go 1.27', 'Your agents, your sign-ins'],
    glanceLabel: 'At a glance',
    alt: 'Agio’s window: a conversation with two turns beside a list of conversations',
    caption: 'chatview.App — every conversation, its turns, and where they run.',
  },

  agents: {
    label: 'Supported agents',
    title: 'One stream for the agents you already use',
    names: ['pi', 'Codex', 'Claude Code', 'CodeBuddy Code'],
    note: 'and any Agent Client Protocol CLI',
  },

  features: {
    eyebrow: 'Why Agio',
    title: 'The parts between a CLI and your window.',
    lead: 'Agio is small on purpose: drivers translate, a host folds, and a view shows. Each layer only knows the one below it.',
    items: [
      {
        icon: 'terminal',
        accent: 'blue',
        title: 'Four drivers, one vocabulary',
        body: 'pi, Codex, Claude Code and CodeBuddy Code each speak their own protocol. Agio turns them into the same events, turns and statuses; nothing above the drivers knows which one is running.',
      },
      {
        icon: 'messages',
        accent: 'green',
        title: 'A transcript, not a log',
        body: 'Thinking, tool calls, streamed markdown and the time a turn took fold into a conversation you can read. A running turn stays open; a finished one gets out of the way.',
      },
      {
        icon: 'server',
        accent: 'violet',
        title: 'Here, or across the wire',
        body: 'host.Host runs agents in this process. wire serves the same backend over TCP, a pipe or an SSH session — and the view cannot tell where the agents run.',
      },
      {
        icon: 'window',
        accent: 'amber',
        title: 'The window is included',
        body: 'chatview.App gives you conversations, a sidebar, a composer, model and reasoning choices, agent and model settings, and each account’s limits. chatview.Conversation is one conversation for a layout of your own.',
      },
      {
        icon: 'layers',
        accent: 'cyan',
        title: 'A server without a UI',
        body: 'Everything below chatview is headless. A server builds without NSGo, drivers speak only neutral types, and views reach agents only through agio.Backend.',
      },
      {
        icon: 'shield',
        accent: 'coral',
        title: 'Nothing kept behind your back',
        body: 'Agio persists nothing. Your app stores a conversation’s cursor, directory and model. Agents work with full access, so keep a server private and give it a token.',
      },
    ] satisfies FeatureCopy[],
  },

  how: {
    eyebrow: 'The shape',
    title: 'One contract. Two places to run.',
    lead: 'A view shows an agio.Backend. A host is one; a wire client is another. That is the whole trick.',
    diagramLabel: 'Agio’s architecture',
    viewLabel: 'your view',
    viewNode: 'chatview.App / Conversation',
    contract: 'agio.Backend',
    localLabel: 'in this process',
    localNode: 'host.Host',
    remoteLabel: 'somewhere else',
    remoteClient: 'wire.Client',
    remoteServer: 'wire.Serve',
    driversLabel: 'drivers',
    drivers: 'pi · Codex · Claude Code · CodeBuddy Code',
    note: 'Swap the backend and the same view shows local or remote agents. The server example serves this host over TCP; the client passes a wire.Client to the same chatview.App.',
    codeTitle: 'Single process',
    link: 'Read the architecture',
  },

  turns: {
    eyebrow: 'The transcript',
    title: 'A transcript, not a log.',
    lead: 'Open a finished turn and read what happened: the reasoning, the files it opened, the commands it ran, the answer that streamed in.',
    alt: 'An Agio turn opened: thinking, an edit, a command, and a markdown answer',
    caption: 'Worked for 4s, opened: thinking, an edit, a command, and the answer.',
    notes: [
      { label: 'Thinking', body: 'Reasoning stays a step, not a wall of text.' },
      { label: 'Tools', body: 'Each call keeps its target and output.' },
      { label: 'Answer', body: 'The final reply streams in markdown.' },
    ],
  },

  under: {
    eyebrow: 'Under the hood',
    title: 'Built for the wire.',
    lead: 'Remote conversations should read live and cost little. The design follows from that.',
    items: [
      {
        value: '30/s',
        title: 'Published, not pushed',
        body: 'A served session coalesces changes and publishes at most about 30 times a second, so a streamed reply reads live without flooding a slow link.',
      },
      {
        value: 'patches',
        title: 'Only what changed',
        body: 'Each conversation’s turns travel as transcript patches. Streamed text sends only what arrived, not the whole conversation again.',
      },
      {
        value: 'groups',
        title: 'Only its own processes',
        body: 'Agents start in their own process group. Stopping one stops what it started, and a host signals only the processes it launched.',
      },
    ],
  },

  start: {
    eyebrow: 'Quick start',
    title: 'A working window in a few lines.',
    lead: 'Install the module, pick where agents run, and hand the backend to a view.',
    requirements:
      'Needs Go 1.27 and the agent CLIs you want on the login shell’s PATH. They run with your own sign-ins.',
    installLabel: 'Install the module',
    tabsLabel: 'Examples',
    tabs: [
      {
        id: 'single',
        label: 'Single process',
        title: 'main.go',
        command: 'go tool nsgo dev examples/single',
        code: `h := host.New(host.Options{Dir: workDir})

nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, h)}
}, nsgo.WindowSize(1040, 720))

h.Close()`,
      },
      {
        id: 'server',
        label: 'Server',
        title: 'server.go',
        command: 'go run ./examples/server -token secret',
        code: `h := host.New(host.Options{Dir: workDir})
defer h.Close()

log.Fatal(wire.ListenAndServe(h, "127.0.0.1:7788", token))`,
      },
      {
        id: 'client',
        label: 'Client',
        title: 'client.go',
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
    ] satisfies TabCopy[],
    readme: 'Read the README',
    examples: 'Run the examples',
  },

  built: {
    eyebrow: 'Built with',
    title: 'Go and NSGo.',
    body: 'The core is headless Go. The views are NSGo components, written once and shown in English and Chinese. This site is Astro and Tailwind, built to static files for GitHub Pages.',
    nsgo: 'nsgo.dev',
    nsgoRepo: 'NSGo on GitHub',
  },

  footer: {
    note: 'The agent core of Neorio. MIT licensed.',
    project: 'Project',
    elsewhere: 'Elsewhere',
    github: 'GitHub',
    readme: 'README',
    chinese: '简体中文',
    issues: 'Issues',
    architecture: 'Architecture',
    wire: 'Wire protocol',
    changelog: 'Changelog',
    license: 'License',
    bottom: 'Go and NSGo. Static on GitHub Pages.',
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
      'Neorio 的智能体核心：四个编程智能体共用一套统一的 API，并用 NSGo 展示对话——就在本机，或隔着一条线。',
    ogAlt: 'Agio — 编程智能体，汇入一条平静的流',
  },

  nav: {
    skip: '跳到内容',
    primary: '主导航',
    home: 'Agio 首页',
    features: '功能',
    how: '工作方式',
    start: '快速开始',
    github: 'GitHub',
    getStarted: '开始使用',
    preview: '预览',
    menu: '打开菜单',
    close: '关闭菜单',
    menuLabel: '菜单',
  },

  theme: {
    label: '外观',
    light: '浅色',
    dark: '深色',
    system: '跟随系统',
  },

  language: {
    label: '语言',
    en: 'English',
    zh: '中文',
  },

  code: {
    copy: '复制代码',
    command: '复制命令',
  },

  hero: {
    title: 'Agio',
    lede: '编程智能体，汇入一条平静的流',
    body: 'Neorio 的智能体核心：四个编程智能体，一套统一的 API，用 NSGo 展示对话——就在本机，或隔着一条线。',
    primary: '开始使用',
    secondary: 'GitHub',
    installLabel: '用 Go 安装',
    fine: ['MIT', 'Go 1.27', '你的智能体，你的登录'],
    glanceLabel: '一览',
    alt: 'Agio 的窗口：一个有两个轮次的对话，旁边是对话列表',
    caption: 'chatview.App——每个对话、它的轮次，以及它们在哪里运行。',
  },

  agents: {
    label: '支持的智能体',
    title: '一条流，接上你已经在用的智能体',
    names: ['pi', 'Codex', 'Claude Code', 'CodeBuddy Code'],
    note: '以及任何 Agent Client Protocol CLI',
  },

  features: {
    eyebrow: '为什么是 Agio',
    title: 'CLI 与你的窗口之间的那些部分',
    lead: 'Agio 刻意保持小巧：驱动负责翻译，host 负责折叠，视图负责展示。每一层只认识下一层。',
    items: [
      {
        icon: 'terminal',
        accent: 'blue',
        title: '四个驱动，一套词汇',
        body: 'pi、Codex、Claude Code、CodeBuddy Code 各说各的协议。Agio 把它们变成同一套事件、轮次和状态；驱动之上，没有人知道正在运行哪一个。',
      },
      {
        icon: 'messages',
        accent: 'green',
        title: '一份记录，不是日志',
        body: '思考、工具调用、流式 Markdown 和一轮所花的时间，折叠成一段可以读的对话。运行中的轮次保持展开，结束的轮次让到一边。',
      },
      {
        icon: 'server',
        accent: 'violet',
        title: '就在这里，或隔着一条线',
        body: 'host.Host 在本进程运行智能体。wire 通过 TCP、管道或 SSH 会话提供同一个后端——视图看不出智能体在哪里运行。',
      },
      {
        icon: 'window',
        accent: 'amber',
        title: '窗口也是现成的',
        body: 'chatview.App 给你对话、侧边栏、输入框、模型和推理档位、智能体与模型设置，以及每个账户的限额。chatview.Conversation 是其中一个对话，适合你自己的布局。',
      },
      {
        icon: 'layers',
        accent: 'cyan',
        title: '没有 UI 的服务端',
        body: 'chatview 以下都是无头的。服务端不依赖 NSGo 就能构建，驱动只说中立类型，视图只通过 agio.Backend 接触智能体。',
      },
      {
        icon: 'shield',
        accent: 'coral',
        title: '不留下任何东西',
        body: 'Agio 不持久化任何东西。你的应用保存对话的 cursor、目录和模型。智能体拥有完全访问权限，所以把服务端放在私有网络，并给它一个令牌。',
      },
    ],
  },

  how: {
    eyebrow: '结构',
    title: '一个契约，两个运行地点',
    lead: '视图展示 agio.Backend。host 是其中一个，wire 客户端是另一个。整个把戏就这么多。',
    diagramLabel: 'Agio 的架构',
    viewLabel: '你的视图',
    viewNode: 'chatview.App / Conversation',
    contract: 'agio.Backend',
    localLabel: '在这个进程',
    localNode: 'host.Host',
    remoteLabel: '在别处',
    remoteClient: 'wire.Client',
    remoteServer: 'wire.Serve',
    driversLabel: '驱动',
    drivers: 'pi · Codex · Claude Code · CodeBuddy Code',
    note: '换掉后端，同一个视图就能展示本机或远程的智能体。服务端示例通过 TCP 提供这个 host；客户端把 wire.Client 交给同一个 chatview.App。',
    codeTitle: '单进程',
    link: '阅读架构',
  },

  turns: {
    eyebrow: '记录',
    title: '一份记录，不是日志',
    lead: '展开一个结束的轮次，读一读发生了什么：推理、打开的文件、运行的命令、流式写出的回答。',
    alt: '展开的 Agio 轮次：思考、一次编辑、一条命令，以及 Markdown 回答',
    caption: '工作了 4 秒，已展开：思考、一次编辑、一条命令，以及回答。',
    notes: [
      { label: '思考', body: '推理是一个步骤，不是一堵文字墙。' },
      { label: '工具', body: '每次调用都保留目标和输出。' },
      { label: '回答', body: '最终回复以 Markdown 流式写出。' },
    ],
  },

  under: {
    eyebrow: '在表面之下',
    title: '为线路而生',
    lead: '远程对话应该读起来是实时的，而且花费很少。设计由此而来。',
    items: [
      {
        value: '30/s',
        title: '发布，而不是推送',
        body: '服务端上的会话会合并变化，每秒最多发布约 30 次，所以流式回复读起来是实时的，又不会淹没慢连接。',
      },
      {
        value: 'patches',
        title: '只发送变化',
        body: '每个对话的轮次以补丁的形式增量传输。流式文本只发送到达的部分，而不是整段对话重来。',
      },
      {
        value: 'groups',
        title: '只碰自己的进程',
        body: '智能体在自己的进程组中启动。停掉一个，就停掉它启动的东西；host 只给自己启动的进程发信号。',
      },
    ],
  },

  start: {
    eyebrow: '快速开始',
    title: '几行代码，\n一个能用的窗口',
    lead: '安装模块，选择智能体在哪里运行，然后把后端交给视图。',
    requirements:
      '需要 Go 1.27，以及登录 shell 的 PATH 中你想用的智能体 CLI。它们使用你自己的登录状态运行。',
    installLabel: '安装模块',
    tabsLabel: '示例',
    tabs: [
      {
        id: 'single',
        label: '单进程',
        title: 'main.go',
        command: 'go tool nsgo dev examples/single',
        code: `h := host.New(host.Options{Dir: workDir})

nsgo.Run(func(cx *core.Scope) core.Node {
	return chatview.App{Model: chatview.Bind(cx, h)}
}, nsgo.WindowSize(1040, 720))

h.Close()`,
      },
      {
        id: 'server',
        label: '服务端',
        title: 'server.go',
        command: 'go run ./examples/server -token secret',
        code: `h := host.New(host.Options{Dir: workDir})
defer h.Close()

log.Fatal(wire.ListenAndServe(h, "127.0.0.1:7788", token))`,
      },
      {
        id: 'client',
        label: '客户端',
        title: 'client.go',
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
    ],
    readme: '阅读 README',
    examples: '运行示例',
  },

  built: {
    eyebrow: '构建于',
    title: 'Go 与 NSGo',
    body: '核心是无头 Go。视图是 NSGo 组件，写一次，以英文和中文展示。这个网站用 Astro 和 Tailwind，构建成静态文件，托管在 GitHub Pages。',
    nsgo: 'nsgo.dev',
    nsgoRepo: 'NSGo 的 GitHub',
  },

  footer: {
    note: 'Neorio 的智能体核心。MIT 许可。',
    project: '项目',
    elsewhere: '别处',
    github: 'GitHub',
    readme: 'README',
    chinese: '简体中文',
    issues: '问题',
    architecture: '架构',
    wire: 'Wire 协议',
    changelog: '更新日志',
    license: '许可证',
    bottom: 'Go 与 NSGo。静态托管于 GitHub Pages。',
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
