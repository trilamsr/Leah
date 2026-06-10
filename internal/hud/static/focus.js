(() => {
  const $ = (id) => document.getElementById(id);
  const IDLE_MS = 30_000;
  let idleTimer = null;

  function resetIdle() {
    if (idleTimer) clearTimeout(idleTimer);
    idleTimer = setTimeout(() => { window.location.href = "/hud/ambient"; }, IDLE_MS);
  }

  function renderResponse(data) {
    const r = $("response");
    r.classList.remove("err");
    r.innerHTML = "";
    const p = document.createElement("p");
    p.textContent = data.text || "(no reply)";
    r.appendChild(p);

    const srcEl = $("sources");
    const srcList = $("sources-list");
    srcList.innerHTML = "";
    if (Array.isArray(data.sources) && data.sources.length) {
      for (const s of data.sources) {
        const li = document.createElement("li");
        const a = document.createElement("a");
        a.href = s.url || "#";
        a.textContent = s.name || s.url || "source";
        a.target = "_blank";
        a.rel = "noopener";
        li.appendChild(a);
        srcList.appendChild(li);
      }
      srcEl.hidden = false;
    } else {
      srcEl.hidden = true;
    }

    const suggEl = $("suggestions");
    const chips = $("chips");
    chips.innerHTML = "";
    if (Array.isArray(data.suggestions) && data.suggestions.length) {
      for (const s of data.suggestions) {
        const b = document.createElement("button");
        b.type = "button";
        b.className = "chip";
        b.textContent = s;
        b.addEventListener("click", () => { $("q").value = s; submit(); });
        chips.appendChild(b);
      }
      suggEl.hidden = false;
    } else {
      suggEl.hidden = true;
    }
  }

  async function submit() {
    const q = $("q").value.trim();
    if (!q) return;
    $("status").textContent = "thinking…";
    resetIdle();
    try {
      const r = await fetch("/api/focus/query", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query: q }),
      });
      const j = await r.json();
      renderResponse(j);
      $("status").textContent = r.ok ? "ready" : "fallback";
    } catch (e) {
      $("response").classList.add("err");
      $("response").textContent = "Reasoner unreachable.";
      $("status").textContent = "offline";
    }
  }

  $("ask").addEventListener("submit", (e) => { e.preventDefault(); submit(); });
  $("dismiss").addEventListener("click", () => { window.location.href = "/hud/ambient"; });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape") window.location.href = "/hud/ambient";
  });
  document.addEventListener("mousemove", resetIdle);
  document.addEventListener("keydown", resetIdle);

  $("q").focus();
  resetIdle();
})();
