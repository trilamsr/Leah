// Vanilla client for the leah dashboard. Polls /api/state every 3s; pauses on
// tab blur. No framework, no build. See docs/specs/2026-06-09-jarvis-ui.md.

const POLL_MS = 3000;
let timer = null;
let missed = 0;
let lastOkAt = null;
let topAuditTs = null;          // tracks newest row ts to flash diffs
let booted = false;

const $ = (id) => document.getElementById(id);

function fmtTime(iso) {
  if (!iso) return '--:--:--';
  const d = new Date(iso);
  return isNaN(d) ? iso : d.toTimeString().slice(0, 8);
}
function fmtUptime(s) {
  if (!s) return '--';
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60);
  return `${h}h ${m}m`;
}
function ago(iso) {
  if (!iso) return 'never';
  const sec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (sec < 5) return 'just now';
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.floor(sec/60)}m ago`;
  return `${Math.floor(sec/3600)}h ago`;
}
function fmtMoney(n) {
  // $0.0123 style — 4 dp, trims trailing zeros after the 2nd dp.
  const v = Number(n || 0);
  return '$' + v.toFixed(4).replace(/(\.\d{2}[1-9]*?)0+$/, '$1').replace(/\.$/, '');
}
function outcomeClass(o) {
  if (o === 'fail' || o === 'error') return 'outcome-fail';
  if (o === 'warn' || o === 'denied') return 'outcome-warn';
  return 'outcome-ok';
}
function esc(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
    '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
  }[c]));
}

function emptyLi(msg) { return `<li class="empty">${esc(msg)}</li>`; }
function skeletonLi() { return '<li><div class="skeleton" style="flex:1"></div></li>'.repeat(4); }

function renderAudit(rows) {
  const el = $('audit-list');
  if (!rows || !rows.length) {
    el.innerHTML = emptyLi('awaiting activity — try `leah ask` to start');
    topAuditTs = null;
    return;
  }
  const reversed = rows.slice().reverse();        // newest first
  const newest = reversed[0].ts;
  const isNew = booted && newest && newest !== topAuditTs;
  el.innerHTML = reversed.map((r, i) => `
    <li${isNew && i === 0 ? ' class="flash"' : ''}>
      <span class="ts">${esc(fmtTime(r.ts))}</span>
      <span class="kind">${esc(r.kind)}</span>
      <span class="detail ${outcomeClass(r.outcome)}" title="${esc(r.detail || r.outcome || '')}">${esc(r.detail || r.outcome || '')}</span>
    </li>`).join('');
  topAuditTs = newest;
}

function renderAgents(rows) {
  const el = $('agents-list');
  if (!rows || !rows.length) { el.innerHTML = emptyLi('no agents spawned'); return; }
  el.innerHTML = rows.map(a => `
    <li>
      <span class="kind">${esc(a.id)}</span>
      <span class="detail ${outcomeClass(a.state)}">${esc(a.state)}</span>
      <span class="ts">${a.pr ? 'PR #' + esc(a.pr) : ''}</span>
    </li>`).join('');
}

function renderMemory(m) {
  if (!m) return;
  $('memory-counts').innerHTML = `
    <div>contacts<strong>${m.contacts || 0}</strong></div>
    <div>projects<strong>${m.projects || 0}</strong></div>
    <div>decisions<strong>${m.decisions || 0}</strong></div>`;
  const dec = m.recent_decisions || [];
  $('memory-decisions').innerHTML = dec.length
    ? dec.map(d => `<li><span class="kind">${esc(d.topic)}</span><span class="detail" title="${esc(d.choice)}">${esc(d.choice)}</span></li>`).join('')
    : emptyLi('no decisions captured yet');
}

function renderOps(o) {
  if (!o) return;
  const spent = o.budget_spent || 0;
  const ceiling = o.budget_ceiling || 1;
  const pct = Math.min(100, Math.round((spent / ceiling) * 100));
  $('ops-budget').innerHTML = `<span class="label">budget</span><span class="val">${esc(fmtMoney(spent))} / ${esc(fmtMoney(ceiling))}</span>`;
  const bar = $('ops-bar');
  bar.style.width = pct + '%';
  bar.className = 'fill' + (pct >= 90 ? ' fail' : pct >= 70 ? ' warn' : '');
  $('ops-heart').innerHTML = `<span class="label">heart</span><span class="val">${esc(ago(o.last_heartbeat_at))}</span>`;
  $('ops-uptime').innerHTML = `<span class="label">daemon</span><span class="val">up ${esc(fmtUptime(o.daemon_uptime_seconds))}</span>`;
  const aliveAgeSec = o.last_heartbeat_at
    ? Math.floor((Date.now() - new Date(o.last_heartbeat_at).getTime()) / 1000)
    : 9999;
  $('alive').className = 'dot alive-dot' + (aliveAgeSec > 300 ? ' fail' : aliveAgeSec > 60 ? ' warn' : '');
}

function renderCosts(c) {
  // Costs is always present (zeroed) — but guard for old-payload safety.
  if (!c) return;
  $('ops-cost-today').innerHTML = `<span class="label">cost 24h</span><span class="val">${esc(fmtMoney(c.today_usd))}</span>`;
  $('ops-cost-week').innerHTML  = `<span class="label">cost 7d</span><span class="val">${esc(fmtMoney(c.week_usd))}</span>`;
  const top = c.top_kinds || [];
  const txt = top.length
    ? top.map(b => `${esc(b.name)} ${esc(fmtMoney(b.usd))}`).join(' · ')
    : 'none';
  $('ops-cost-top').innerHTML = `<span class="label">top kinds</span><span class="val" title="${esc(txt)}">${txt}</span>`;
}

function renderMetrics(m) {
  const el = $('ops-metrics');
  if (!el) return;
  if (!m || !m.counters) { el.innerHTML = ''; return; }
  // Surface the two highest-signal counters; rest available via raw /api/state.
  const c = m.counters;
  const panics = Object.entries(c)
    .filter(([k]) => k.startsWith('leah_panic_total'))
    .reduce((a, [, v]) => a + v, 0);
  const entries = Object.entries(c).slice(0, 1);
  const head = entries.length
    ? `${esc(entries[0][0].split('|')[0])}=${esc(String(entries[0][1]))}`
    : 'no metrics';
  el.innerHTML = `<span class="label">metrics</span><span class="val">${head} · panics=${esc(String(panics))}</span>`;
}

function showBoot() {
  $('audit-list').innerHTML = skeletonLi();
  $('agents-list').innerHTML = skeletonLi();
  $('memory-decisions').innerHTML = skeletonLi();
}

function showBanner(detail) {
  const seen = lastOkAt ? ago(lastOkAt) : 'never';
  $('banner').textContent = `daemon unreachable — last seen ${seen} — ${detail}`;
  $('banner').hidden = false;
}
function hideBanner() { $('banner').hidden = true; }

async function tick() {
  try {
    const r = await fetch('/api/state', { cache: 'no-store' });
    if (!r.ok) throw new Error('HTTP ' + r.status);
    const data = await r.json();
    missed = 0;
    lastOkAt = new Date().toISOString();
    hideBanner();
    renderAudit(data.audit);
    renderAgents(data.agents);
    renderMemory(data.memory);
    renderOps(data.ops);
    renderCosts(data.costs);
    renderMetrics(data.metrics);
    if (!booted) { booted = true; document.body.classList.remove('boot'); }
  } catch (e) {
    missed++;
    if (missed >= 2) showBanner(e.message);
  }
}

function start() { tick(); timer = setInterval(tick, POLL_MS); }
function stop()  { if (timer) { clearInterval(timer); timer = null; } }

document.addEventListener('visibilitychange', () => {
  if (document.hidden) { document.body.classList.add('paused'); stop(); }
  else { document.body.classList.remove('paused'); start(); }
});

showBoot();
start();
