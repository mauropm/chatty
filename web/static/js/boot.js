/* Chatty pre-paint bootstrap: applies persisted theme + font size before the
 * first render to avoid a flash of the wrong theme. External file so the
 * strict Content-Security-Policy (script-src 'self') allows it. */
(function () {
  try {
    var t = localStorage.getItem('chatty.theme') || 'system';
    var dark = t === 'dark' || (t === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
    document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light');
    var fs = localStorage.getItem('chatty.fontSize');
    if (fs) document.documentElement.style.setProperty('--app-font-size', fs + 'px');
  } catch (e) { /* localStorage may be unavailable */ }
})();
