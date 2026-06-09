// Vanilla client for leah dashboard. Polls /api/state every 3s; pauses on
// tab blur. No framework, no build, no dependencies. See
// docs/specs/2026-06-09-jarvis-ui.md.

const POLL_MS = 3000;
let timer = null;
let missed = 0;

const $ = (id) => document.getElementById(id);

function fmtTime(iso) {
  if (!iso) return '--:--:--';
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  return d.toTimeString().slice(0, 8);
}

function fmtUptime(s) {
  if (!s) return '--';
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  return `${h}h ${m}m`;
}

function ago(iso) {
  if (!iso) return 'never';
  const sec = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.floor(sec/60)}m ago`;
  return `${Math.floor(sec/3600)}h ago`;
}

function outcomeClass(o) {
  if (o === 'fail' || o === 'error') return 'outcome-fail';
  if (o === 'warn' || o === 'denied') return 'outcome-warn';
  return 'outcome-ok';
}

function renderAudit(rows) {
  const el = $('audit-list');
  if (!rows || !rows.length) { el.innerHTML = '<li class="ts">(empty)</li>'; return; }
  el.innerHTML = rows.slice().reverse().map(r => `
    <li>
      <span class="ts">${fmtTime(r.ts)}</span>
      <span class="kind">${escape(r.kind)}</span>
      <span class="detail ${outcomeClass(r.outcome)}">${escape(r.detail || r.outcome || '')}</span>
    </li>`).join('');
}

function renderAgents(rows) {
  const el = $('agents-list');
  if (!rows || !rows.length) { el.innerHTML = '<li class="ts">(no agents)</li>'; return; }
  el.innerHTML = rows.map(a => `
    <li>
      <span class="kind">${escape(a.id)}</span>
      <span class="detail ${outcomeClass(a.state)}">${escape(a.state)}</span>
      <span class="ts">${a.pr ? 'PR #' + a.pr : ''}</span>
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
    ? dec.map(d => `<li><span class="kind">${escape(d.topic)}</span><span class="detail">${escape(d.choice)}</span></li>`).join('')
    : '<li class="ts">(none)</li>';
}

function renderOps(o) {
  if (!o) return;
  const spent = o.budget_spent || 0;
  const ceiling = o.budget_ceiling || 1;
  const pct = Math.min(100, Math.round((spent / ceiling) * 100));
  $('ops-budget').innerHTML = `<span class="label">budget</span><span class="val">$${spent.toFixed(2)} / $${ceiling.toFixed(2)}</span>`;
  const bar = $('ops-bar');
  bar.style.width = pct + '%';
  bar.className = 'fill' + (pct >= 90 ? ' fail' : pct >= 70 ? ' warn' : '');
  $('ops-heart').innerHTML = `<span class="label">heart</span><span class="val">${ago(o.last_heartbeat_at)}</span>`;
  $('ops-uptime').innerHTML = `<span class="label">daemon</span><span class="val">up ${fmtUptime(o.daemon_uptime_seconds)}</span>`;

  const aliveAgeSec = o.last_heartbeat_at ? Math.floor((Date.now() - new Date(o.last_heartbeat_at).getTime()) / 1000) : 9999;
  const dot = $('alive');
  dot.className = 'dot alive-dot' + (aliveAgeSec > 300 ? ' fail' : aliveAgeSec > 60 ? ' warn' : '');
}

function escape(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({
    '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
  }[c]));
}

function showBanner(msg) {
  const b = $('banner');
  b.textContent = msg;
  b.hidden = false;
}
function hideBanner() { $('banner').hidden = true; }

async function tick() {
  try {
    const r = await fetch('/api/state', { cache: 'no-store' });
    if (!r.ok) throw new Error('HTTP ' + r.status);
    const data = await r.json();
    missed = 0;
    hideBanner();
    renderAudit(data.audit);
    renderAgents(data.agents);
    renderMemory(data.memory);
    renderOps(data.ops);
  } catch (e) {
    missed++;
    if (missed >= 2) showBanner('daemon unreachable (' + e.message + ')');
  }
}

function start() {
  tick();
  timer = setInterval(tick, POLL_MS);
}
function stop() {
  if (timer) { clearInterval(timer); timer = null; }
}

document.addEventListener('visibilitychange', () => {
  if (document.hidden) { document.body.classList.add('paused'); stop(); }
  else { document.body.classList.remove('paused'); start(); }
});

start();
