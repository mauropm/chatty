/* Chatty — front-end application (vanilla JS, no framework). */
(() => {
  'use strict';

  // ---------------------------------------------------------------- icons
  const ICONS = {
    logo: '<svg viewBox="0 0 32 32" fill="none"><rect x="2" y="3" width="28" height="22" rx="8" fill="url(#lg)"/><path d="M10 25.5c0 2.3 3 3.6 5 2l3-2.5" stroke="url(#lg)" stroke-width="3.2" stroke-linecap="round"/><circle cx="11.5" cy="14" r="2.1" fill="#101014"/><circle cx="16" cy="14" r="2.1" fill="#101014"/><circle cx="20.5" cy="14" r="2.1" fill="#101014"/><defs><linearGradient id="lg" x1="2" y1="3" x2="30" y2="27"><stop stop-color="#A78BFA"/><stop offset="1" stop-color="#67E8F9"/></linearGradient></defs></svg>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    search: '<circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>',
    send: '<path d="M22 2 11 13"/><path d="M22 2 15 22l-4-9-9-4 20-7z"/>',
    stop: '<rect x="6" y="6" width="12" height="12" rx="2.5" fill="currentColor" stroke="none"/>',
    copy: '<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15V5a2 2 0 0 1 2-2h10"/>',
    check: '<path d="M20 6 9 17l-5-5"/>',
    refresh: '<path d="M3 12a9 9 0 0 1 15-6.7L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-15 6.7L3 16"/><path d="M3 21v-5h5"/>',
    dots: '<circle cx="12" cy="5" r="1.6" fill="currentColor" stroke="none"/><circle cx="12" cy="12" r="1.6" fill="currentColor" stroke="none"/><circle cx="12" cy="19" r="1.6" fill="currentColor" stroke="none"/>',
    pencil: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4 12.5-12.5z"/>',
    archive: '<rect x="3" y="4" width="18" height="4" rx="1"/><path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8"/><path d="M10 12h4"/>',
    trash: '<path d="M3 6h18"/><path d="M8 6V4a1 1 0 0 1 1-1h6a1 1 0 0 1 1 1v2"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-2.7 1.1V21a2 2 0 1 1-4 0v-.1A1.6 1.6 0 0 0 7 19.4l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1A1.6 1.6 0 0 0 3.1 14H3a2 2 0 1 1 0-4h.1A1.6 1.6 0 0 0 4.6 7l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1A1.6 1.6 0 0 0 10 3.1V3a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 2.7 1.1l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0 1.1 2.7H21a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1z"/>',
    keyboard: '<rect x="2" y="5" width="20" height="14" rx="2"/><path d="M6 9h.01M10 9h.01M14 9h.01M18 9h.01M8 13h8"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/>',
    download: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 10l5 5 5-5"/><path d="M12 15V3"/>',
    file: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/><path d="M9 13h6"/><path d="M9 17h4"/>',
    braces: '<path d="M8 3H7a2 2 0 0 0-2 2v4a2 2 0 0 1-2 2 2 2 0 0 1 2 2v4a2 2 0 0 0 2 2h1"/><path d="M16 3h1a2 2 0 0 1 2 2v4a2 2 0 0 0 2 2 2 2 0 0 0-2 2v4a2 2 0 0 1-2 2h-1"/>',
    panel: '<rect x="3" y="3" width="18" height="18" rx="2"/><path d="M9 3v18"/>',
    chevron: '<path d="m9 18 6-6-6-6"/>',
    spark: '<path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M18.4 5.6l-2.1 2.1M7.7 16.3l-2.1 2.1"/>',
    bug: '<path d="M8 2l1.5 1.5M16 2l-1.5 1.5"/><rect x="8" y="6" width="8" height="12" rx="4"/><path d="M4 10h4M16 10h4M4 15h4M16 15h4M9 18l-2 3M15 18l2 3"/>',
    bulb: '<path d="M9 18h6M10 22h4M12 2a7 7 0 0 0-4 12.7c.6.5 1 1.3 1 2.1V17h6v-.2c0-.8.4-1.6 1-2.1A7 7 0 0 0 12 2z"/>',
    chat: '<path d="M21 11.5a8.4 8.4 0 0 1-8.5 8.5 8.5 8.5 0 0 1-3.8-.9L3 21l1.9-5.7A8.4 8.4 0 0 1 4 11.5 8.5 8.5 0 0 1 12.5 3 8.4 8.4 0 0 1 21 11.5z"/>',
    alert: '<path d="M12 9v4M12 17h.01"/><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/>',
    close: '<path d="M18 6 6 18M6 6l12 12"/>',
  };
  const icon = (name, extra = '') =>
    `<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" ${extra}>${ICONS[name] || ''}</svg>`;

  // ---------------------------------------------------------------- helpers
  const $ = (sel, root = document) => root.querySelector(sel);
  const el = (tag, props = {}, kids = []) => {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(props)) {
      if (k === 'class') n.className = v;
      else if (k === 'html') n.innerHTML = v;
      else if (k === 'text') n.textContent = v;
      else if (k.startsWith('on') && typeof v === 'function') n.addEventListener(k.slice(2), v);
      else if (v !== null && v !== undefined) n.setAttribute(k, v);
    }
    for (const c of [].concat(kids)) if (c != null) n.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    return n;
  };
  const fmtTime = (iso) => new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const debounce = (fn, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => fn(...a), ms); }; };

  // ---------------------------------------------------------------- state
  const state = {
    conversations: [],
    activeId: null,
    messages: [],
    generating: false,
    controller: null,
    model: '',
  };
  const prefs = {
    get theme() { return localStorage.getItem('chatty.theme') || 'system'; },
    set theme(v) { localStorage.setItem('chatty.theme', v); },
    get sidebar() { return localStorage.getItem('chatty.sidebar') || 'expanded'; },
    set sidebar(v) { localStorage.setItem('chatty.sidebar', v); },
    get fontSize() { return parseInt(localStorage.getItem('chatty.fontSize') || '16', 10); },
    set fontSize(v) { localStorage.setItem('chatty.fontSize', String(v)); },
  };

  // ---------------------------------------------------------------- api
  const api = {
    async req(method, url, body) {
      const opts = { method, headers: {} };
      if (body !== undefined) { opts.headers['Content-Type'] = 'application/json'; opts.body = JSON.stringify(body); }
      const res = await fetch(url, opts);
      if (res.status === 204) return null;
      const text = await res.text();
      const data = text ? JSON.parse(text) : null;
      if (!res.ok) throw new Error((data && data.error) || `Request failed (${res.status})`);
      return data;
    },
    get: (u) => api.req('GET', u),
    post: (u, b) => api.req('POST', u, b),
    patch: (u, b) => api.req('PATCH', u, b),
    del: (u) => api.req('DELETE', u),
  };

  // ---------------------------------------------------------------- toasts
  function toast(msg, kind = '') {
    const t = el('div', { class: `toast ${kind}`, html: icon(kind === 'err' ? 'alert' : kind === 'ok' ? 'check' : 'spark') + `<span></span>` });
    t.lastChild.textContent = msg;
    $('#toasts').appendChild(t);
    setTimeout(() => { t.style.opacity = '0'; t.style.transform = 'translateY(6px)'; t.style.transition = 'all 200ms'; }, 2600);
    setTimeout(() => t.remove(), 2900);
  }

  // ---------------------------------------------------------------- markdown
  if (window.marked) marked.setOptions({ gfm: true, breaks: false, headerIds: false, mangle: false });
  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    if (node.tagName === 'A' && node.getAttribute('href')) {
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer nofollow');
    }
  });
  function renderMarkdown(text, highlight = true) {
    const raw = window.marked ? marked.parse(text || '') : escapeHtml(text || '');
    const clean = DOMPurify.sanitize(raw, { ADD_ATTR: ['target'], USE_PROFILES: { html: true } });
    const wrap = el('div', { class: 'md', html: clean });
    decorateCode(wrap, highlight);
    return wrap;
  }
  function decorateCode(root, highlight) {
    root.querySelectorAll('pre > code').forEach((code) => {
      if (code.closest('.codeblock')) return;
      const pre = code.parentElement;
      const lang = (code.className.match(/language-([\w-]+)/) || [])[1] || '';
      const block = el('div', { class: 'codeblock' });
      const copyBtn = el('button', { class: 'codeblock__copy', type: 'button', html: `${icon('copy', 'width="13" height="13"')}<span>Copy</span>` });
      copyBtn.addEventListener('click', () => {
        navigator.clipboard.writeText(code.innerText).then(() => {
          copyBtn.lastChild.textContent = 'Copied';
          setTimeout(() => (copyBtn.lastChild.textContent = 'Copy'), 1400);
        });
      });
      const head = el('div', { class: 'codeblock__head' }, [el('span', { class: 'codeblock__lang', text: lang || 'code' }), copyBtn]);
      const clone = pre.cloneNode(true);
      block.append(head, clone);
      pre.replaceWith(block);
      if (highlight && window.hljs) {
        try { hljs.highlightElement(clone.querySelector('code')); } catch (e) {}
      }
    });
  }
  function escapeHtml(s) { return (s || '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])); }

  // ---------------------------------------------------------------- sidebar
  function groupLabel(iso) {
    const d = new Date(iso); const now = new Date();
    const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const startOfThat = new Date(d.getFullYear(), d.getMonth(), d.getDate());
    const days = Math.round((startOfToday - startOfThat) / 86400000);
    if (days <= 0) return 'Today';
    if (days === 1) return 'Yesterday';
    if (days <= 7) return 'Previous 7 days';
    if (days <= 30) return 'Previous 30 days';
    return 'Older';
  }

  async function loadConversations() {
    const data = await api.get('/api/conversations');
    state.conversations = data.conversations || [];
    renderConversations();
  }

  function renderConversations(list) {
    const container = $('#convList');
    const items = (list || state.conversations);
    container.innerHTML = '';
    if (!items.length) {
      container.appendChild(el('div', { class: 'side-label', text: items === state.conversations ? 'No conversations yet' : 'No results' }));
      return;
    }
    const groups = new Map();
    for (const c of items) {
      const g = groupLabel(c.updated_at);
      if (!groups.has(g)) groups.set(g, []);
      groups.get(g).push(c);
    }
    const order = ['Today', 'Yesterday', 'Previous 7 days', 'Previous 30 days', 'Older'];
    for (const name of order) {
      if (!groups.has(name)) continue;
      const g = el('div', { class: 'conv-group' });
      g.appendChild(el('div', { class: 'side-label', text: name }));
      for (const c of groups.get(name)) g.appendChild(convItem(c));
      container.appendChild(g);
    }
  }

  function convItem(c) {
    const item = el('div', { class: 'conv-item' + (c.id === state.activeId ? ' active' : ''), role: 'button', tabindex: '0' }, [
      el('span', { class: 'conv-item__title', text: c.title || 'Untitled' }),
      el('button', { class: 'conv-item__menu', 'aria-expanded': 'false', title: 'More', html: icon('dots', 'width="16" height="16"') }),
    ]);
    item.addEventListener('click', (e) => { if (!e.target.closest('.conv-item__menu')) openConversation(c.id); });
    item.addEventListener('keydown', (e) => { if (e.key === 'Enter') openConversation(c.id); });
    item.querySelector('.conv-item__menu').addEventListener('click', (e) => {
      e.stopPropagation(); showCtxMenu(e.currentTarget, c);
    });
    return item;
  }

  // ---------------------------------------------------------------- ctx menu
  let ctxOpen = null;
  function showCtxMenu(anchor, conv) {
    const menu = $('#ctxMenu');
    menu.innerHTML = '';
    const add = (ic, label, fn, danger) => {
      const b = el('button', { class: danger ? 'danger' : '', html: icon(ic, 'width="16" height="16"') + `<span></span>` });
      b.lastChild.textContent = label;
      b.addEventListener('click', () => { closeCtxMenu(); fn(); });
      menu.appendChild(b);
    };
    add('pencil', 'Rename', () => renameConversation(conv.id));
    add('file', 'Export Markdown', () => downloadExport(conv.id, 'md'));
    add('braces', 'Export JSON', () => downloadExport(conv.id, 'json'));
    menu.appendChild(el('div', { class: 'menu__sep' }));
    add('archive', conv.archived ? 'Restore' : 'Archive', () => toggleArchive(conv.id, !conv.archived));
    add('trash', 'Delete', () => deleteConversation(conv.id), true);
    const r = anchor.getBoundingClientRect();
    menu.style.left = Math.min(r.left, window.innerWidth - 190) + 'px';
    menu.style.top = r.bottom + 6 + 'px';
    menu.classList.add('open');
    ctxOpen = menu;
    setTimeout(() => document.addEventListener('click', onDocCloseCtx, { once: true }), 0);
  }
  const onDocCloseCtx = () => closeCtxMenu();
  function closeCtxMenu() { if (ctxOpen) { ctxOpen.classList.remove('open'); ctxOpen = null; } }

  // ---------------------------------------------------------------- conversation ops
  async function renameConversation(id) {
    const conv = state.conversations.find((c) => c.id === id) || (state.activeId === id ? { title: $('#convTitle').textContent } : null);
    const name = window.prompt('Rename conversation', conv ? conv.title : '');
    if (name == null || !name.trim()) return;
    await api.patch(`/api/conversations/${id}`, { title: name.trim() });
    if (id === state.activeId) $('#convTitle').textContent = name.trim();
    loadConversations();
  }
  async function toggleArchive(id, archived) {
    await api.patch(`/api/conversations/${id}`, { archived });
    if (id === state.activeId && archived) { state.activeId = null; showWelcome(); }
    loadConversations();
    toast(archived ? 'Conversation archived' : 'Conversation restored', 'ok');
  }
  async function deleteConversation(id) {
    if (!window.confirm('Delete this conversation permanently?')) return;
    try {
      await api.del(`/api/conversations/${id}`);
      if (id === state.activeId) { state.activeId = null; showWelcome(); }
      loadConversations();
      toast('Conversation deleted', 'ok');
    } catch (e) { toast(e.message, 'err'); }
  }
  function downloadExport(id, format) {
    const a = el('a', { href: `/api/conversations/${id}/export?format=${format}`, rel: 'noopener' });
    document.body.appendChild(a); a.click(); a.remove();
  }

  // ---------------------------------------------------------------- render conversation
  async function openConversation(id) {
    closeDrawer();
    state.activeId = id;
    history.replaceState(null, '', '#/c/' + id);
    renderConversations();
    const [conv, msgData] = await Promise.all([api.get(`/api/conversations/${id}`), api.get(`/api/conversations/${id}/messages`)]);
    state.messages = msgData.messages || [];
    $('#convTitle').textContent = conv.title || 'Untitled';
    $('#convTitle').setAttribute('data-id', id);
    const has = state.messages.length > 0;
    $('#renameBtn').hidden = !has; $('#exportMdBtn').hidden = !has; $('#exportJsonBtn').hidden = !has;
    renderMessages();
    $('#input').focus();
  }

  function renderMessages() {
    const inner = $('#convoInner');
    inner.innerHTML = '';
    if (!state.messages.length) { showWelcome(); return; }
    for (const m of state.messages) inner.appendChild(messageNode(m));
    scrollToBottom(true);
  }

  function messageNode(m) {
    const isUser = m.role === 'user';
    const node = el('div', { class: `msg msg--${isUser ? 'user' : 'assistant'}` + (m.status === 'failed' ? ' is-failed' : ''), 'data-id': m.id });
    const avatar = el('div', { class: 'msg__avatar', text: isUser ? 'You' : '✦' });
    if (!isUser) avatar.innerHTML = icon('chat', 'width="16" height="16"');
    const name = el('span', { class: 'name', text: isUser ? 'You' : 'Chatty' });
    const time = el('span', { text: fmtTime(m.created_at) });
    const meta = el('div', { class: 'msg__meta' }, [name, time]);
    if (!isUser && m.status === 'stopped') meta.appendChild(el('span', { text: '· stopped' }));
    if (!isUser && m.status === 'failed') meta.appendChild(el('span', { text: '· failed' }));

    const body = el('div', { class: 'msg__body' }, [meta]);
    const content = el('div', { class: 'msg__content' });
    if (isUser) content.textContent = m.content;
    else content.appendChild(renderMarkdown(m.content || '', true));
    body.appendChild(content);

    if (!isUser) {
      const tools = el('div', { class: 'msg__tools' });
      const copyBtn = el('button', { class: 'tool', title: 'Copy', html: icon('copy', 'width="15" height="15"') });
      copyBtn.addEventListener('click', () => { navigator.clipboard.writeText(m.content || ''); toast('Copied', 'ok'); });
      tools.appendChild(copyBtn);
      if (m.status !== 'streaming') {
        const regen = el('button', { class: 'tool', title: 'Regenerate', html: icon('refresh', 'width="15" height="15"') });
        regen.addEventListener('click', () => regenerate());
        tools.appendChild(regen);
      }
      meta.appendChild(tools);
      if (m.status === 'failed') {
        const row = el('div', { class: 'retry-row' });
        const errText = el('div', { text: m.error || 'The response could not be generated.', style: 'color:var(--error);font-size:.85rem' });
        const retry = el('button', { class: 'btn btn--ghost', html: icon('refresh', 'width="15" height="15"') + '<span></span>' });
        retry.lastChild.textContent = 'Retry';
        retry.addEventListener('click', () => regenerate());
        row.append(errText, retry);
        body.appendChild(row);
        content.remove();
      }
    }
    node.append(avatar, body);
    return node;
  }

  // ---------------------------------------------------------------- welcome
  function showWelcome() {
    state.activeId = null;
    state.messages = [];
    history.replaceState(null, '', '#/');
    renderConversations();
    $('#convTitle').textContent = 'New conversation';
    $('#renameBtn').hidden = $('#exportMdBtn').hidden = $('#exportJsonBtn').hidden = true;
    const inner = $('#convoInner');
    inner.innerHTML = '';
    const suggestions = [
      { icon: 'bulb', t: 'Explain something complex', d: 'Break down a hard concept into plain language.' },
      { icon: 'bug', t: 'Help me debug this', d: 'Paste code and I’ll help find the bug.' },
      { icon: 'spark', t: 'Explore a new idea', d: 'Brainstorm, outline, and think it through.' },
      { icon: 'chat', t: 'Challenge my assumptions', d: 'Play devil’s advocate on a decision.' },
    ];
    const sug = el('div', { class: 'suggestions' });
    for (const s of suggestions) {
      const b = el('button', { class: 'suggestion', type: 'button' }, [
        el('div', { class: 'suggestion__t', html: icon(s.icon, 'width="16" height="16"') + `<span></span>` }),
        el('div', { class: 'suggestion__d', text: s.d }),
      ]);
      b.querySelector('.suggestion__t').lastChild.textContent = s.t;
      b.addEventListener('click', () => { $('#input').value = s.t + ' '; $('#input').focus(); updateSendState(); autoGrow(); });
      sug.appendChild(b);
    }
    const welcome = el('div', { class: 'welcome' }, [
      el('div', { class: 'welcome__logo', html: ICONS.logo }),
      el('div', { class: 'welcome__eyebrow', text: 'Your thoughts, amplified.' }),
      el('h1', { html: 'What’s on <span class="grad">your mind?</span>' }),
      el('p', { class: 'welcome__sub', text: 'Start a conversation below, or try one of these.' }),
      sug,
    ]);
    inner.appendChild(welcome);
  }

  async function newConversation() {
    const conv = await api.post('/api/conversations', {});
    await loadConversations();
    await openConversation(conv.id);
  }

  // ---------------------------------------------------------------- composer
  const input = () => $('#input');
  function autoGrow() {
    const t = input(); t.style.height = 'auto';
    t.style.height = Math.min(t.scrollHeight, window.innerHeight * 0.44) + 'px';
  }
  function updateSendState() {
    const hasText = input().value.trim().length > 0;
    const btn = $('#sendBtn');
    if (state.generating) {
      btn.disabled = false; btn.classList.add('is-stop'); btn.innerHTML = icon('stop');
      btn.title = 'Stop generation (Esc)'; btn.dataset.mode = 'stop';
    } else {
      btn.disabled = !hasText; btn.classList.remove('is-stop'); btn.innerHTML = icon('send');
      btn.title = 'Send (Enter)'; btn.dataset.mode = 'send';
    }
    const n = input().value.length;
    $('#charCount').textContent = n > 200 ? n.toLocaleString() : '';
  }

  async function sendMessage() {
    if (state.generating) return;
    const text = input().value.trim();
    if (!text) return;
    input().value = ''; autoGrow(); updateSendState();
    let id = state.activeId;
    if (!id) { const conv = await api.post('/api/conversations', {}); id = conv.id; state.activeId = id; await loadConversations(); }
    // Optimistic user bubble.
    state.messages.push({ role: 'user', content: text, created_at: new Date().toISOString(), status: 'complete' });
    if ($('#convoInner .welcome')) $('#convoInner').innerHTML = '';
    $('#convoInner').appendChild(messageNode(state.messages[state.messages.length - 1]));
    scrollToBottom();
    await runStream(id, { content: text });
  }

  async function regenerate() {
    if (state.generating || !state.activeId) return;
    // remove trailing assistant message optimistically
    while (state.messages.length && state.messages[state.messages.length - 1].role === 'assistant') state.messages.pop();
    const inner = $('#convoInner'); const nodes = inner.querySelectorAll('.msg');
    if (nodes.length) nodes[nodes.length - 1].remove();
    await runStream(state.activeId, { regenerate: true });
  }

  // Shows an animated "thinking" loader with a rotating kid-friendly phrase that
  // stays up long enough to read. Each phrase holds for PHRASE_MS; if the model
  // is still thinking after that, the next phrase fades in. Returns a stop fn.
  const PHRASE_MS = 10000;   // one phrase stays ~10s so it can be read
  const MIN_LOADER_MS = 2500; // keep the loader+first phrase on screen long enough to register
  function startWaiting(node) {
    const list = window.CHATTY_WAITING;
    if (!Array.isArray(list) || !list.length) return () => {};
    let i = Math.floor(Math.random() * list.length);
    const tick = () => {
      node.textContent = list[i++ % list.length];
      node.style.animation = 'none'; void node.offsetWidth; node.style.animation = '';
    };
    tick();
    const id = setInterval(tick, PHRASE_MS);
    return () => clearInterval(id);
  }

  function mountThinking(contentEl) {
    const box = el('div', { class: 'thinking' }, [el('span', { class: 'thinking__spinner' })]);
    const phrase = el('span', { class: 'thinking__phrase' });
    box.appendChild(phrase);
    contentEl.appendChild(box);
    return { box, stop: startWaiting(phrase) };
  }

  async function runStream(id, body) {
    setGenerating(true);
    // assistant placeholder
    const placeholder = { role: 'assistant', content: '', created_at: new Date().toISOString(), status: 'streaming' };
    state.messages.push(placeholder);
    const node = messageNode(placeholder);
    const contentEl = node.querySelector('.msg__content');
    $('#convoInner').appendChild(node);
    contentEl.innerHTML = '';
    const thinking = mountThinking(contentEl);
    let stopWaiting = thinking.stop;
    scrollToBottom();

    const startedAt = Date.now();
    let firstPaint = true;
    const buffer = { text: '', reasoning: '' };
    let pending = false;
    const renderOnce = () => {
      pending = false;
      if (stopWaiting) { stopWaiting(); stopWaiting = null; }
      contentEl.innerHTML = '';
      contentEl.appendChild(renderMarkdown(buffer.text, false));
      const caret = el('span', { class: 'caret' });
      contentEl.querySelector('.md') ? contentEl.querySelector('.md').appendChild(caret) : contentEl.appendChild(caret);
      scrollToBottom();
    };
    const scheduleRender = () => {
      if (pending) return; pending = true;
      const wait = firstPaint ? Math.max(0, MIN_LOADER_MS - (Date.now() - startedAt)) : 0;
      firstPaint = false;
      if (wait > 0) setTimeout(() => requestAnimationFrame(renderOnce), wait);
      else requestAnimationFrame(renderOnce);
    };

    let finalStatus = 'complete';
    state.controller = new AbortController();
    try {
      const res = await fetch(`/api/conversations/${id}/${body.regenerate ? 'regenerate' : 'generate'}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body.content != null ? { content: body.content } : {}),
        signal: state.controller.signal,
      });
      if (!res.ok || !res.body) {
        const errText = await safeError(res); throw new Error(errText);
      }
      await readSSE(res.body, (ev) => {
        if (ev.event === 'meta') { placeholder.id = ev.data.message_id; }
        else if (ev.event === 'reasoning') { buffer.reasoning += ev.data.content; }
        else if (ev.event === 'delta') { buffer.text += ev.data.content; placeholder.content = buffer.text; scheduleRender(); }
        else if (ev.event === 'error') { finalStatus = ev.data.status || 'failed'; placeholder.error = ev.data.message; if (!placeholder.content) placeholder.content = ''; }
        else if (ev.event === 'done') {
          if (ev.data.status) finalStatus = ev.data.status;
          if (ev.data.content != null) { placeholder.content = ev.data.content; buffer.text = ev.data.content; }
        }
      });
    } catch (err) {
      if (err.name === 'AbortError') finalStatus = placeholder.content ? 'stopped' : 'stopped';
      else { finalStatus = 'failed'; placeholder.error = err.message; toast(err.message, 'err'); }
    } finally {
      state.controller = null; setGenerating(false);
      if (stopWaiting) { stopWaiting(); stopWaiting = null; }
      const hasMsgs = state.messages.length > 0;
      $('#renameBtn').hidden = !hasMsgs; $('#exportMdBtn').hidden = !hasMsgs; $('#exportJsonBtn').hidden = !hasMsgs;
      // final render with highlighting
      contentEl.innerHTML = '';
      if (finalStatus === 'failed') {
        const meta = node.querySelector('.msg__meta');
        if (meta) { node.classList.add('is-failed'); }
      }
      contentEl.appendChild(renderMarkdown(placeholder.content || '', true));
      placeholder.status = finalStatus;
      node.querySelector('.msg__meta')?.insertAdjacentText('beforeend', finalStatus === 'stopped' ? ' · stopped' : finalStatus === 'failed' ? ' · failed' : '');
      if (finalStatus === 'failed') {
        contentEl.innerHTML = '';
        contentEl.appendChild(el('div', { text: placeholder.error || 'Could not generate a response.', style: 'color:var(--error)' }));
        node.classList.add('is-failed');
      }
      // append regenerate/copy tools to meta
      const meta = node.querySelector('.msg__meta');
      if (meta && !meta.querySelector('.msg__tools')) {
        const tools = el('div', { class: 'msg__tools' });
        const copyBtn = el('button', { class: 'tool', title: 'Copy', html: icon('copy', 'width="15" height="15"') });
        copyBtn.addEventListener('click', () => { navigator.clipboard.writeText(placeholder.content || ''); toast('Copied', 'ok'); });
        const regen = el('button', { class: 'tool', title: 'Regenerate', html: icon('refresh', 'width="15" height="15"') });
        regen.addEventListener('click', () => regenerate());
        tools.append(copyBtn, regen);
        meta.appendChild(tools);
      }
      scrollToBottom();
      loadConversations(); // refresh titles/order
    }
  }

  async function safeError(res) {
    try { const t = await res.text(); const j = t ? JSON.parse(t) : {}; return j.error || `Request failed (${res.status})`; }
    catch { return `Request failed (${res.status})`; }
  }

  async function stopGeneration() {
    if (!state.activeId) return;
    try { await api.post(`/api/conversations/${state.activeId}/stop`, {}); } catch (e) {}
    if (state.controller) state.controller.abort();
  }

  function setGenerating(v) { state.generating = v; updateSendState(); }

  // SSE reader over fetch stream
  async function readSSE(stream, onEvent) {
    const reader = stream.getReader();
    const decoder = new TextDecoder();
    let buf = '';
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      let idx;
      while ((idx = buf.indexOf('\n\n')) !== -1) {
        const frame = buf.slice(0, idx); buf = buf.slice(idx + 2);
        let ev = 'message', data = '';
        for (const line of frame.split('\n')) {
          if (line.startsWith('event:')) ev = line.slice(6).trim();
          else if (line.startsWith('data:')) data += line.slice(5).trim();
        }
        if (!data) continue;
        let parsed; try { parsed = JSON.parse(data); } catch { parsed = { content: data }; }
        onEvent({ event: ev, data: parsed });
      }
    }
  }

  function scrollToBottom(force) {
    const c = $('#convo');
    const nearBottom = c.scrollHeight - c.scrollTop - c.clientHeight < 160;
    if (force || nearBottom) c.scrollTop = c.scrollHeight;
  }

  // ---------------------------------------------------------------- status
  async function refreshStatus() {
    try {
      const h = await api.get('/health');
      $('#dotApp').className = 'dot ' + (h.status === 'healthy' ? 'ok' : h.status === 'degraded' ? 'warn' : 'down');
      $('#dotDb').className = 'dot ' + (h.database.status === 'connected' ? 'ok' : 'down');
      $('#dotMimir').className = 'dot ' + (h.mimir.status === 'connected' ? 'ok' : 'down');
      if (h.mimir.status === 'connected') $('#statusMimir').textContent = 'Inference · ' + (h.mimir.latency_ms ?? '?') + 'ms';
      else $('#statusMimir').textContent = 'Inference unavailable';
      state.model = h.mimir.model || '';
      renderModelChip(state.model);
      $('#statusApp').textContent = h.status === 'healthy' ? `Chatty v${h.version}` : `Chatty · ${h.status}`;
    } catch (e) {
      $('#dotApp').className = 'dot down'; $('#dotDb').className = 'dot down'; $('#dotMimir').className = 'dot down';
      $('#statusApp').textContent = 'Chatty · offline';
    }
  }
  function renderModelChip(model) {
    $('#modelChip').textContent = model ? (model.split('/').pop() || model) : 'no model';
    $('#modelChip').title = model || 'No model detected';
  }

  // ---------------------------------------------------------------- theme/sidebar
  function applyTheme() {
    const t = prefs.theme;
    const dark = t === 'dark' || (t === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
    $('#themeBtn').innerHTML = icon(dark ? 'sun' : 'moon');
    document.querySelector('meta[name=theme-color]').setAttribute('content', dark ? '#101014' : '#fbfbfd');
  }
  function cycleTheme() { prefs.theme = prefs.theme === 'dark' ? 'light' : prefs.theme === 'light' ? 'system' : 'dark'; applyTheme(); }
  function applySidebar() { $('#app').setAttribute('data-sidebar', prefs.sidebar === 'collapsed' ? 'collapsed' : 'expanded'); }
  function toggleSidebar() { prefs.sidebar = prefs.sidebar === 'collapsed' ? 'expanded' : 'collapsed'; applySidebar(); }
  function openDrawer() { $('#app').setAttribute('data-drawer', 'open'); }
  function closeDrawer() { $('#app').removeAttribute('data-drawer'); }

  // ---------------------------------------------------------------- settings modal
  function openSettings() { $('#settingsOverlay').classList.add('open'); loadSettings(); }
  async function loadSettings() {
    let s; try { s = await api.get('/api/settings'); } catch (e) { toast('Could not load settings', 'err'); return; }
    renderSettings(s);
  }
  function renderSettings(s) {
    const body = $('#settingsBody');
    const tab = document.querySelector('.tab.active')?.dataset.tab || 'general';
    body.innerHTML = '';
    if (tab === 'general') body.append(sectionGeneral(), sectionShortcutsLink());
    else if (tab === 'model') body.append(sectionModel(s));
    else body.append(sectionSystem(s));
  }
  function section(title, rows) { const sec = el('div', { class: 'section' }, [el('div', { class: 'section__title', text: title })]); rows.forEach((r) => sec.appendChild(r)); return sec; }
  function field(label, desc, control) {
    const left = el('div', {}, [el('div', { class: 'field__label', text: label })]);
    if (desc) left.appendChild(el('div', { class: 'field__desc', text: desc }));
    return el('div', { class: 'field' }, [left, el('div', { class: 'field__control' }, [control])]);
  }
  function sectionGeneral() {
    const seg = el('div', { class: 'seg' });
    ['dark', 'light', 'system'].forEach((v) => {
      const b = el('button', { class: prefs.theme === v ? 'active' : '', text: v[0].toUpperCase() + v.slice(1), type: 'button' });
      b.addEventListener('click', () => { prefs.theme = v; applyTheme(); renderSettings(currentSettings()); });
      seg.appendChild(b);
    });
    const fs = el('input', { type: 'range', min: '14', max: '19', step: '1', value: String(prefs.fontSize) });
    fs.addEventListener('input', () => { prefs.fontSize = parseInt(fs.value, 10); document.documentElement.style.setProperty('--app-font-size', fs.value + 'px'); });
    const sidebarSwitch = el('div', { class: 'switch ' + (prefs.sidebar === 'expanded' ? 'on' : ''), role: 'switch', tabindex: '0' });
    sidebarSwitch.addEventListener('click', () => { toggleSidebar(); sidebarSwitch.classList.toggle('on', prefs.sidebar === 'expanded'); });
    return section('General', [
      field('Theme', 'Interface appearance.', seg),
      field('Font size', 'Base text size across the app.', fs),
      field('Sidebar', 'Show the navigation sidebar.', sidebarSwitch),
    ]);
  }
  function sectionShortcutsLink() { return section('Tips', [field('Keyboard shortcuts', 'Press ? or the keyboard icon to view them.', el('span', {}))]); }
  function sectionModel(s) {
    const g = s.generation;
    const temp = el('input', { type: 'range', min: '0', max: '2', step: '0.1', value: String(g.temperature) });
    const tempVal = el('span', { class: 'kv', text: g.temperature.toFixed(1) });
    temp.addEventListener('input', () => { tempVal.textContent = parseFloat(temp.value).toFixed(1); });
    temp.addEventListener('change', () => patchSettings({ temperature: parseFloat(temp.value) }));

    const maxTok = el('select');
    [512, 1024, 1536, 2048].forEach((n) => maxTok.appendChild(el('option', { value: n, text: n + ' tokens', ...(n === g.max_tokens ? { selected: '' } : {}) })));
    maxTok.addEventListener('change', () => patchSettings({ max_tokens: parseInt(maxTok.value, 10) }));

    const ctxMsgs = el('select');
    [10, 20, 40, 60].forEach((n) => ctxMsgs.appendChild(el('option', { value: n, text: n + ' messages', ...(n === g.max_context_messages ? { selected: '' } : {}) })));
    ctxMsgs.addEventListener('change', () => patchSettings({ max_context_messages: parseInt(ctxMsgs.value, 10) }));

    const think = el('div', { class: 'switch ' + (g.thinking ? 'on' : ''), role: 'switch', tabindex: '0' });
    think.addEventListener('click', () => { patchSettings({ thinking: !g.thinking }).then(renderSettingsSafe); });
    const modelChip = el('span', { class: 'model-chip', text: s.model.id ? s.model.id.split('/').pop() : '—' });
    return section('Model', [
      field('Active model', 'Served by Mimir.', modelChip),
      field('Endpoint', 'Mimir base URL.', el('span', { class: 'kv', text: s.model.base_url })),
      field('Backend', '', el('span', { class: 'kv', text: s.model.backend })),
      field('Temperature', '0 = focused, 2 = creative.', el('div', { class: 'field__control' }, [temp, tempVal])),
      field('Max output tokens', 'Upper bound on reply length.', maxTok),
      field('Max context messages', 'Conversation history sent to the model.', ctxMsgs),
      field('Thinking (Qwen3)', 'Show model reasoning before the answer.', think),
    ]);
  }
  function sectionSystem(s) {
    const rows = [
      field('Version', '', el('span', { class: 'kv', text: s.system.version })),
      field('Environment', '', el('span', { class: 'kv', text: s.system.env })),
      field('Uptime', '', el('span', { class: 'kv', text: humanDuration(s.system.uptime_seconds) })),
      field('Database', s.database.status === 'connected' ? 'Connected' : 'Unavailable',
        el('span', { class: 'dot ' + (s.database.status === 'connected' ? 'ok' : 'down') })),
      field('Inference (Mimir)', s.mimir.status === 'connected' ? 'Connected' : 'Unavailable',
        el('span', { class: 'dot ' + (s.mimir.status === 'connected' ? 'ok' : 'down') })),
    ];
    return section('System', rows);
  }
  async function renderSettingsSafe() { const s = await api.get('/api/settings'); renderSettings(s); }
  function currentSettings() { return { model: { id: state.model }, system: {}, database: {}, mimir: {}, generation: {} }; }
  async function patchSettings(patch) {
    try { await api.patch('/api/settings', patch); toast('Saved', 'ok'); } catch (e) { toast(e.message, 'err'); }
  }
  function humanDuration(s) {
    s = Math.max(0, Math.floor(s)); const d = Math.floor(s / 86400); const h = Math.floor((s % 86400) / 3600); const m = Math.floor((s % 3600) / 60);
    if (d) return `${d}d ${h}h`; if (h) return `${h}h ${m}m`; if (m) return `${m}m`; return `${s}s`;
  }

  function renderShortcuts() {
    const list = [
      ['Send message', 'Enter / ⌘↵'], ['New line', 'Shift↵'], ['New conversation', '⌘⇧O'], ['Search', '⌘K'],
      ['Stop generation', 'Esc'], ['Toggle sidebar', '⌘B'], ['Open settings', '⌘,'], ['This help', '?'],
    ];
    const body = $('#shortcutsBody'); body.innerHTML = '';
    const sec = el('div', { class: 'section' });
    for (const [k, v] of list) sec.appendChild(el('div', { class: 'kv' }, [el('span', { class: 'k', text: k }), el('span', { class: 'v', text: v })]));
    body.appendChild(sec);
  }

  // ---------------------------------------------------------------- shortcuts
  function onKeydown(e) {
    const mod = e.metaKey || e.ctrlKey;
    const typingInField = e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA';
    if (e.key === 'Escape') {
      if (state.generating) { stopGeneration(); e.preventDefault(); return; }
      if ($('#settingsOverlay').classList.contains('open')) { $('#settingsOverlay').classList.remove('open'); return; }
      if ($('#shortcutsOverlay').classList.contains('open')) { $('#shortcutsOverlay').classList.remove('open'); return; }
      if (ctxOpen) { closeCtxMenu(); return; }
    }
    if (mod && e.key.toLowerCase() === 'enter') { if ($('#input') === document.activeElement) { e.preventDefault(); sendMessage(); } return; }
    if (mod && e.shiftKey && e.key.toLowerCase() === 'o') { e.preventDefault(); newConversation(); return; }
    if (mod && e.key.toLowerCase() === 'k') { e.preventDefault(); $('#searchInput').focus(); $('#searchInput').select(); return; }
    if (mod && e.key.toLowerCase() === 'b') { e.preventDefault(); toggleSidebar(); return; }
    if (mod && e.key === ',') { e.preventDefault(); openSettings(); return; }
    if (!typingInField && e.key === '?') { e.preventDefault(); $('#shortcutsOverlay').classList.add('open'); return; }
    // Up-arrow in empty composer loads last user message.
    if (e.key === 'ArrowUp' && $('#input') === document.activeElement && !input().value && !state.generating) {
      const lastUser = [...state.messages].reverse().find((m) => m.role === 'user');
      if (lastUser) { e.preventDefault(); input().value = lastUser.content; autoGrow(); updateSendState(); }
    }
  }

  // ---------------------------------------------------------------- init
  function wire() {
    $('#brandMark').innerHTML = ICONS.logo;
    $('#searchIcon').innerHTML = icon('search', 'width="15" height="15"');
    $('#newChatBtn').innerHTML = icon('plus', 'width="17" height="17"') + '<span>New conversation</span>';
    $('#settingsBtn').innerHTML = icon('settings');
    $('#shortcutsBtn').innerHTML = icon('keyboard');
    $('#collapseBtn').innerHTML = icon('panel');
    $('#openSidebarBtn').innerHTML = icon('panel');
    $('#renameBtn').innerHTML = icon('pencil');
    $('#exportMdBtn').innerHTML = icon('file');
    $('#exportJsonBtn').innerHTML = icon('braces');
    $('#settingsClose').innerHTML = icon('close');
    $('#shortcutsClose').innerHTML = icon('close');
    $('#settingsClose').innerHTML = '<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>';
    $('#shortcutsClose').innerHTML = $('#settingsClose').innerHTML;
    $('#searchIcon').innerHTML = icon('search', 'width="15" height="15"');

    $('#newChatBtn').addEventListener('click', () => newConversation());
    $('#collapseBtn').addEventListener('click', toggleSidebar);
    $('#openSidebarBtn').addEventListener('click', openDrawer);
    $('#scrim').addEventListener('click', closeDrawer);
    $('#themeBtn').addEventListener('click', cycleTheme);
    $('#settingsBtn').addEventListener('click', openSettings);
    $('#shortcutsBtn').addEventListener('click', () => { renderShortcuts(); $('#shortcutsOverlay').classList.add('open'); });
    $('#settingsClose').addEventListener('click', () => $('#settingsOverlay').classList.remove('open'));
    $('#shortcutsClose').addEventListener('click', () => $('#shortcutsOverlay').classList.remove('open'));
    $('#renameBtn').addEventListener('click', () => renameConversation(state.activeId));
    $('#exportMdBtn').addEventListener('click', () => downloadExport(state.activeId, 'md'));
    $('#exportJsonBtn').addEventListener('click', () => downloadExport(state.activeId, 'json'));
    document.querySelectorAll('.tab').forEach((t) => t.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach((x) => x.classList.remove('active'));
      t.classList.add('active'); renderSettingsSafe();
    }));

    $('#composer').addEventListener('submit', (e) => { e.preventDefault(); if (state.generating) return; sendMessage(); });
    $('#sendBtn').addEventListener('click', (e) => { if ($('#sendBtn').dataset.mode === 'stop') { e.preventDefault(); stopGeneration(); } });
    input().addEventListener('input', () => { autoGrow(); updateSendState(); });
    input().addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey && !e.metaKey && !e.ctrlKey && !e.isComposing) { e.preventDefault(); sendMessage(); }
    });
    $('#searchInput').addEventListener('input', debounce(async (e) => {
      const q = e.target.value.trim();
      if (!q) { renderConversations(); return; }
      try { const d = await api.get('/api/conversations?q=' + encodeURIComponent(q)); renderConversations(d.conversations || []); }
      catch { renderConversations(); }
    }, 220));
    document.addEventListener('keydown', onKeydown);
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => { if (prefs.theme === 'system') applyTheme(); });
    window.addEventListener('hashchange', () => {
      const mm = location.hash.match(/#\/c\/([0-9a-fA-F-]{36})/);
      if (mm) { if (mm[1] !== state.activeId) openConversation(mm[1]); }
      else if (!state.activeId) showWelcome();
    });

    // handle deep link
    const m = location.hash.match(/#\/c\/([0-9a-fA-F-]{36})/);
    if (m) openConversation(m[1]); else showWelcome();
  }

  async function init() {
    // static icons + prefs
    applyTheme(); applySidebar(); updateSendState();
    wire();
    renderModelChip('');
    await Promise.all([loadConversations(), refreshStatus()]);
    setInterval(refreshStatus, 25000);
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init);
  else init();
})();
