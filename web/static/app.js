// v0.22 — консоль orchestrator: раны, live-терминал, контекст, DAG.
// Без внешних зависимостей (офлайн: всё встроено).
const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));

let state = {
  tab: 'runs',
  runs: [],
  currentRun: null,
  journal: { events: [], since: 0, total: 0 },
  autoScroll: true,
  pipelines: [],
  currentPipe: null,
  timers: {},
};

async function api(path, opts = {}) {
  const r = await fetch(path, opts);
  const ct = r.headers.get('content-type') || '';
  const body = ct.includes('json') ? await r.json() : await r.text();
  if (!r.ok) throw new Error(typeof body === 'string' ? body : JSON.stringify(body));
  return body;
}

// ── header ───────────────────────────────────────────────────────────────
async function init() {
  try {
    const h = await api('/api/health');
    $('#ver').textContent = 'v' + h.version;
  } catch { $('#ver').textContent = 'оффлайн'; }
  $('#tab-runs').onclick = () => setTab('runs');
  $('#tab-pipelines').onclick = () => setTab('pipelines');
  $('#run-btn').onclick = startRun;
  loadPipelines();
  tickRuns();
  state.timers.runs = setInterval(tickRuns, 2500);
}

function setTab(t) {
  state.tab = t;
  $('#tab-runs').classList.toggle('active', t === 'runs');
  $('#tab-pipelines').classList.toggle('active', t === 'pipelines');
  $('#runs-aside').style.display = t === 'runs' ? '' : 'none';
  $('#pip-aside').style.display = t === 'pipelines' ? '' : 'none';
  $('#detail').style.display = t === 'runs' ? '' : 'none';
  $('#pdetail').style.display = t === 'pipelines' ? '' : 'none';
}

// ── раны ─────────────────────────────────────────────────────────────────
async function tickRuns() {
  let runs;
  try { runs = await api('/api/runs'); } catch { return; }
  state.runs = runs;
  // новый ран после запуска из браузера → открыть его деталку
  if (state.timers.pendingNew) {
    const fresh = runs.find(r => state.runIdsBeforeStart.indexOf(r.id) < 0);
    if (fresh) {
      state.timers.pendingNew = false;
      openRunDetail(fresh.id, true);
      const rs = $('#run-status');
      if (rs) rs.textContent = 'идёт — смотри таймлайн справа';
    }
  }
  renderRuns();
  const btn = $('#run-btn');
  const anyRunning = runs.some(r => r.status === 'running');
  if (!anyRunning && btn && btn.disabled) {
    btn.disabled = false;
    const rs = $('#run-status');
    if (rs && /идёт|старт/.test(rs.textContent)) rs.textContent = '';
  }
}

function renderRuns() {
  const el = $('#runs-list');
  if (!state.runs.length) { el.innerHTML = '<div class="empty">ранов пока нет</div>'; return; }
  el.innerHTML = state.runs.slice(0, 60).map(r => {
    const st = r.status === 'ok' ? 'ok' : r.status === 'running' ? 'run' : r.status === 'aborted' ? 'err' : r.status === 'failed' ? 'err' : 'skip';
    const label = r.status === 'running' ? 'идёт…' : r.status;
    const t = (r.last || r.started || '').replace('T', ' ').replace('Z', '');
    return `<div class="run-item ${state.currentRun === r.id ? 'active' : ''}" onclick="openRunDetail('${r.id}',true)">
      <div class="top"><span class="pipeline">${esc(r.pipeline || '?')}</span><span class="badge ${st}">${label}</span></div>
      <div class="meta">${esc(r.id)} · ${r.steps} ш. · ${esc(t)}</div>
    </div>`;
  }).join('');
  $('#runs-count').textContent = `(${state.runs.length})`;
}

// ── запуск из браузера ───────────────────────────────────────────────────
async function loadPipelines() {
  try {
    state.pipelines = await api('/api/pipelines');
    const sel = $('#run-select');
    sel.innerHTML = state.pipelines
      .filter(p => !p.error)
      .map(p => `<option value="${esc(p.file)}">${esc(p.name || p.file)} — ${p.steps} ш.${p.foreach ? ' · foreach' : ''}</option>`)
      .join('');
    renderPipList();
  } catch (e) { console.error(e); }
}

function renderPipList() {
  const el = $('#pip-list');
  el.innerHTML = state.pipelines.map(p =>
    `<div class="pitem ${state.currentPipe === p.file ? 'active' : ''}" onclick="openPipeline('${esc(p.file)}')">
      ${esc(p.name || p.file)} <small>${p.steps} ш.${p.foreach ? ' · foreach ' + esc(p.foreach) : ''}${p.error ? ' · error' : ''}</small>
    </div>`).join('');
  $('#pip-count').textContent = `(${state.pipelines.length})`;
}

async function startRun() {
  const file = $('#run-select').value;
  if (!file) return;
  const btn = $('#run-btn');
  btn.disabled = true;
  $('#run-status').textContent = 'старт…';
  try {
    state.runIdsBeforeStart = state.runs.map(r => r.id);
    state.runsBeforeStart = state.runs.length;
    state.timers.pendingNew = true;
    const yes = !$('#run-gate').checked;
    await api('/api/run', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({file, yes}) });
    $('#run-status').textContent = yes ? 'ран запущен (--yes) — ищем его в списке…' : 'ран запущен — гейты будут решать прямо здесь…';
  } catch (e) {
    $('#run-status').innerHTML = '<span style="color:var(--err)">' + esc(e.message) + '</span>';
    btn.disabled = false;
    state.timers.pendingNew = false;
  }
}

// ── деталка рана: таймлайн + контекст + журнал ───────────────────────────
let stateDetailStatus = '';
async function openRunDetail(id, force) {
  if (state.currentRun === id && !force) return;
  state.currentRun = id;
  renderRuns();
  let d;
  try { d = await api('/api/runs/' + id); } catch (e) { return; }
  state.detailStatus = d.status;
  const st = d.status === 'ok' ? 'ok' : d.status === 'running' ? 'run' : 'err';
  const label = d.status === 'running' ? 'идёт…' : d.status;
  const ctx = d.context || {};
  const steps = (ctx.steps && Object.keys(ctx.steps).length) || 0;
  const inp = ctx.input ? Object.keys(ctx.input).length : 0;

  $('#detail').innerHTML = `
    <div id="gate-card" style="display:none"></div>
    <div class="dhead">
      <button class="btn" onclick="closeDetail()">←</button>
      <h2>${esc(d.pipeline || '?')}</h2>
      <span class="badge ${st}">${label}</span>
      <span class="sub">${esc(id)} · контекст: input ${inp} полей, steps ${steps}</span>
    </div>
    <div class="cols">
      <div class="card">
        <h3>Таймлайн (журнал)</h3>
        <div class="tl" id="tl">${renderTimeline(d.events)}</div>
      </div>
      <div class="card">
        <h3>
          Контекст / Журнал
          <span class="rtabs">
            <button id="rt-ctx" class="active" onclick="showRTab('ctx')">контекст</button>
            <button id="rt-jnl" onclick="showRTab('jnl')">журнал <span id="jnl-n"></span></button>
          </span>
        </h3>
        <div class="ctx" id="rt-ctx-body">${renderCtx(ctx)}</div>
        <div class="jnl" id="rt-jnl-body" style="display:none">
          <div style="margin-bottom:8px"><span class="autochip on" id="autochip" onclick="toggleAuto()">авто-скролл</span>
          <span style="font-size:10px;color:var(--dim);margin-left:8px">live: обновление каждые 2 c</span></div>
          <div id="jnl-lines"></div>
        </div>
      </div>
    </div>`;
  // журнал: полный прогон + polling хвоста
  state.journal = { events: d.events, since: d.events.length, total: d.events.length };
  renderJournal(d.events);
  updateGateCard(id);
  clearInterval(state.timers.journal);
  state.timers.journal = setInterval(async () => {
    if (state.currentRun !== id || state.tab !== 'runs') { clearInterval(state.timers.journal); return; }
    try {
      const tail = await api(`/api/runs/${id}/journal?since=${state.journal.since}`);
      if (tail.events.length) {
        state.journal.since = tail.total;
        state.journal.events = state.journal.events.concat(tail.events);
        const tl = $('#tl'); if (tl) tl.innerHTML = renderTimeline(state.journal.events);
        renderJournal(tail.events, true);
        updateGateCard(id);
        const d2 = state.journal.events[state.journal.events.length - 1];
        if (d2 && (d2.type === 'run_end' || d2.type === 'run_failed')) {
          const dd = await api('/api/runs/' + id);
          state.detailStatus = dd.status;
          tickRuns();
        }
      }
      const jn = $('#jnl-n'); if (jn) jn.textContent = `(${state.journal.total})`;
    } catch {}
  }, 2000);
}

window.closeDetail = () => {
  state.currentRun = null;
  clearInterval(state.timers.journal);
  renderRuns();
  $('#detail').innerHTML = '<div class="empty">Выбери ран слева — таймлайн, контекст и live-журнал появятся здесь</div>';
};

function renderTimeline(events) {
  let html = '';
  let inItem = -1, inPar = false;
  const t = e => (e.ts || '').split('T')[1] || '';
  for (const e of events) {
    const type = e.type;
    if (type === 'item_start') {
      if (inItem >= 0) html += '</div>';
      if (inPar) { html += '</div>'; inPar = false; }
      inItem = e.item_index;
      html += `<div class="item"><div class="hd">элемент ${e.item_index}${e.item != null ? ' · ' + esc(JSON.stringify(e.item)) : ''}</div>`;
      continue;
    }
    if (type === 'item_end' || type === 'item_aborted') {
      if (inItem >= 0) { html += '</div>'; inItem = -1; }
      const bad = type === 'item_aborted';
      html += ev(t(e), bad ? 'err' : 'ok', `${bad ? 'элемент прерван' : 'элемент завершён'}`, e.status || '');
      continue;
    }
    if (type === 'parallel_start') { inPar = true; html += `<div class="par"><div class="hd">‖ параллельно: ${esc((e.steps || []).join(', '))}</div>`; continue; }
    if (type === 'parallel_end') { if (inPar) html += '</div>'; inPar = false; html += ev(t(e), 'par', `‖ группа завершена за ${e.duration_ms ?? '?'} мс`, e.statuses ? esc(JSON.stringify(e.statuses)) : ''); continue; }
    if (type === 'foreach_item_start') { html += ev(t(e), 'dim', `⤷ ${esc(e.step)} · элемент ${e.item_index}`, ''); continue; }
    if (type === 'foreach_item_end') { html += ev(t(e), 'dim', `⤷ ${esc(e.step)} · элемент ${e.item_index} → ${esc(e.status)}`, e.duration_ms != null ? e.duration_ms + ' мс' : ''); continue; }
    if (type === 'step_start') { html += ev(t(e), 'run', `▶ ${esc(e.step)}`, e.attempt > 1 ? `повтор ${e.attempt}` : ''); continue; }
    if (type === 'step_end') {
      const cls = e.status === 'ok' ? 'ok' : 'err';
      html += ev(t(e), cls, `${e.status === 'ok' ? '✓' : '✗'} ${esc(e.step)}`, `${e.duration_ms ?? '?'} мс · exit ${e.exit_code}${e.error ? ' · ' + esc(e.error) : ''}`);
      continue;
    }
    if (type === 'step_failed') { html += ev(t(e), 'err', `✗ ${esc(e.step)}: ${esc(e.error || 'ошибка')}`, ''); continue; }
    if (type === 'step_skipped') { html += ev(t(e), 'skip', `↷ ${esc(e.step)} пропущен`, e.reason ? `reason: ${esc(e.reason)}${e.condition ? ' · ' + esc(e.condition) : ''}` : ''); continue; }
    if (type === 'gate_wait') { html += ev(t(e), 'run', `👤 гейт ${esc(e.step)}: ожидает решение (в браузере)`, (e.actions || []).join('/') ); continue; }
    if (type === 'gate_retry') { html += ev(t(e), 'skip', `⚠ гейт ${esc(e.step)}: ${esc(e.reason || 'переспрос')}`, 'попытка ' + (e.attempt || '?')); continue; }
    if (type === 'gate_decision') { html += ev(t(e), e.action === 'accept' ? 'ok' : 'skip', `👤 гейт ${esc(e.step)}: ${esc(e.action)}${e.auto ? ' (авто --yes)' : ''}`, e.materialized ? esc(JSON.stringify(e.materialized)) : ''); continue; }
    if (type === 'run_start') { html += ev(t(e), 'dim', `ран: ${esc(e.pipeline || '?')}${e.foreach ? ' · foreach ' + esc(e.foreach) : ''}`, ''); continue; }
    if (type === 'run_resumed') { html += ev(t(e), 'par', `ран возобновлён (resume)`, ''); continue; }
    if (type === 'run_end') { html += ev(t(e), (e.aborted || 0) ? 'err' : 'ok', `■ ран завершён: ok=${e.ok} aborted=${e.aborted || 0}`, ''); continue; }
    if (type === 'run_failed') { html += ev(t(e), 'err', `■ ран упал: ${esc(e.error || '')}`, ''); continue; }
    if (type === 'post_phase_start') { html += ev(t(e), 'dim', 'post-фаза (после foreach)…', ''); continue; }
    if (type === 'post_phase_end') { html += ev(t(e), 'dim', 'post-фаза завершена', ''); continue; }
    if (type === 'foreach_item_failed') { html += ev(t(e), 'err', `⤷ ${esc(e.step)} · элемент ${e.item_index}: ${esc(e.error || 'ошибка')}`, ''); continue; }
    if (type === 'file_ref_warning' || type === 'contract_warning') { html += ev(t(e), 'dim', '· ' + esc(e.message || e.warning || JSON.stringify(e)), ''); continue; }
  }
  if (inItem >= 0) html += '</div>';
  if (inPar) html += '</div>';
  return html || '<div class="empty">пусто</div>';
}

function ev(time, cls, msg, extra) {
  return `<div class="ev ${cls}"><span class="t">${esc(time)}</span><span class="m">${msg}</span>${extra ? `<span class="x">${extra}</span>` : ''}</div>`;
}

// ── контекст ─────────────────────────────────────────────────────────────
function renderCtx(ctx) {
  if (!ctx || !Object.keys(ctx).length) return '<div class="empty">контекст пуст</div>';
  return `<div style="font-size:11px;color:var(--dim);margin-bottom:8px">снапшот context.json — input.* и steps.*</div>` +
    Object.entries(ctx).map(([k, v]) => `<details ${k === 'steps' ? 'open' : ''}><summary><span class="k">${esc(k)}</span></summary>${jsonTree(v, 1)}</details>`).join('');
}

function jsonTree(v, depth) {
  const pad = '· '.repeat(depth);
  if (v === null) return '<span class="m">null</span>';
  if (typeof v === 'string') return `<span class="s">"${esc(v.length > 200 ? v.slice(0, 200) + '…' : v)}"</span>`;
  if (typeof v === 'number') return `<span class="n">${v}</span>`;
  if (typeof v === 'boolean') return `<span class="b">${v}</span>`;
  if (Array.isArray(v)) {
    if (!v.length) return '<span class="m">[]</span>';
    if (v.every(x => typeof x !== 'object' || x === null)) return `<span class="m">[${v.map(x => typeof x === 'string' ? esc(x) : JSON.stringify(x)).join(', ')}]</span>`;
    return '<div>' + v.map((x, i) => `<div>${pad}[${i}] ${jsonTree(x, depth)}</div>`).join('') + '</div>';
  }
  const keys = Object.keys(v);
  if (!keys.length) return '<span class="m">{}</span>';
  return '<div>' + keys.map(k =>
    `<details ${depth < 2 ? 'open' : ''} style="margin-left:${depth * 10}px"><summary><span class="k">${esc(k)}</span>${renderInline(v[k], depth)}</summary>${typeof v[k] === 'object' && v[k] !== null ? jsonTree(v[k], depth + 1) : ''}</details>`
  ).join('') + '</div>';
}

function renderInline(v, depth) {
  if (v === null) return ' = <span class="m">null</span>';
  if (typeof v === 'string') return ` = <span class="s">"${esc(v.length > 60 ? v.slice(0, 60) + '…' : v)}"</span>`;
  if (typeof v === 'number' || typeof v === 'boolean') return ` = <span class="${typeof v === 'number' ? 'n' : 'b'}">${v}</span>`;
  if (Array.isArray(v)) return ` = <span class="m">[${v.length}]</span>`;
  return ` = <span class="m">{${Object.keys(v).length}}</span>`;
}

// ── live-журнал ──────────────────────────────────────────────────────────
window.showRTab = which => {
  $('#rt-ctx').classList.toggle('active', which === 'ctx');
  $('#rt-jnl').classList.toggle('active', which === 'jnl');
  $('#rt-ctx-body').style.display = which === 'ctx' ? '' : 'none';
  $('#rt-jnl-body').style.display = which === 'jnl' ? '' : 'none';
};
window.toggleAuto = () => {
  state.autoScroll = !state.autoScroll;
  $('#autochip').classList.toggle('on', state.autoScroll);
};

function renderJournal(newEvents, append) {
  const body = $('#jnl-lines');
  if (!body) return;
  const src = append ? newEvents : state.journal.events;
  const lines = src.map(e => {
    const ts = (e.ts || '').split('T')[1] || '';
    const {ts: _t, ...rest} = e;
    return `<div class="ln"><span class="ts">${esc(ts)}</span> ${esc(JSON.stringify(rest))}</div>`;
  }).join('');
  if (append && body.firstChild) body.insertAdjacentHTML('beforeend', lines);
  else body.innerHTML = lines;
  if (state.autoScroll) $('#rt-jnl-body').scrollTop = $('#rt-jnl-body').scrollHeight;
}

// ── пайплайны + DAG ──────────────────────────────────────────────────────
async function openPipeline(file) {
  state.currentPipe = file;
  renderPipList();
  $('#pdetail').innerHTML = '<div class="empty">загрузка…</div>';
  let yaml, plan;
  try {
    yaml = await api('/api/pipelines/' + file);
    const r = await fetch('/api/plan/pipeline', { method: 'POST', body: yaml, headers: {'Content-Type': 'text/yaml'} });
    plan = await r.json();
  } catch (e) {
    $('#pdetail').innerHTML = '<div class="empty">' + esc(e.message) + '</div>';
    return;
  }
  const p = state.pipelines.find(x => x.file === file) || {};
  $('#pdetail').innerHTML = `
    <div class="dhead" style="margin-bottom:12px">
      <h2>${esc(p.name || file)}</h2>
      <span class="sub">${p.steps} ш.${p.foreach ? ' · foreach ' + esc(p.foreach) : ''}</span>
      ${plan.errors && plan.errors.length ? `<span class="badge err">errors: ${plan.errors.length}</span>` : '<span class="badge ok">валиден</span>'}
    </div>
    ${plan.errors && plan.errors.length ? `<div style="color:var(--err);font-size:12px;margin-bottom:10px">${plan.errors.map(esc).join('<br>')}</div>` : ''}
    ${plan.warnings && plan.warnings.length ? `<div style="color:var(--skip);font-size:12px;margin-bottom:10px">${plan.warnings.map(esc).join('<br>')}</div>` : ''}
    <div class="card" style="margin-bottom:16px">
      <h3>DAG</h3>
      <svg id="dag"></svg>
    </div>
    <div class="card"><h3>YAML</h3><pre class="yaml">${esc(yaml)}</pre></div>`;
  drawDag(plan.dag);
}

function drawDag(dag) {
  const svg = $('#dag');
  if (!dag || !dag.nodes.length) { svg.outerHTML = '<div class="empty">DAG пуст</div>'; return; }
  const W = 180, H = 64, GX = 60, GY = 34;
  // топологические слои
  const ids = dag.nodes.map(n => n.id);
  const indeg = Object.fromEntries(ids.map(i => [i, 0]));
  const adj = {};
  for (const e of dag.edges || []) {
    if (!adj[e.from]) adj[e.from] = [];
    adj[e.from].push(e.to);
    if (indeg[e.to] != null) indeg[e.to]++;
    if (indeg[e.from] == null) indeg[e.from] = 0;
  }
  const layer = {};
  let changed = true, guard = 0;
  for (const i of ids) layer[i] = 0;
  while (changed && guard++ < 50) {
    changed = false;
    for (const e of dag.edges || []) {
      if (layer[e.from] != null && layer[e.to] != null && layer[e.to] < layer[e.from] + 1) {
        layer[e.to] = layer[e.from] + 1;
        changed = true;
      }
    }
  }
  const byLayer = {};
  for (const n of dag.nodes) (byLayer[layer[n.id]] = byLayer[layer[n.id]] || []).push(n);
  const layers = Object.keys(byLayer).map(Number).sort((a, b) => a - b);
  const maxCol = Math.max(...layers.map(l => byLayer[l].length));
  const padTop = 30;
  const pos = {};
  layers.forEach((l, li) => {
    const col = byLayer[l];
    col.forEach((n, ci) => {
      pos[n.id] = { x: 40 + li * (W + GX), y: padTop + ci * (H + GY) + ((maxCol - col.length) * (H + GY)) / 2 };
    });
  });
  const width = 40 + layers.length * (W + GX) + 40;
  const height = padTop * 2 + maxCol * (H + GY) + 60;
  let s = `<svg id="dag" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">` +
    `<defs><marker id="arr" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="#484f58"/></marker></defs>`;
  // параллельные группы: рамка
  const groups = {};
  for (const n of dag.nodes) if (n.parallel_group) (groups[n.parallel_group] = groups[n.parallel_group] || []).push(n);
  for (const [g, members] of Object.entries(groups)) {
    if (members.length < 2) continue;
    const xs = members.map(m => pos[m.id].x), ys = members.map(m => pos[m.id].y);
    const x = Math.min(...xs) - 10, y = Math.min(...ys) - 20;
    const w = Math.max(...xs) + W - x + 10, h = Math.max(...ys) + H - y + 30;
    s += `<rect class="daggrp" x="${x}" y="${y}" width="${w}" height="${h}" rx="10"/><text class="dagtxt par" x="${x + 8}" y="${y - 6}">‖ ${esc(g)}</text>`;
  }
  for (const e of dag.edges || []) {
    const a = pos[e.from], b = pos[e.to];
    if (!a || !b) continue;
    const x1 = a.x + W, y1 = a.y + H / 2, x2 = b.x, y2 = b.y + H / 2;
    const mx = (x1 + x2) / 2;
    s += `<path class="dagedge" d="M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}" marker-end="url(#arr)"/>`;
  }
  for (const n of dag.nodes) {
    const p = pos[n.id];
    const phase = ['pre', 'foreach', 'post'].includes(n.phase) ? n.phase : (n.parallel_group ? 'foreach' : 'foreach');
    s += `<rect class="dagnode ${phase}" x="${p.x}" y="${p.y}" width="${W}" height="${H}" rx="8"/>`;
    s += `<text class="dagtxt" x="${p.x + 10}" y="${p.y + 20}">${esc(n.id)}</text>`;
    const plug = String(n.plugin || '').split('/').pop();
    s += `<text class="dagtxt dim" x="${p.x + 10}" y="${p.y + 36}">${esc(plug)}</text>`;
    let ly = p.y + 51;
    if (n.when) s += `<text class="dagtxt acc" x="${p.x + 10}" y="${ly}">when: ${esc(n.when)}</text>`;
    if (n.foreach) s += `<text class="dagtxt acc" x="${p.x + 10 + (n.when ? 100 : 0)}" y="${ly}">foreach: ${esc(n.foreach)}</text>`;
    if (n.phase === 'pre' || n.phase === 'post') s += `<text class="dagtxt dim" x="${p.x + W - 44}" y="${p.y + 20}">${n.phase}</text>`;
  }
  s += '</svg>';
  svg.outerHTML = s;
}

// ── v0.24: гейт-карточка (решение из браузера) ────────────────────────────
function pendingGate(events) {
  let gw = null;
  for (const e of events) {
    if (e.type === 'gate_wait') gw = e;
    else if (e.type === 'gate_decision') gw = null;
  }
  return gw;
}

function updateGateCard(id) {
  const card = $('#gate-card');
  if (!card || state.currentRun !== id) return;
  const gw = pendingGate(state.journal.events);
  const key = gw ? gw.ts + ':' + gw.step : '';
  if (!gw) {
    if (card.dataset.submitted !== key && key === '') {
      // гейт решён (или его не было): если карточка была нашей — убрать
      if (card.dataset.gateFor && card.dataset.gateFor !== 'done') card.innerHTML = '';
      card.style.display = 'none';
      card.dataset.gateFor = '';
    }
    return;
  }
  if (card.dataset.gateFor === key) return; // уже отрисована (или отправлена) — не затираем ввод
  card.dataset.gateFor = key;
  card.style.display = '';
  const fields = (gw.form || []).map(f => {
    const val = String(f.value == null ? '' : f.value);
    if (f.editable) {
      return `<div class="gfield"><label>* ${esc(f.field)}${f.type ? ' · ' + esc(f.type) : ''} (JSON, пусто — оставить)</label>
        <input data-field="${esc(f.field)}" value="${esc(val)}" spellcheck="false"/></div>`;
    }
    return `<div class="gfield"><label>${esc(f.field)}</label>
      <input readonly value="${esc(val)}"/></div>`;
  }).join('');
  const btns = (gw.actions || ['accept', 'reject']).map(a => {
    const cls = a === 'accept' ? 'ok' : (a === 'reject' ? 'err' : '');
    const label = a === 'accept' ? '✓ принять' : a === 'reject' ? '✗ отклонить' : esc(a);
    return `<button class="${cls}" onclick="submitGate('${id}','${esc(a)}')">${label}</button>`;
  }).join('');
  card.innerHTML = `
    <h3>👤 human_gate · ${esc(gw.step)} — ран ждёт твоего решения</h3>
    ${fields || '<div class="gstatus">форма пуста</div>'}
    <div class="gactions">${btns}</div>
    <div class="gstatus" id="gate-status"></div>`;
}

window.submitGate = async (id, action) => {
  const card = $('#gate-card');
  const st = $('#gate-status');
  const edits = {};
  let bad = null;
  card.querySelectorAll('input[data-field]').forEach(inp => {
    const v = inp.value.trim();
    if (v === '') return; // пусто = оставить
    let parsed;
    try { parsed = JSON.parse(v); }
    catch { bad = inp.dataset.field; return; }
    edits[inp.dataset.field] = parsed;
  });
  if (bad) {
    st.className = 'gstatus error';
    st.textContent = 'не JSON: ' + bad + ' — поправь и нажми действие ещё раз';
    return;
  }
  try {
    await api('/api/runs/' + id + '/gate', {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({ action, edits }),
    });
    card.dataset.gateFor = 'done'; // больше не перерисовывать
    card.querySelectorAll('button').forEach(b => b.disabled = true);
    card.querySelectorAll('input').forEach(i => i.readOnly = true);
    st.className = 'gstatus';
    st.textContent = 'решение «' + action + '» отправлено — ран продолжится…';
  } catch (e) {
    st.className = 'gstatus error';
    st.textContent = e.message;
  }
};

init();
