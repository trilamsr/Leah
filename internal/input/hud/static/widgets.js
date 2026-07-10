(() => {
  // Server pushes tile HTML over SSE so the page never polls. See B1 in the
  // 2026-06-10 cross-surface UX audit — the previous setInterval(refresh, ttl)
  // hid daemon errors as silent em-dashes and ate battery.

  function swap(id, html) {
    const wrap = document.createElement("div");
    wrap.innerHTML = (html || "").trim();
    const next = wrap.firstElementChild;
    if (!next) return;
    next.id = id;
    const cur = document.getElementById(id);
    if (cur && cur.parentNode) cur.parentNode.replaceChild(next, cur);
  }

  function offlineNode(id) {
    const cur = document.getElementById(id);
    if (!cur) return;
    cur.className = `${cur.className.split(" ").filter((c) => !c.startsWith("widget-")).join(" ")} widget-error`.trim();
    cur.textContent = "couldn't load — retry";
  }

  function openStream() {
    let backoff = 2000;
    // widget.init seeds tileIds before any refresh; refresh frames also union
    // in so a server-side tile addition reaches the offline-marking set
    // without a client release.
    let tileIds = new Set();
    try {
      const es = new EventSource("/api/widgets/stream");
      es.addEventListener("widget.init", (e) => {
        try {
          const ids = JSON.parse(e.data);
          if (Array.isArray(ids)) tileIds = new Set(ids);
        } catch { /* leave previous set */ }
      });
      es.addEventListener("widget.refresh", (e) => {
        backoff = 2000;
        let f;
        try { f = JSON.parse(e.data); } catch { return; }
        if (f && f.id) {
          tileIds.add(f.id);
          swap(f.id, f.html);
        }
      });
      es.onerror = () => {
        es.close();
        tileIds.forEach(offlineNode);
        setTimeout(openStream, backoff);
        backoff = Math.min(backoff * 2, 30000);
      };
    } catch {
      // EventSource unavailable: leave placeholders in place rather than
      // silently looping a polyfill the user didn't ask for.
    }
  }

  openStream();
})();
