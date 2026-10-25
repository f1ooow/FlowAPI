import { createHash } from 'node:crypto'
import { readFile, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

// Uses the frontend's existing Marked dependency; run from web/ when building
// documentation in a worktree that does not have its own node_modules.
const directory = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(directory, '../..')
const require = createRequire(import.meta.url)
const markedPath = require.resolve('marked', {
  paths: [path.join(root, 'web'), process.cwd()],
})
const { marked } = await import(pathToFileURL(markedPath).href)
const markdown = await readFile(path.join(root, 'docs/suno.md'), 'utf8')
const sourceHash = createHash('sha256').update(markdown).digest('hex')
const sourceJSON = JSON.stringify(markdown).replaceAll('<', '\\u003c')
const tokens = marked.lexer(markdown)
const title = tokens.find((token) => token.type === 'heading' && token.depth === 1)
const leadIndex = tokens.findIndex((token) => token.type === 'paragraph')
if (!title || leadIndex < 0) throw new Error('Suno Markdown needs a title and introduction.')

function escapeHTML(value) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')
}

const copyIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><rect x="8" y="8" width="12" height="13" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/></svg>'
const downloadIcon = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M12 3v12m-5-5 5 5 5-5M4 15v4a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-4"/></svg>'
const renderer = new marked.Renderer()
const renderTable = renderer.table
const toc = []
let section = 0
let subsection = 0
let codeIndex = 0
renderer.heading = function (token) {
  const content = this.parser.parseInline(token.tokens)
  let id
  if (token.depth === 2) {
    section += 1
    subsection = 0
    id = `section-${section}`
    toc.push({ id, title: token.text.replace(/^\d+\.\s*/, '') })
  } else {
    subsection += 1
    id = `section-${section}-${subsection}`
  }
  return `<h${token.depth} id="${id}">${content}</h${token.depth}>\n`
}
renderer.code = function (token) {
  const index = codeIndex++
  const language = (token.lang || 'text').split(/\s+/)[0]
  return `<div class="code-block"><div class="code-bar"><span>${escapeHTML(language)}</span><button type="button" class="copy-code" data-copy-code="${index}" aria-label="复制第 ${index + 1} 个代码块">${copyIcon}<span>复制</span></button></div><pre tabindex="0"><code id="code-${index}" class="language-${escapeHTML(language)}">${escapeHTML(token.text)}</code></pre></div>\n`
}
renderer.table = function (token) {
  return `<div class="table-scroll" tabindex="0" role="region" aria-label="文档表格，可横向滚动">${renderTable.call(this, token)}</div>\n`
}
const bodyTokens = tokens.slice(leadIndex + 1)
bodyTokens.links = tokens.links
const content = marked.parser(bodyTokens, { renderer })
const lead = marked.parser([tokens[leadIndex]])
const tocHTML = toc.map((entry, index) => `<a href="#${entry.id}"><span class="toc-number">${String(index + 1).padStart(2, '0')}</span>${escapeHTML(entry.title)}</a>`).join('\n')

const html = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="description" content="FlowAPI Suno 调用方法：灵感歌曲、自定义歌词、纯音乐生成，以及提交任务、轮询、音频结果与完整 Python 示例。">
  <meta name="flowapi-markdown-sha256" content="${sourceHash}">
  <meta name="color-scheme" content="light">
  <title>${escapeHTML(title.text)} · FlowAPI 文档</title>
  <style>
    :root{--bg:#fff;--surface:#f6f8fa;--ink:#17212d;--muted:#5a6978;--line:#e2e8ed;--accent:#087e86;--accent-dark:#065f66;--accent-soft:#edf8f8;--radius:12px}
    *{box-sizing:border-box}
    html{scroll-behavior:smooth;scroll-padding-top:112px}
    body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.8 'Public Sans','Avenir Next','PingFang SC','Microsoft YaHei',sans-serif;-webkit-font-smoothing:antialiased}
    a{color:var(--accent);text-decoration:none}
    a:hover{text-decoration:underline;text-underline-offset:4px}
    button,a{-webkit-tap-highlight-color:transparent}
    button{font:inherit;cursor:pointer}
    :focus-visible{outline:3px solid #159ca6;outline-offset:4px}
    button:disabled{cursor:wait;opacity:.7}
    .skip-link{position:fixed;top:-80px;left:16px;z-index:60;padding:8px 16px;background:var(--ink);color:white;border-radius:8px}
    .skip-link:focus{top:12px}
    .topbar{position:sticky;top:0;z-index:30;border-bottom:1px solid var(--line);background:rgba(255,255,255,.97)}
    .top-inner{max-width:1320px;min-height:76px;margin:auto;padding:14px 32px;display:flex;align-items:center;justify-content:space-between;gap:24px}
    .brand{display:inline-flex;align-items:center;gap:12px;color:var(--ink);flex-shrink:0}
    .brand:hover{text-decoration:none}
    .brand-mark{width:32px;height:32px;display:grid;place-items:center;border-radius:9px;background:var(--accent);color:white}
    .brand-mark svg{width:22px;height:22px}
    .brand-name{font-size:19px;font-weight:700;letter-spacing:-.6px}
    .brand-divider{width:1px;height:20px;background:var(--line);margin-inline:3px}
    .brand-area{font-size:14px;color:var(--muted);font-weight:500}
    .top-actions{display:flex;align-items:center;gap:10px}
    .button{display:inline-flex;justify-content:center;align-items:center;gap:9px;min-height:44px;padding:9px 17px;border:1px solid var(--line);border-radius:8px;background:var(--bg);color:var(--ink);font-size:14px;font-weight:600;line-height:1.5;white-space:nowrap;transition:background .15s,border-color .15s}
    .button svg{width:17px;height:17px;flex-shrink:0}
    .button:hover{background:var(--surface);border-color:#b8c8ce}
    .button.primary{border-color:var(--accent);background:var(--accent);color:white}
    .button.primary:hover{background:var(--accent-dark);border-color:var(--accent-dark)}
    .layout{display:grid;grid-template-columns:220px minmax(0,1fr);gap:56px;max-width:1232px;margin:auto;padding:44px 32px 80px}
    .sidebar{align-self:start;position:sticky;top:112px;font-size:13px;color:var(--muted)}
    .sidebar-label{margin:0 0 12px;color:var(--muted);font-size:11px;letter-spacing:1.4px;font-weight:600;text-transform:uppercase}
    .document-link{display:flex;align-items:center;gap:10px;border-left:2px solid var(--accent);background:var(--accent-soft);padding:11px 14px;margin-bottom:32px;color:var(--accent-dark);font-size:14px;font-weight:600;border-radius:0 6px 6px 0}
    .document-link svg{width:17px;height:17px;flex-shrink:0}
    .document-link:hover{text-decoration:none;background:#e3f3f3}
    .toc{display:flex;flex-direction:column;gap:3px}
    .toc a{display:flex;align-items:baseline;gap:12px;line-height:1.5;color:var(--muted);padding:9px 10px;border-radius:6px}
    .toc a:hover,.toc a[aria-current="location"]{color:var(--accent-dark);background:var(--accent-soft);text-decoration:none}
    .toc-number{font-size:10px;font-weight:600;letter-spacing:.3px;color:#7d8d97;font-variant-numeric:tabular-nums}
    .sidebar-footer{display:block;margin:30px 10px 0;padding-top:16px;border-top:1px solid var(--line);color:var(--muted);font-size:12px}
    main{min-width:0;max-width:836px}
    .doc-header{padding:0 0 31px;border-bottom:1px solid var(--line);margin-bottom:35px;background:radial-gradient(ellipse at 100% 0,rgba(13,148,154,.035),transparent 65%)}
    .eyebrow{display:flex;align-items:center;gap:8px;margin:0 0 14px;color:var(--accent);font-size:12px;font-weight:700;letter-spacing:1.5px}
    .eyebrow:before{content:'';width:6px;height:6px;border-radius:50%;background:var(--accent)}
    h1{font-size:clamp(30px,3.5vw,40px);line-height:1.25;letter-spacing:-1.2px;font-weight:700;margin:0 0 18px}
    .lead{max-width:700px;font-size:16px;line-height:1.9;color:var(--muted)}
    .lead p{margin:0}
    .doc-meta{display:flex;flex-wrap:wrap;gap:8px;margin-top:21px}
    .meta-tag{display:inline-flex;align-items:center;border:1px solid var(--line);border-radius:5px;padding:3px 9px;color:var(--muted);background:white;font-size:11px;line-height:1.5;font-weight:500}
    .mobile-toc{display:none}
    article{overflow-wrap:anywhere}
    article h2{font-size:24px;line-height:1.45;font-weight:650;letter-spacing:-.5px;margin:49px 0 18px;padding-top:5px;scroll-margin-top:108px}
    article h2:first-child{margin-top:0}
    article h3{font-size:18px;line-height:1.5;font-weight:600;margin:30px 0 14px;scroll-margin-top:108px}
    article p{margin:14px 0}
    article strong{font-weight:650;color:#102c35}
    article p>strong:only-child{display:block;border-left:3px solid var(--accent);border-radius:0 6px 6px 0;background:var(--accent-soft);padding:13px 16px;font-size:14px;line-height:1.85}
    article code{font-family:'SFMono-Regular','Cascadia Code','Consolas',monospace;font-size:.85em;background:#f0f4f6;border:1px solid #e8eef1;border-radius:4px;padding:2px 5px;word-break:normal}
    .table-scroll{width:100%;max-width:100%;overflow-x:auto;border:1px solid var(--line);border-radius:9px;margin:20px 0 24px;-webkit-overflow-scrolling:touch}
    table{border-collapse:collapse;width:100%;font-size:13px;line-height:1.8}
    th,td{border-bottom:1px solid var(--line);padding:12px 14px;text-align:left;vertical-align:top}
    th{background:var(--surface);color:var(--muted);font-size:11px;letter-spacing:.2px;font-weight:600;white-space:nowrap}
    th:first-child,td:first-child{min-width:118px}
    td code{font-size:12px;white-space:normal}
    tr:last-child td{border-bottom:0}
    .code-block{margin:20px 0 24px;border:1px solid #dce5e9;border-radius:10px;overflow:hidden;background:#f7f9fb;box-shadow:0 1px 1px rgba(18,39,48,.025)}
    .code-bar{min-height:43px;display:flex;justify-content:space-between;align-items:center;padding:0 9px 0 16px;border-bottom:1px solid #e3eaee;background:#f0f4f6;color:var(--muted);font:10px/1.4 'Avenir Next','Public Sans',sans-serif;letter-spacing:.8px;text-transform:uppercase}
    .copy-code{min-width:66px;min-height:36px;padding:6px 8px;display:flex;align-items:center;gap:6px;border:0;border-radius:5px;background:transparent;color:#465d69;font-size:12px;letter-spacing:0}
    .copy-code svg{height:14px;width:14px}
    .copy-code:hover{background:#e3ebef;color:var(--accent-dark)}
    .copy-code[data-copied="true"]{color:var(--accent-dark);background:#ddf0ef}
    pre{margin:0;padding:20px;overflow:auto;line-height:1.9;tab-size:4;white-space:pre;-webkit-overflow-scrolling:touch}
    article pre code{display:block;border:0;padding:0;background:transparent;border-radius:0;color:#263d4a;font-size:12px;overflow-wrap:normal;white-space:pre}
    .doc-footer{margin-top:52px;padding-top:24px;border-top:1px solid var(--line);display:flex;flex-wrap:wrap;justify-content:space-between;gap:10px;color:var(--muted);font-size:11px}
    .doc-footer a{color:var(--muted)}
    .toast{position:fixed;left:50%;bottom:28px;z-index:50;max-width:calc(100% - 32px);transform:translate(-50%,12px);padding:11px 18px;border:1px solid #cce1e2;border-radius:9px;background:#f0fafa;color:var(--accent-dark);box-shadow:0 6px 24px rgba(23,52,58,.1);font-size:14px;line-height:1.5;opacity:0;pointer-events:none;transition:opacity .15s,transform .15s}
    .toast.visible{transform:translate(-50%,0);opacity:1}
    noscript{display:block;padding:12px 16px;background:var(--accent-soft);font-size:14px;color:var(--accent-dark)}
    @media(min-width:1450px){.layout{gap:76px}}
    @media(max-width:1000px){.layout{grid-template-columns:180px minmax(0,1fr);gap:30px;padding-inline:24px}.top-inner{padding-inline:24px}.sidebar{font-size:12px}.toc a{padding-inline:6px;gap:8px}.toc-number{display:none}}
    @media(max-width:760px){.layout{display:block;padding:30px 20px 56px}.sidebar{display:none}.top-inner{padding-inline:20px;gap:16px}.brand-divider,.brand-area{display:none}.button{padding:9px 12px;font-size:13px}.button svg{height:16px;width:16px}.brand-name{font-size:17px}.brand-mark{width:29px;height:29px}.brand{gap:8px}.top-actions{gap:8px}.doc-header{padding-bottom:26px;margin-bottom:26px}.mobile-toc{display:block;border:1px solid var(--line);border-radius:8px;padding:0 14px;margin:0 0 30px;background:var(--bg)}.mobile-toc summary{cursor:pointer;padding:11px 0;font-size:13px;font-weight:600;color:var(--muted)}.mobile-toc .toc{padding:0 0 12px}.mobile-toc .toc-number{display:inline}.lead{font-size:15px}.doc-meta{gap:6px}article h2{font-size:22px;margin-top:39px}article h3{font-size:17px}article p{font-size:15px;line-height:1.9}th,td{padding:11px 12px}.code-block{margin:18px 0 22px}pre{padding:16px}article pre code{font-size:11px}}
    @media(max-width:540px){html{scroll-padding-top:148px}.top-inner{flex-wrap:wrap;padding:12px 18px;gap:12px}.brand-divider,.brand-area{display:block}.brand-name{font-size:18px}.top-actions{width:100%}.top-actions .primary{flex:1}.top-actions .button{min-height:43px}.layout{padding-top:26px}article h2,article h3{scroll-margin-top:146px}.doc-header{margin-bottom:23px}.eyebrow{font-size:10px}h1{font-size:32px;letter-spacing:-1px}.meta-tag{font-size:10px}}
    @media(prefers-reduced-motion:reduce){html{scroll-behavior:auto}*,*:before,*:after{transition:none!important}}
    @media print{.topbar,.sidebar,.mobile-toc,.toast,.copy-code,.skip-link{display:none}.layout{display:block;padding:0}.doc-header{background:none}.code-block,.table-scroll{break-inside:avoid}pre{white-space:pre-wrap;overflow-wrap:anywhere}article pre code{white-space:pre-wrap}.doc-footer{margin-top:28px}}
  </style>
</head>
<body>
  <a class="skip-link" href="#main-content">跳到文档正文</a>
  <header class="topbar">
    <div class="top-inner">
      <a class="brand" href="https://flowapi.robusta.top/" aria-label="返回 FlowAPI 首页">
        <span class="brand-mark"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M5 8h14M5 16h14M9 4l-4 4 4 4m6 0 4 4-4 4"/></svg></span>
        <span class="brand-name">Flow API</span><span class="brand-divider"></span><span class="brand-area">文档区</span>
      </a>
      <div class="top-actions">
        <button id="copy-markdown" class="button primary" type="button">${copyIcon}<span>一键复制 Markdown</span></button>
        <button id="download-markdown" class="button" type="button">${downloadIcon}<span>下载 .md</span></button>
      </div>
    </div>
  </header>
  <noscript>正文可以直接阅读。复制全文、复制代码和下载 Markdown 功能需要启用 JavaScript。</noscript>
  <div class="layout">
    <aside class="sidebar" aria-label="文档导航">
      <p class="sidebar-label">开发者文档</p>
      <a class="document-link" href="#main-content" aria-current="page"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="M9 18V5l12-2v13M9 8l12-2"/><ellipse cx="6" cy="18" rx="3" ry="2"/><ellipse cx="18" cy="16" rx="3" ry="2"/></svg>Suno 调用方法</a>
      <p class="sidebar-label">本页内容</p>
      <nav class="toc" aria-label="本页目录">${tocHTML}</nav>
      <a class="sidebar-footer" href="https://flowapi.robusta.top/">← 返回 FlowAPI</a>
    </aside>
    <main id="main-content" tabindex="-1">
      <header class="doc-header">
        <p class="eyebrow">音乐生成 / SUNO</p>
        <h1>${escapeHTML(title.text)}</h1>
        <div class="lead">${lead}</div>
        <div class="doc-meta"><span class="meta-tag">灵感创作</span><span class="meta-tag">自定义歌词</span><span class="meta-tag">纯音乐</span><span class="meta-tag">完整调用示例</span></div>
      </header>
      <details class="mobile-toc"><summary>本页目录</summary><nav class="toc" aria-label="移动端本页目录">${tocHTML}</nav></details>
      <article aria-label="Suno 完整调用文档">${content}</article>
      <footer class="doc-footer"><span>FlowAPI · 开发者文档</span><a href="#main-content">返回顶部 ↑</a></footer>
    </main>
  </div>
  <div id="copy-feedback" class="toast" role="status" aria-live="polite" aria-atomic="true"></div>
  <script id="markdown-source" type="application/json">${sourceJSON}</script>
  <script>
    const markdown = JSON.parse(document.getElementById('markdown-source').textContent);
    const feedback = document.getElementById('copy-feedback');
    const resetTimers = new WeakMap();
    let feedbackTimer;
    function announce(message) {
      clearTimeout(feedbackTimer);
      feedback.textContent = message;
      feedback.classList.add('visible');
      feedbackTimer = setTimeout(() => feedback.classList.remove('visible'), 3200);
    }
    async function copyText(text) {
      try {
        if (navigator.clipboard && window.isSecureContext) {
          await navigator.clipboard.writeText(text);
          return;
        }
      } catch (_) {}
      const previousFocus = document.activeElement;
      const textarea = document.createElement('textarea');
      textarea.value = text;
      textarea.readOnly = true;
      textarea.style.cssText = 'position:fixed;top:0;left:-9999px;opacity:0';
      document.body.appendChild(textarea);
      textarea.select();
      const copied = document.execCommand('copy');
      textarea.remove();
      if (previousFocus instanceof HTMLElement) previousFocus.focus({preventScroll:true});
      if (!copied) throw new Error('Clipboard is unavailable');
    }
    async function copyWithFeedback(button, text, message) {
      const label = button.querySelector('span');
      const original = button.id === 'copy-markdown' ? '一键复制 Markdown' : '复制';
      clearTimeout(resetTimers.get(button));
      button.disabled = true;
      try {
        await copyText(text);
        label.textContent = '已复制';
        button.dataset.copied = 'true';
        announce(message);
      } catch (_) {
        announce('复制未成功，请使用「下载 .md」保存完整文档。');
      } finally {
        button.disabled = false;
        resetTimers.set(button, setTimeout(() => {
          label.textContent = original;
          delete button.dataset.copied;
        }, 2200));
      }
    }
    document.getElementById('copy-markdown').addEventListener('click', function () {
      copyWithFeedback(this, markdown, '已复制完整 Markdown，可直接粘贴给开发工具或团队成员。');
    });
    document.querySelectorAll('[data-copy-code]').forEach((button) => {
      button.addEventListener('click', function () {
        const code = document.getElementById('code-' + this.dataset.copyCode);
        copyWithFeedback(this, code.textContent, '代码已复制。');
      });
    });
    document.getElementById('download-markdown').addEventListener('click', () => {
      const blob = new Blob([markdown], {type:'text/markdown;charset=utf-8'});
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = 'FlowAPI-Suno-调用方法.md';
      document.body.appendChild(link);
      link.click();
      link.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
      announce('Markdown 下载已开始。');
    });
    const headings = document.querySelectorAll('article h2');
    const tocLinks = document.querySelectorAll('.toc a');
    if ('IntersectionObserver' in window) {
      const observer = new IntersectionObserver((entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue;
          tocLinks.forEach((link) => {
            if (link.hash === '#' + entry.target.id) link.setAttribute('aria-current', 'location');
            else link.removeAttribute('aria-current');
          });
        }
      }, {rootMargin:'-150px 0px -65% 0px', threshold:0});
      headings.forEach((heading) => observer.observe(heading));
    }
  </script>
</body>
</html>
`

await writeFile(path.join(directory, 'index.html'), html, 'utf8')
console.log(`Generated docs/site/index.html from docs/suno.md (${codeIndex} code blocks, SHA-256 ${sourceHash}).`)
