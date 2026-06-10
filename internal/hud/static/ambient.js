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

  function applyEvent(payload) {
    let m;
    try { m = JSON.parse(payload); } catch { return; }
    if (!m || !m.kind) return;
    switch (m.kind) {
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
        $("state").textContent = m.value || "ambient";
        $("listen").classList.toggle("active", !!m.listening);
        $("think").classList.toggle("active", !!m.thinking);
        break;
    }
  }

  function openStream() {
    try {
      const es = new EventSource("/api/events");
      es.onmessage = (e) => applyEvent(e.data);
      es.onerror = () => { es.close(); setTimeout(openStream, 2000); };
    } catch {
      // EventSource unavailable: fall back to no-op; metrics poll still runs.
    }
  }

  async function pollMetrics() {
    try {
      const r = await fetch("/api/state", { cache: "no-store" });
      if (!r.ok) return;
      const j = await r.json();
      if (j.state) $("state").textContent = j.state;
    } catch {}
  }

  tickClock();
  setInterval(tickClock, 1000);
  openStream();
  pollMetrics();
  setInterval(pollMetrics, 5000);
})();
