// EventSource client for /events SSE. Rolling 100-entry timeline; the
// browser's built-in reconnect handles network drops. No framework dep —
// matches the dashboard's vanilla-JS aesthetic.

(function () {
  const MAX_ENTRIES = 100;
  const listEl = document.getElementById("events-list");
  const statusEl = document.getElementById("events-status");

  function fmtTime(ts) {
    if (!ts) return "";
    const d = new Date(ts);
    if (isNaN(d.getTime())) return "";
    return d.toISOString().slice(11, 23); // HH:MM:SS.mmm
  }

  function fmtLatency(ms) {
    if (!ms) return "";
    if (ms < 1000) return ms + "ms";
    return (ms / 1000).toFixed(2) + "s";
  }

  function appendEvent(ev) {
    const li = document.createElement("li");
    li.className = ev.outcome || "";
    li.innerHTML = ""; // no inline content; build children to avoid XSS
    const ts = document.createElement("span");
    ts.className = "ev-ts";
    ts.textContent = fmtTime(ev.ts);
    const kind = document.createElement("span");
    kind.className = "ev-kind";
    kind.textContent = ev.kind || "";
    const actor = document.createElement("span");
    actor.className = "ev-actor";
    actor.textContent = ev.actor || "";
    const detail = document.createElement("span");
    detail.className = "ev-detail";
    detail.title = ev.detail || "";
    detail.textContent = ev.detail || (ev.target || "");
    const latency = document.createElement("span");
    latency.className = "ev-latency";
    latency.textContent = fmtLatency(ev.latency_ms);
    li.appendChild(ts);
    li.appendChild(kind);
    li.appendChild(actor);
    li.appendChild(detail);
    li.appendChild(latency);
    listEl.insertBefore(li, listEl.firstChild);
    while (listEl.children.length > MAX_ENTRIES) {
      listEl.removeChild(listEl.lastChild);
    }
  }

  const es = new EventSource("/events");

  es.onopen = function () {
    statusEl.textContent = "live";
    statusEl.className = "live";
  };

  es.onerror = function () {
    statusEl.textContent = "reconnecting...";
    statusEl.className = "lost";
  };

  // The server names every frame with `event: <kind>`; the default
  // "message" handler does NOT fire for named events, so attach a
  // wildcard via addEventListener on each kind we see. Simpler: also
  // listen for the generic "message" fallback in case the server emits
  // events with no kind name.
  function handle(e) {
    try {
      appendEvent(JSON.parse(e.data));
    } catch (_err) {
      // Drop malformed payloads; the SSE stream itself stays open.
    }
  }
  es.onmessage = handle;

  // Subscribe to known kinds. The schema-drift test enforces this list
  // matches the bounded enum in event-timeline.md §3.
  const KINDS = [
    "dispatch.ship", "dispatch.review", "dispatch.merge",
    "attestation.attempt", "attestation.granted", "attestation.revoked",
    "audit.append", "audit.rotate",
    "connect.exchange", "connect.refresh", "connect.api_call",
    "voice.speak", "voice.fallback",
    "subagent.spawn", "subagent.complete",
    "reasoner.call", "reasoner.retry",
    "memory.query", "memory.upsert",
    "recommendation.propose", "recommendation.accept", "recommendation.reject", "recommendation.apply",
    "obs.snapshot", "obs.selfcheck", "obs.panic",
  ];
  for (const k of KINDS) {
    es.addEventListener(k, handle);
  }
})();
