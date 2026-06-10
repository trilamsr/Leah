(() => {
  const $ = (id) => document.getElementById(id);

  function tickClock() {
    const d = new Date();
    const hh = String(d.getHours()).padStart(2, "0");
    const mm = String(d.getMinutes()).padStart(2, "0");
    $("clock").textContent = `${hh}:${mm}`;
    $("date").textContent = d.toLocaleDateString(undefined, {
      weekday: "short", month: "short", day: "numeric",
    });
  }

  function applyEvent(kind, payload) {
    let m;
    try { m = JSON.parse(payload); } catch { return; }
    if (!m) return;
    const k = kind || m.kind;
    switch (k) {
      case "weather":
        if (m.location) $("loc").textContent = m.location;
        if (m.condition) $("cond").textContent = m.condition;
        if (m.temp) $("temp").textContent = m.temp;
        break;
      case "calendar":
        $("cal-time").textContent = m.time || "—";
        $("cal-title").textContent = m.title || "no upcoming event";
        break;
      case "news":
        if (m.text) $("headline").textContent = m.text;
        break;
      case "market":
        if (m.text) $("ticker").textContent = m.text;
        break;
      case "state":
      case "hud.state": {
        const p = m.payload || m;
        const stateName = p.state || p.value || "ambient";
        $("state").textContent = stateName;
        $("listen").classList.toggle("active", !!p.listening);
        $("think").classList.toggle("active", !!p.thinking);
        break;
      }
    }
  }

  function openStream() {
    try {
      const es = new EventSource("/api/events");
      es.onmessage = (e) => applyEvent(null, e.data);
      es.addEventListener("hud.state", (e) => applyEvent("hud.state", e.data));
      es.onerror = () => { es.close(); setTimeout(openStream, 2000); };
    } catch {
      // EventSource unavailable: no fallback — keep the page static.
    }
  }

  tickClock();
  setInterval(tickClock, 1000);
  openStream();
})();
