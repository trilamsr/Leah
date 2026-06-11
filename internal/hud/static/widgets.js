(() => {
  // TTLs mirror internal/hud.TTLs(); keep in sync with spec §6.
  const tiles = [
    { id: "weather",     url: "/api/widgets/weather",            ttl: 600000, kind: "weather"  },
    { id: "market-AAPL", url: "/api/widgets/market?symbol=AAPL", ttl: 60000,  kind: "market"   },
    { id: "news",        url: "/api/widgets/news",               ttl: 900000, kind: "news"     },
    { id: "calendar",    url: "/api/widgets/calendar-next",      ttl: 30000,  kind: "calendar" },
  ];

  function errorNode(tile) {
    const div = document.createElement("div");
    div.id = tile.id;
    div.className = `widget ${tile.kind} widget-error`;
    div.textContent = "couldn't load — retry";
    return div;
  }

  function swap(tile, node) {
    node.id = tile.id;
    const old = document.getElementById(tile.id);
    if (old && old.parentNode) old.parentNode.replaceChild(node, old);
  }

  async function refresh(tile) {
    try {
      const r = await fetch(tile.url, { cache: "no-store" });
      if (!r.ok) { swap(tile, errorNode(tile)); return; }
      const txt = await r.text();
      const wrap = document.createElement("div");
      wrap.innerHTML = txt.trim();
      const node = wrap.firstElementChild;
      if (!node) { swap(tile, errorNode(tile)); return; }
      swap(tile, node);
    } catch {
      swap(tile, errorNode(tile));
    }
  }

  tiles.forEach((t) => {
    refresh(t);
    setInterval(() => refresh(t), t.ttl);
  });
})();
