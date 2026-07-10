(function () {
  const listEl = document.getElementById("recs-list");
  const emptyEl = document.getElementById("recs-empty");
  const statusEl = document.getElementById("recs-status");

  function setStatus(s) { statusEl.textContent = s; }

  function buildRow(r) {
    const li = document.createElement("li");
    li.className = "card";
    li.dataset.id = r.id;

    const pat = document.createElement("span");
    pat.className = "pattern";
    pat.textContent = r.pattern;
    li.appendChild(pat);

    const buttons = document.createElement("span");
    buttons.className = "buttons";
    const accept = document.createElement("button");
    accept.type = "button";
    accept.className = "accept";
    accept.textContent = "Accept";
    const reject = document.createElement("button");
    reject.type = "button";
    reject.className = "reject";
    reject.textContent = "Reject";
    buttons.appendChild(accept);
    buttons.appendChild(reject);
    li.appendChild(buttons);

    const act = document.createElement("span");
    act.className = "action";
    act.textContent = r.action || "";
    li.appendChild(act);

    const meta = document.createElement("span");
    meta.className = "meta";
    const tier = document.createElement("span");
    tier.className = "tier-" + (r.tier || "auto");
    tier.textContent = r.tier || "";
    const src = document.createElement("span");
    src.textContent = r.source || "";
    const conf = document.createElement("span");
    conf.textContent = "p=" + (typeof r.confidence === "number" ? r.confidence.toFixed(2) : "?");
    meta.appendChild(tier);
    meta.appendChild(src);
    meta.appendChild(conf);
    li.appendChild(meta);

    accept.addEventListener("click", () => act_on(r.id, "accept", li));
    reject.addEventListener("click", () => act_on(r.id, "reject", li));
    return li;
  }

  function render(recs) {
    listEl.innerHTML = "";
    if (!recs || recs.length === 0) {
      emptyEl.hidden = false;
      setStatus("");
      return;
    }
    emptyEl.hidden = true;
    for (const r of recs) listEl.appendChild(buildRow(r));
    setStatus(recs.length + " pending");
  }

  function upsert(rec) {
    if (!rec || !rec.id) return;
    const existing = listEl.querySelector(`[data-id="${CSS.escape(rec.id)}"]`);
    if (existing) {
      existing.replaceWith(buildRow(rec));
    } else {
      emptyEl.hidden = true;
      listEl.appendChild(buildRow(rec));
    }
    setStatus(listEl.children.length + " pending");
  }

  function act_on(id, verb, li) {
    const btns = li.querySelectorAll("button");
    btns.forEach((b) => (b.disabled = true));
    fetch("/hud/recommendations/" + encodeURIComponent(id) + "/" + verb, { method: "POST" })
      .then((r) => {
        if (!r.ok && r.status !== 204) throw new Error(verb + " failed: " + r.status);
        li.remove();
        if (listEl.children.length === 0) {
          emptyEl.hidden = false;
          setStatus("");
        } else {
          setStatus(listEl.children.length + " pending");
        }
      })
      .catch((e) => {
        setStatus(String(e));
        btns.forEach((b) => (b.disabled = false));
      });
  }

  function load() {
    setStatus("loading…");
    fetch("/hud/recommendations", { headers: { Accept: "application/json" } })
      .then((r) => {
        if (r.status === 204) return [];
        if (!r.ok) throw new Error("list failed: " + r.status);
        return r.json();
      })
      .then(render)
      .catch((e) => setStatus(String(e)));
  }

  function openStream() {
    try {
      const es = new EventSource("/api/events");
      es.addEventListener("recommendation.propose", (e) => {
        try {
          const evt = JSON.parse(e.data);
          const rec = evt.payload || evt;
          upsert({
            id: rec.id || rec.ID || evt.ref_id,
            pattern: rec.pattern || rec.Pattern || evt.detail,
            action: rec.action || rec.Action,
            tier: rec.tier || rec.Tier,
            source: rec.source || rec.Source,
            confidence: rec.confidence !== undefined ? rec.confidence : rec.Confidence,
          });
        } catch {}
      });
      es.onerror = () => { es.close(); setTimeout(openStream, 2000); };
    } catch {}
  }

  load();
  openStream();
})();
