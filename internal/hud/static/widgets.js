(() => {
  // TTLs mirror internal/hud.TTLs(); keep in sync with spec §6.
  const tiles = [
    { id: "weather",     url: "/api/widgets/weather",            ttl: 600000 },
    { id: "market-AAPL", url: "/api/widgets/market?symbol=AAPL", ttl: 60000  },
    { id: "news",        url: "/api/widgets/news",               ttl: 900000 },
    { id: "calendar",    url: "/api/widgets/calendar-next",      ttl: 30000  },
  ];

  async function refresh(tile) {
    try {
      const r = await fetch(tile.url, { cache: "no-store" });
      if (!r.ok) return;
      const txt = await r.text();
      const wrap = document.createElement("div");
      wrap.innerHTML = txt.trim();
      const node = wrap.firstElementChild;
      if (!node) return;
      // Preserve the slot id so the next refresh finds the tile we just swapped in.
      node.id = tile.id;
      const old = document.getElementById(tile.id);
      if (old && old.parentNode) old.parentNode.replaceChild(node, old);
    } catch {
      // Network flap: leave the prior tile in place.
    }
  }

  tiles.forEach((t) => {
    refresh(t);
    setInterval(() => refresh(t), t.ttl);
  });
})();
