// v0.25 — редактор пайплайнов: палитра → холст (сетка 20px), bind-связи,
// undo/redo, валидация и сериализация через ядро (Go), сохранение PUT.
// v0.27: when — условие шага (path/op/value, 10 операторов ядра).
// v0.28: foreach/parallel_group/after_foreach на шаге + foreach-батч
// пайплайна (foreach/foreach_item/item_type/item_format).
// v0.29: retry (on_error: retry + retry{attempts, delay, backoff}).
// v0.5: secrets (pipeline.secrets — env-ключи плагинов) в UI (чипы в
// блоке «Пайплайн» + подсказка в шаге, какой ключ просит плагин).
// v0.6: network (pipeline.network — политика allow/deny) в UI: чекбокс
// «deny» + подсказка в шаге, какую сеть просит плагин (и не запрещено ли).
// v0.8a: перетаскивание палитра→холст на mouse-событиях (ghost) + клик =
// добавить на свободное место. Нативный HTML5 DnD не работает в sandboxed
// iframe (превью) — mouse-события работают везде.
// Честный скоуп: type-объявления input не управляются — такие пайплайны
// открываются с баннером и без сохранения.
const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>\"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','\"':'&quot;'}[c]));
const GRID = 20;
const snap = v => Math.round(v / GRID) * GRID;

let state = {
  plugins: [],
  doc: { name: 'new_pipeline', file: 'new_pipeline.yaml', format_version: '', input: [], steps: [],
    secrets: [], network: '',
    foreach: '', foreach_item: '', item_type: '', item_format: '' },
  unsupported: [],
  sel: null,
  undo: [],
  redo: [],
  yaml: '',
  valErrs: [],
  valWarns: [],
  validating: false,
};

async function api(path, opts = {}) {
  const r = await fetch(path, opts);
  const ct = r.headers.get('content-type') || '';
  const body = ct.includes('json') ? await r.json() : await r.text();
  if (!r.ok) throw new Error(typeof body === 'string' ? body : JSON.stringify(body));
  return body;
}
async function apiRaw(path, opts = {}) {
  const r = await fetch(path, opts);
  const text = await r.text();
  if (!r.ok) throw new Error(text);
  return text;
}

// ── manifest-хелперы ─────────────────────────────────────────────────────
function pluginInfo(id) {
  if (id === 'core/human_gate') return { id, input: {}, output: {}, description: 'человек в петле' };
  return state.plugins.find(p => p.id === id || p.dir.endsWith('/' + id)) || null;
}
function inFields(p) {
  const info = pluginInfo(p);
  return info ? Object.keys(info.input || {}) : [];
}
function outFields(p) {
  const info = pluginInfo(p);
  return info ? Object.keys(info.output || {}) : [];
}
// editKey — правило материализации гейта (как в ядре): basename, при
// коллизии — steps.X.field → «X_field»
function gateOutKeys(gateStep) {
  const bnCount = {};
  for (const f of gateStep.form || []) {
    const bn = f.field.split('.').pop();
    bnCount[bn] = (bnCount[bn] || 0) + 1;
  }
  return (gateStep.form || []).map(f => {
    const bn = f.field.split('.').pop();
    if (bnCount[bn] > 1) {
      const parts = f.field.split('.');
      if (parts.length >= 3 && parts[0] === 'steps') return parts[1] + '_' + bn;
      return f.field.replace(/\./g, '_');
    }
    return bn;
  });
}
// источники для bind: input.* + steps.<id>.<out> (у гейта — из его формы)
function sourceOptions(excludeId) {
  const opts = [];
  for (const i of state.doc.input) opts.push('input.' + i.name);
  for (const st of state.doc.steps) {
    if (st.id === excludeId) continue;
    const outs = st.plugin === 'core/human_gate' ? gateOutKeys(st) : outFields(st.plugin);
    for (const o of outs) opts.push('steps.' + st.id + '.' + o);
  }
  return opts;
}

// ── undo/redo ─────────────────────────────────────────────────────────────
function pushUndo() {
  state.undo.push(JSON.stringify(state.doc));
  if (state.undo.length > 100) state.undo.shift();
  state.redo = [];
}
function undo() {
  if (!state.undo.length) return;
  state.redo.push(JSON.stringify(state.doc));
  state.doc = JSON.parse(state.undo.pop());
  state.sel = null;
  renderAll();
}
function redo() {
  if (!state.redo.length) return;
  state.undo.push(JSON.stringify(state.doc));
  state.doc = JSON.parse(state.redo.pop());
  state.sel = null;
  renderAll();
}

// ── модель → DOM ──────────────────────────────────────────────────────────
function nextStepId() {
  let n = state.doc.steps.length + 1;
  let id = 's' + n;
  while (state.doc.steps.some(s => s.id === id)) id = 's' + (++n);
  return id;
}

function addStep(plugin, x, y) {
  pushUndo();
  const st = {
    id: nextStepId(), plugin, pos: [snap(x), snap(y)],
    on_error: 'stop', timeout: '', bind: {}, when: null,
    foreach: '', foreach_item: '', after_foreach: false, parallel_group: '', retry: null,
  };
  if (plugin === 'core/human_gate') {
    st.form = []; st.actions = ['accept', 'reject']; st.on_reject = 'stop';
  }
  state.doc.steps.push(st);
  state.sel = st.id;
  renderAll();
}

function renderPalette() {
  const el = $('#plugin-list');
  el.innerHTML = state.plugins.map(p => {
    const ins = Object.keys(p.input || {}).length, outs = Object.keys(p.output || {}).length;
    return `<div class="plug" data-plugin="${esc(p.id)}">
      <b>${esc(p.id)}</b>
      <small>${esc(p.description || '')}</small>
      <div class="io">in: ${ins} · out: ${outs}</div>
    </div>`;
  }).join('') || '<div class="empty">плагинов нет</div>';
  // v0.8a: mouse-based drag (нативный DnD мёртв в sandboxed iframe)
  el.querySelectorAll('.plug').forEach(el2 => {
    el2.addEventListener('mousedown', e => startPaletteDrag(e, el2.dataset.plugin));
  });
}

// v0.8a: перетаскивание плагина на холст через mouse-события:
// мousedown → ghost следует за курсором → mouseup над холстом = добавить;
// клик без движения = добавить на свободное место. Работает в sandboxed
// iframe (превью), WebView2 и обычном браузере.
function startPaletteDrag(e, pluginId) {
  if (e.button !== 0) return;
  e.preventDefault(); // запрет выделения текста при волочении
  const startX = e.clientX, startY = e.clientY;
  const wrap = $('#canvas-wrap'), canvas = $('#canvas');
  let moved = false;
  const ghost = document.createElement('div');
  ghost.className = 'plug-ghost';
  ghost.textContent = pluginId;
  document.body.appendChild(ghost);
  const placeGhost = ev => { ghost.style.left = (ev.clientX + 12) + 'px'; ghost.style.top = (ev.clientY + 10) + 'px'; };
  placeGhost(e);
  const overCanvas = ev => {
    const r = wrap.getBoundingClientRect();
    return ev.clientX >= r.left && ev.clientX <= r.right && ev.clientY >= r.top && ev.clientY <= r.bottom;
  };
  const mm = ev => {
    if (Math.abs(ev.clientX - startX) + Math.abs(ev.clientY - startY) > 4) moved = true;
    placeGhost(ev);
    wrap.classList.toggle('drop-over', moved && overCanvas(ev));
  };
  const mu = ev => {
    document.removeEventListener('mousemove', mm);
    document.removeEventListener('mouseup', mu);
    ghost.remove();
    wrap.classList.remove('drop-over');
    if (overCanvas(ev)) {
      const cRect = canvas.getBoundingClientRect();
      addStep(pluginId, ev.clientX - cRect.left - 115, ev.clientY - cRect.top - 20);
    } else if (!moved) {
      const i = state.doc.steps.length;
      addStep(pluginId, 40 + (i % 3) * 270, 40 + Math.floor(i / 3) * 140);
    }
  };
  document.addEventListener('mousemove', mm);
  document.addEventListener('mouseup', mu);
}

function renderNodes() {
  const canvas = $('#canvas');
  canvas.querySelectorAll('.node').forEach(n => n.remove());
  for (const st of state.doc.steps) {
    const gate = st.plugin === 'core/human_gate';
    const info = pluginInfo(st.plugin);
    const node = document.createElement('div');
    node.className = 'node' + (gate ? ' gate' : '') + (state.sel === st.id ? ' selected' : '');
    node.style.left = st.pos[0] + 'px';
    node.style.top = st.pos[1] + 'px';
    node.dataset.id = st.id;
    const ins = gate
      ? '<div class="inrow"><span>form</span><span class="src set">см. справа</span></div>'
      : (inFields(st.plugin).map(f => {
          const src = st.bind[f] || '';
          return `<div class="inrow" data-field="${esc(f)}"><span>${esc(f)}</span><span class="src ${src ? 'set' : ''}">${esc(src || '—')}</span></div>`;
        }).join('') || '<div class="inrow"><span>входов нет</span></div>');
    const outs = gate
      ? '<span class="outchip" title="выходы = поля формы">поля формы</span>'
      : (outFields(st.plugin).map(o => `<span class="outchip" data-out="${esc(o)}">${esc(o)}</span>`).join('') || '<span class="outchip">нет</span>');
    node.innerHTML = `
      <div class="nh"><span class="nid">${esc(st.id)}</span><span class="nplug">${gate ? 'human_gate' : esc((info && (info.id === st.plugin ? st.plugin.split('/').pop() : st.plugin)) || st.plugin)}</span></div>
      <div class="body">${ins}<div style="margin-top:6px">${outs}</div></div>
      <div class="foot"><span>on_err: ${esc(st.on_error || 'stop')}</span>${st.when ? `<span title="when: ${esc(st.when.path || '')}">⚖ when:${esc(st.when.op || '')}</span>` : ''}${st.foreach ? '<span title="foreach: ' + esc(st.foreach) + '">⤨ foreach</span>' : ''}${st.parallel_group ? `<span title="parallel_group">∥ ${esc(st.parallel_group)}</span>` : ''}${st.after_foreach ? '<span title="after_foreach">⤓ post</span>' : ''}${st.timeout ? `<span>${esc(st.timeout)}</span>` : ''}</div>`;
    canvas.appendChild(node);
    node.addEventListener('mousedown', e => startNodeDrag(e, st));
    node.addEventListener('click', e => { e.stopPropagation(); state.sel = st.id; renderAll(); });
  }
  renderEdges();
}

function renderEdges() {
  const svg = $('#edges');
  const cRect = $('#canvas').getBoundingClientRect();
  const path = (x1, y1, x2, y2) => {
    const mx = (x1 + x2) / 2;
    return `M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2},${y2}`;
  };
  let s = '';
  for (const st of state.doc.steps) {
    for (const [field, src] of Object.entries(st.bind || {})) {
      if (!src) continue;
      const m = src.match(/^steps\.([^.]+)\.([\w\-.]+)$/);
      const dstEl = document.querySelector(`.node[data-id="${st.id}"] .inrow[data-field="${field}"]`);
      if (!dstEl) continue;
      const dRect = dstEl.getBoundingClientRect();
      const x2 = dRect.left - cRect.left - 4, y2 = dRect.top - cRect.top + dRect.height / 2;
      let x1 = 0, y1 = 0, ok = true;
      if (m) {
        const chip = document.querySelector(`.node[data-id="${m[1]}"] .outchip[data-out="${m[2]}"]`);
        if (!chip) { ok = false; }
        else {
          const sRect = chip.getBoundingClientRect();
          x1 = sRect.right - cRect.left + 4; y1 = sRect.top - cRect.top + sRect.height / 2;
        }
      } else {
        ok = false; // input.* — рисуем с левого края холста
        x1 = 8; y1 = y2 - 14;
      }
      s += `<path class="edge${ok ? '' : ' hi'}" d="${path(x1, y1, x2, y2)}"/>`;
    }
  }
  svg.innerHTML = s;
}

// ── панель свойств ────────────────────────────────────────────────────────
const WHEN_OPS = [
  ['', '— без условия —'], ['truthy', 'truthy — значение истинно'],
  ['exists', 'exists — путь существует'], ['missing', 'missing — пути нет'],
  ['eq', 'eq — равно'], ['neq', 'neq — не равно'],
  ['gt', 'gt — больше (число)'], ['gte', 'gte — больше или равно (число)'],
  ['lt', 'lt — меньше (число)'], ['lte', 'lte — меньше или равно (число)'],
  ['contains', 'contains — содержит (строка/массив)'],
];
const NO_VALUE_OPS = ['truthy', 'exists', 'missing'];
function whenBlock(st) {
  const w = st.when;
  const opOpts = WHEN_OPS.map(([v, t]) => `<option value="${v}"${(w ? w.op : '') === v ? ' selected' : ''}>${t}</option>`).join('');
  if (!w) return `<div class="pblock"><label>when — условие (шаг идёт, только если истинно; иначе skipped)</label>
    <select data-whenop>${opOpts}</select></div>`;
  const srcs = sourceOptions(st.id);
  let pathOpts = '<option value="">— путь —</option>' +
    srcs.map(o => `<option value="${esc(o)}"${w.path === o ? ' selected' : ''}>${esc(o)}</option>`).join('');
  if (w.path && !srcs.includes(w.path)) {
    pathOpts += `<option value="${esc(w.path)}" selected>${esc(w.path)} (вручную)</option>`;
  }
  const needVal = !NO_VALUE_OPS.includes(w.op);
  return `
  <div class="pblock"><label>when — условие (шаг идёт, только если истинно; иначе skipped)</label>
    <select data-whenop>${opOpts}</select></div>
  <div class="prow"><div class="pblock" style="flex:2"><label>when.path (input.* / steps.X.out)</label>
    <select data-whenpath>${pathOpts}</select></div>
  ${needVal ? `<div class="pblock" style="flex:1"><label>when.value${w.op.startsWith('g') || w.op.startsWith('l') ? ' (число)' : ''}</label>
    <input data-whenv value="${esc(w.value ?? '')}" placeholder="10 / text" spellcheck="false"/></div>` : ''}</div>`;
}

// v0.28: управляющий поток шага (foreach/parallel_group/after_foreach).
// human_gate: foreach и parallel_group запрещены ядром — вместо полей подсказка.
function flowBlock(st) {
  const gate = st.plugin === 'core/human_gate';
  if (gate) {
    return '<div class="hint">foreach / parallel_group: не применяются к human_gate (гейт сериализует терминал) — ядро это ошибками валидации.</div>';
  }
  const srcs = sourceOptions(st.id);
  let fOpts = '<option value="">— без foreach —</option>' +
    srcs.map(o => `<option value="${esc(o)}"${st.foreach === o ? ' selected' : ''}>${esc(o)}</option>`).join('');
  if (st.foreach && !srcs.includes(st.foreach)) {
    fOpts += `<option value="${esc(st.foreach)}" selected>${esc(st.foreach)} (вручную)</option>`;
  }
  return `
  <div class="pblock"><label>foreach — массив: шаг по каждому элементу (input.&lt;foreach_item&gt; перезаписывается)</label>
    <select data-sforeach>${fOpts}</select></div>
  <div class="prow"><div class="pblock" style="flex:1"><label>foreach_item (по умолчанию item)</label>
    <input data-sfitem value="${esc(st.foreach_item || '')}" placeholder="item" spellcheck="false"/></div>
  <div class="pblock" style="flex:1"><label>parallel_group — смежные шаги с одним именем — параллельно</label>
    <input data-spgrp value="${esc(st.parallel_group || '')}" placeholder="analyze" spellcheck="false"/></div></div>
  <div class="pblock"><label><input type="checkbox" data-safter ${st.after_foreach ? 'checked' : ''}/> after_foreach — один раз после всего pipeline-foreach (агрегаты steps.X_all)</label></div>`;
}

function pipelineFlowBlock() {
  const d = state.doc;
  const srcs = sourceOptions(null);
  let fOpts = '<option value="">— без батча —</option>' +
    srcs.map(o => `<option value="${esc(o)}"${d.foreach === o ? ' selected' : ''}>${esc(o)}</option>`).join('');
  if (d.foreach && !srcs.includes(d.foreach)) {
    fOpts += `<option value="${esc(d.foreach)}" selected>${esc(d.foreach)} (вручную)</option>`;
  }
  return `
  <div class="pblock"><label>foreach — весь пайплайн по каждому элементу массива</label>
    <select data-pforeach>${fOpts}</select></div>
  <div class="prow"><div class="pblock" style="flex:1"><label>foreach_item (item-переменная, по умолчанию item)</label>
    <input data-pfitem value="${esc(d.foreach_item || '')}" placeholder="row" spellcheck="false"/></div>
  <div class="pblock" style="flex:1"><label>item_type (object / string / number)</label>
    <input data-ptype value="${esc(d.item_type || '')}" placeholder="object" spellcheck="false"/></div>
  <div class="pblock" style="flex:1"><label>item_format</label>
    <input data-pformat value="${esc(d.item_format || '')}" placeholder="(пусто)" spellcheck="false"/></div></div>
  <div class="hint">Шаги с after_foreach выполняются один раз после всех элементов
  (агрегаты steps.X_all). См. examples/csv_foreach_summary.yaml.</div>`;
}

// v0.29: retry — повтор таймаутов и retryable-ошибок; исчерпан = stop.
function retryBlock(st) {
  const r = st.retry;
  const on = r || st.on_error === 'retry';
  return `
  <div class="pblock"><label><input type="checkbox" data-sretry ${on ? 'checked' : ''}/> retry — повторять таймауты и retryable-ошибки (исчерпан = stop)</label></div>
  ${on ? `<div class="prow"><div class="pblock" style="flex:1"><label>attempts (≥1)</label>
    <input data-srattempts type="number" min="1" value="${esc(r ? r.attempts : 3)}" spellcheck="false"/></div>
  <div class="pblock" style="flex:1"><label>delay (2s, 500ms, 1m…)</label>
    <input data-srdelay value="${esc(r ? (r.delay || '') : '')}" placeholder="2s" spellcheck="false"/></div>
  <div class="pblock" style="flex:1"><label>backoff</label>
    <select data-srbackoff><option value="fixed"${(r && r.backoff === 'fixed') || (!r) ? ' selected' : ''}>fixed</option><option value="exponential"${r && r.backoff === 'exponential' ? ' selected' : ''}>exponential</option></select></div></div>` : ''}`;
}

// v0.6: подсказка в шаге — какую сеть просит плагин (манифест) и не
// запрещена ли она политикой пайплайна.
function pluginNetworkHint(st) {
  const info = pluginInfo(st.plugin);
  const net = (info && info.permissions && info.permissions.network) || [];
  if (!net.length) return '';
  const hosts = net.map(n => (n.any_host ? '*' : (n.host + ':' + n.port))).join(', ');
  if (state.doc.network === 'deny')
    return `<div class="hint">сеть плагина: <span style="color:var(--err,#e5534b)">${esc(hosts)} — пайплайн запрещает сеть (network: deny, ошибка валидации)</span></div>`;
  return `<div class="hint">сеть плагина: ${esc(hosts)} (declare-now, аудит — журнал)</div>`;
}

// v0.5: подсказка в шаге — какие env-ключи просит плагин (манифест) и
// объявлены ли они в pipeline secrets.
function pluginSecretsHint(st) {
  const info = pluginInfo(st.plugin);
  const needs = (info && info.permissions && info.permissions.secrets) || [];
  if (!needs.length) return '';
  const have = new Set(state.doc.secrets || []);
  const missing = needs.filter(k => !have.has(k));
  const txt = missing.length
    ? `<span style="color:var(--err,#e5534b)">нужны ключи ${needs.map(esc).join(', ')} — не все объявлены в secrets пайплайна (блок «Пайплайн»)</span>`
    : `просит ключи ${needs.map(esc).join(', ')} — объявлены в secrets пайплайна ✓`;
  return `<div class="hint">secrets плагина: ${txt}</div>`;
}

// v0.5: secrets — чипы env-ключей + добавление. Ключ читается ядром из env
// и передаётся плагину; кросс-чек с permissions.secrets — валидатор (warnings).
function secretsBlock() {
  const keys = state.doc.secrets || [];
  const chips = keys.map((k, i) =>
    `<span class="chip">${esc(k)}<button data-sdel="${i}" title="убрать">×</button></span>`).join(' ');
  return `<div class="chips">${chips || '<span class="hint">нет ключей</span>'}
    <input data-skey placeholder="ENV_KEY" spellcheck="false" style="width:160px"/>
    <button data-sadd>+ ключ</button></div>
    <div class="hint">Ключи читаются из окружения при запуске (secrets: [KEY] в YAML).</div>`;
}

function renderProps() {
  const el = $('#props');
  let html = '';
  if (state.unsupported.length) {
    html += `<div id="banner" style="display:block">Пайплайн содержит поля, которые редактор v0.6 не управляет:
      <b>${state.unsupported.map(esc).join(', ')}</b>. Сохранение из редактора запрещено —
      правь в YAML (вкладка «Пайплайны» в консоли), иначе эти поля будут потеряны.</div>`;
  }
  const st = state.doc.steps.find(s => s.id === state.sel);
  if (st) {
    const gate = st.plugin === 'core/human_gate';
    const ins = gate ? '' : inFields(st.plugin).map(f => {
      const opts = ['<option value="">— не привязано —</option>']
        .concat(sourceOptions(st.id).map(o => `<option value="${esc(o)}"${st.bind[f] === o ? ' selected' : ''}>${esc(o)}</option>`)).join('');
      // текущее значение, которого нет в списке (например, ручной путь)
      if (st.bind[f] && !sourceOptions(st.id).includes(st.bind[f]) && !('input.' + st.bind[f])) {
        opts += `<option value="${esc(st.bind[f])}" selected>${esc(st.bind[f])} (вручную)</option>`;
      }
      return `<div class="pblock"><label>bind: ${esc(f)}</label><select data-bind="${esc(f)}">${opts}</select></div>`;
    }).join('');
    const gateBlock = gate ? `
      <div class="pblock"><label>form — построчно: «путь» или «e:путь» (editable)</label>
        <textarea data-gform rows="4" spellcheck="false">${esc((st.form || []).map(f => (f.editable ? 'e:' : '') + f.field).join('\n'))}</textarea></div>
      <div class="pblock"><label>actions — по одному на строку</label>
        <textarea data-gactions rows="3" spellcheck="false">${esc((st.actions || []).join('\n'))}</textarea></div>
      <div class="pblock"><label>on_reject</label>
        <select data-gonreject>
          <option value="stop"${(st.on_reject || 'stop') === 'stop' ? ' selected' : ''}>stop (ран остановлен)</option>
          <option value=""${!st.on_reject ? ' selected' : ''}>continue (идём дальше)</option>
        </select></div>` : '';
    html += `
      <h3>Шаг</h3>
      <div class="prow">
        <div class="pblock"><label>id</label><input data-sid value="${esc(st.id)}" spellcheck="false"/></div>
        <div class="pblock"><label>on_error</label>
          <select data-serr><option value="stop"${st.on_error === 'stop' ? ' selected' : ''}>stop</option><option value="skip"${st.on_error === 'skip' ? ' selected' : ''}>skip</option><option value="retry"${st.on_error === 'retry' ? ' selected' : ''}>retry (блок ниже)</option></select></div>
      </div>
      <div class="pblock"><label>плагин</label><input value="${esc(st.plugin)}" readonly style="color:var(--dim)"/></div>
      ${pluginSecretsHint(st)}
      ${pluginNetworkHint(st)}
      <div class="pblock"><label>timeout (пусто = 60s): 10s, 1m30s, …</label><input data-stimeout value="${esc(st.timeout || '')}" placeholder="60s" spellcheck="false"/></div>
      ${whenBlock(st)}
      ${flowBlock(st)}
      ${retryBlock(st)}
      ${ins}
      ${gateBlock}
      <button class="danger" data-del>удалить шаг</button>
      <div class="hint">Двигай шаг за заголовок. Связи — в «bind» (список источников) — рёбра перерисуются сами.</div>`;
  } else {
    const rows = state.doc.input.map((i, idx) => `
      <div class="irow"><input data-iname="${idx}" value="${esc(i.name)}" placeholder="имя"/><input data-indef="${idx}" value="${esc(i.default || '')}" placeholder="значение по умолчанию"/><button data-indel="${idx}">×</button></div>`).join('');
    html += `
      <h3>Пайплайн (выбери шаг для правки шага)</h3>
      <div class="pblock"><label>имя</label><input data-pname value="${esc(state.doc.name)}" spellcheck="false"/></div>
      <div class="pblock"><label>вход (input.*)</label>${rows || '<div class="hint">нет входов</div>'}
        <button data-inadd>+ вход</button></div>
      <div class="pblock"><label>secrets — env-ключи для плагинов (v0.5)</label>
        ${secretsBlock()}</div>
      <div class="pblock"><label>network — сетевая политика (v0.6)</label>
        <label style="cursor:pointer"><input type="checkbox" data-ndeny ${state.doc.network === 'deny' ? 'checked' : ''}/> deny — запретить сеть</label>
        <div class="hint">Выкл (allow): плагин сам декларирует сеть в манифесте (declare-now, аудит — журнал).
        deny: шаг, чей плагин заявил сеть, — ошибка (валидатор и раннер: WEDRA_NETWORK=deny).</div></div>
      <h3>Батч (pipeline foreach)</h3>
      ${pipelineFlowBlock()}
      <div class="hint">Шагов: ${state.doc.steps.length}. Перетащи плагин слева на холст (или кликни по нему — узел появится на свободном месте).
      type-объявления input — пока в YAML вручную (редактор их не трогает и такие файлы не сохраняет).</div>`;
  }
  el.innerHTML = html;
  wireProps(st);
}

function wireProps(st) {
  const el = $('#props');
  el.querySelectorAll('[data-bind]').forEach(sel => {
    sel.onchange = () => {
      pushUndo();
      if (st.bind) {
        if (sel.value) st.bind[sel.dataset.bind] = sel.value;
        else delete st.bind[sel.dataset.bind];
      }
      renderAll();
    };
  });
  const sid = el.querySelector('[data-sid]');
  if (sid) sid.onchange = () => {
    const old = st.id, v = sid.value.trim();
    if (!v || v === old) { sid.value = old; return; }
    if (state.doc.steps.some(s => s.id === v)) { sid.value = old; alert('id уже занят'); return; }
    pushUndo();
    // переименовываем и все ссылки в чужих bind
    for (const s of state.doc.steps) for (const [k, src] of Object.entries(s.bind || {})) {
      if (src && src.startsWith('steps.' + old + '.')) s.bind[k] = src.replace('steps.' + old + '.', 'steps.' + v + '.');
    }
    st.id = v; state.sel = v;
    renderAll();
  };
  const serr = el.querySelector('[data-serr]');
  if (serr) serr.onchange = () => {
    pushUndo();
    st.on_error = serr.value;
    // v0.29: on_error=retry без блока — заполняем дефолты (3/1s/fixed)
    if (serr.value === 'retry' && !st.retry) st.retry = { attempts: 3, delay: '1s', backoff: 'fixed' };
    renderAll();
  };
  const sret = el.querySelector('[data-sretry]');
  if (sret) sret.onchange = () => {
    pushUndo();
    if (sret.checked) {
      st.retry = st.retry || { attempts: 3, delay: '1s', backoff: 'fixed' };
    } else {
      st.retry = null;
      if (st.on_error === 'retry') st.on_error = 'stop';
    }
    renderAll();
  };
  const sra = el.querySelector('[data-srattempts]');
  if (sra) sra.onchange = () => { pushUndo(); if (st.retry) st.retry.attempts = Math.max(1, parseInt(sra.value, 10) || 1); renderAll(); };
  const srd = el.querySelector('[data-srdelay]');
  if (srd) srd.onchange = () => { pushUndo(); if (st.retry) st.retry.delay = srd.value.trim(); renderAll(); };
  const srb = el.querySelector('[data-srbackoff]');
  if (srb) srb.onchange = () => { pushUndo(); if (st.retry) st.retry.backoff = srb.value; renderAll(); };
  const sto = el.querySelector('[data-stimeout]');
  if (sto) sto.onchange = () => { pushUndo(); st.timeout = sto.value.trim(); renderAll(); };
  const wop = el.querySelector('[data-whenop]');
  if (wop) wop.onchange = () => {
    pushUndo();
    const op = wop.value;
    if (!op) { st.when = null; }
    else {
      const cur = st.when || {};
      st.when = {
        path: cur.path || '', op,
        value: NO_VALUE_OPS.includes(op) ? undefined : (cur.value !== undefined ? cur.value : ''),
      };
    }
    renderAll();
  };
  const wpath = el.querySelector('[data-whenpath]');
  if (wpath) wpath.onchange = () => { pushUndo(); if (st.when) st.when.path = wpath.value; renderAll(); };
  const wval = el.querySelector('[data-whenv]');
  if (wval) wval.onchange = () => { pushUndo(); if (st.when) st.when.value = wval.value; renderAll(); };
  const sf = el.querySelector('[data-sforeach]');
  if (sf) sf.onchange = () => { pushUndo(); st.foreach = sf.value; renderAll(); };
  const sfi = el.querySelector('[data-sfitem]');
  if (sfi) sfi.onchange = () => { pushUndo(); st.foreach_item = sfi.value.trim(); renderAll(); };
  const spg = el.querySelector('[data-spgrp]');
  if (spg) spg.onchange = () => { pushUndo(); st.parallel_group = spg.value.trim(); renderAll(); };
  const sa = el.querySelector('[data-safter]');
  if (sa) sa.onchange = () => { pushUndo(); st.after_foreach = sa.checked; renderAll(); };
  const gform = el.querySelector('[data-gform]');
  if (gform) gform.onchange = () => {
    pushUndo();
    st.form = gform.value.split('\n').map(l => l.trim()).filter(Boolean).map(l => {
      if (l.startsWith('e:')) return { field: l.slice(2), editable: true };
      return { field: l, editable: false };
    });
    renderAll();
  };
  const gact = el.querySelector('[data-gactions]');
  if (gact) gact.onchange = () => {
    pushUndo();
    st.actions = gact.value.split('\n').map(l => l.trim()).filter(Boolean);
    renderAll();
  };
  const gonr = el.querySelector('[data-gonreject]');
  if (gonr) gonr.onchange = () => { pushUndo(); st.on_reject = gonr.value; renderAll(); };
  const del = el.querySelector('[data-del]');
  if (del) del.onclick = () => {
    pushUndo();
    state.doc.steps = state.doc.steps.filter(s => s.id !== st.id);
    // чужие bind на удалённый — очищаем
    for (const s of state.doc.steps) for (const [k, src] of Object.entries(s.bind || {})) {
      if (src && src.startsWith('steps.' + st.id + '.')) delete s.bind[k];
    }
    state.sel = null;
    renderAll();
  };
  const pn = el.querySelector('[data-pname]');
  if (pn) pn.onchange = () => { pushUndo(); state.doc.name = pn.value.trim() || state.doc.name; renderAll(); };
  el.querySelectorAll('[data-iname]').forEach(i => i.onchange = () => { pushUndo(); state.doc.input[i.dataset.iname].name = i.value.trim(); renderAll(); });
  el.querySelectorAll('[data-indef]').forEach(i => i.onchange = () => { pushUndo(); state.doc.input[i.dataset.indef].default = i.value; renderAll(); });
  el.querySelectorAll('[data-indel]').forEach(b => b.onclick = () => { pushUndo(); state.doc.input.splice(+b.dataset.indel, 1); renderAll(); });
  const pf = el.querySelector('[data-pforeach]');
  if (pf) pf.onchange = () => { pushUndo(); state.doc.foreach = pf.value; renderAll(); };
  const pfi = el.querySelector('[data-pfitem]');
  if (pfi) pfi.onchange = () => { pushUndo(); state.doc.foreach_item = pfi.value.trim(); renderAll(); };
  const pt = el.querySelector('[data-ptype]');
  if (pt) pt.onchange = () => { pushUndo(); state.doc.item_type = pt.value.trim(); renderAll(); };
  const pfmt = el.querySelector('[data-pformat]');
  if (pfmt) pfmt.onchange = () => { pushUndo(); state.doc.item_format = pfmt.value.trim(); renderAll(); };
  const inadd = el.querySelector('[data-inadd]');
  if (inadd) inadd.onclick = () => { pushUndo(); state.doc.input.push({ name: 'field' + (state.doc.input.length + 1), default: '' }); renderAll(); };
  // v0.5: secrets — добавление/удаление env-ключей
  const skey = el.querySelector('[data-skey]');
  const sadd = el.querySelector('[data-sadd]');
  const addSecret = () => {
    const v = (skey && skey.value || '').trim();
    if (!v) return;
    if ((state.doc.secrets || []).includes(v)) { renderAll(); return; }
    pushUndo();
    state.doc.secrets = state.doc.secrets || [];
    state.doc.secrets.push(v);
    renderAll();
  };
  if (sadd) sadd.onclick = addSecret;
  if (skey) skey.onkeydown = e => { if (e.key === 'Enter') { e.preventDefault(); addSecret(); } };
  el.querySelectorAll('[data-sdel]').forEach(b => b.onclick = () => {
    pushUndo();
    state.doc.secrets.splice(+b.dataset.sdel, 1);
    renderAll();
  });
  // v0.6: network — политика allow/deny
  const ndeny = el.querySelector('[data-ndeny]');
  if (ndeny) ndeny.onchange = () => { pushUndo(); state.doc.network = ndeny.checked ? 'deny' : ''; renderAll(); };
}

function renderAll() {
  renderNodes();
  renderProps();
  scheduleValidate();
}

// ── канвас: drop из палитры, drag узлов ───────────────────────────────────
function initCanvas() {
  const wrap = $('#canvas-wrap'), canvas = $('#canvas');
  // v0.8a: добавление из палитры — через mouse-события (startPaletteDrag);
  // нативный dragover/drop не работает в sandboxed iframe (превью)
  canvas.addEventListener('mousedown', e => {
    if (e.target === canvas || e.target.id === 'edges') { state.sel = null; renderAll(); }
  });
  window.addEventListener('keydown', e => {
    const tag = (e.target.tagName || '').toLowerCase();
    const typing = tag === 'input' || tag === 'textarea' || tag === 'select';
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z' && !e.shiftKey) { e.preventDefault(); undo(); return; }
    if ((e.ctrlKey || e.metaKey) && (e.key.toLowerCase() === 'y' || (e.key.toLowerCase() === 'z' && e.shiftKey))) { e.preventDefault(); redo(); return; }
    if ((e.key === 'Delete' || e.key === 'Backspace') && state.sel && !typing) {
      const st = state.doc.steps.find(s => s.id === state.sel);
      if (st) { e.preventDefault();
        pushUndo();
        state.doc.steps = state.doc.steps.filter(s => s.id !== st.id);
        for (const s of state.doc.steps) for (const [k, src] of Object.entries(s.bind || {})) {
          if (src && src.startsWith('steps.' + st.id + '.')) delete s.bind[k];
        }
        state.sel = null;
        renderAll();
      }
    }
  });
}

function startNodeDrag(e, st) {
  if (e.button !== 0) return;
  const nh = e.target.closest('.nh');
  if (!nh) return;
  e.preventDefault();
  const wrap = $('#canvas-wrap'), node = nh.closest('.node');
  const startX = e.clientX, startY = e.clientY;
  const ox = st.pos[0], oy = st.pos[1];
  let moved = false;
  const mm = ev => {
    const dx = ev.clientX - startX, dy = ev.clientY - startY;
    if (Math.abs(dx) + Math.abs(dy) > 3) moved = true;
    node.style.left = (ox + dx) + 'px';
    node.style.top = (oy + dy) + 'px';
  };
  const mu = ev => {
    document.removeEventListener('mousemove', mm);
    document.removeEventListener('mouseup', mu);
    if (moved) {
      pushUndo();
      const dx = ev.clientX - startX, dy = ev.clientY - startY;
      st.pos = [Math.max(0, snap(ox + dx)), Math.max(0, snap(oy + dy))];
      renderAll();
    }
  };
  document.addEventListener('mousemove', mm);
  document.addEventListener('mouseup', mu);
}

// ── валидация / сериализация / сохранение ─────────────────────────────────
let valTimer = null;
function scheduleValidate() {
  state.validating = true;
  $('#val-badge').textContent = '…';
  $('#val-badge').className = 'badge run';
  clearTimeout(valTimer);
  valTimer = setTimeout(doValidate, 500);
}
async function doValidate() {
  try {
    const res = await api('/api/serialize/pipeline', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(state.doc),
    });
    state.yaml = res.yaml || '';
    state.valErrs = res.errors || [];
    state.valWarns = res.warnings || [];
    const b = $('#val-badge');
    if (state.valErrs.length) { b.textContent = 'ошибки: ' + state.valErrs.length; b.className = 'badge err'; }
    else { b.textContent = 'валиден' + (state.valWarns.length ? ' · ' + state.valWarns.length + ' warn' : ''); b.className = 'badge ok'; }
    const ve = $('#val-err');
    if (ve) {
      ve.style.display = state.valErrs.length ? 'block' : 'none';
      ve.textContent = state.valErrs.join('\n');
    }
  } catch (e) {
    const b = $('#val-badge');
    b.textContent = 'ошибка'; b.className = 'badge err';
    state.valErrs = [String(e.message || e)];
  } finally {
    state.validating = false;
  }
}

async function save() {
  const file = (state.doc.file || '').replace(/\.ya?ml$/i, '') + '.yaml';
  $('#file-name').value = file;
  try {
    await doValidate();
  } catch {}
  if (state.validating) return;
  if (state.unsupported.length) { alert('Нельзя сохранить: редактор не управляет полями: ' + state.unsupported.join(', ')); return; }
  if (state.valErrs.length) { alert('Сначала исправь ошибки:\n' + state.valErrs.join('\n')); return; }
  const st = $('#save-status');
  try {
    await apiRaw('/api/pipelines/' + encodeURIComponent(file), { method: 'PUT', body: state.yaml });
    st.textContent = 'сохранено: ' + file; st.className = 'badge ok';
    await loadFileList();
  } catch (e) {
    st.textContent = 'ошибка'; st.className = 'badge err';
    alert('Сохранение: ' + e.message);
  }
}

// ── открытие / список файлов ──────────────────────────────────────────────
async function loadFileList() {
  try {
    const list = await api('/api/pipelines');
    const sel = $('#file-open');
    const cur = state.doc.file;
    sel.innerHTML = '<option value="">— открыть… —</option>' +
      list.filter(p => !p.error).map(p => `<option value="${esc(p.file)}"${p.file === cur ? ' selected' : ''}>${esc(p.name || p.file)}</option>`).join('');
  } catch (e) { console.error(e); }
}

async function openFile(file) {
  try {
    const yamlText = await apiRaw('/api/pipelines/' + encodeURIComponent(file));
    const doc = await api('/api/parse/pipeline', { method: 'POST', body: yamlText });
    pushUndo();
    state.doc = {
      name: doc.name, file, format_version: doc.format_version || '',
      input: (doc.input || []).map(i => ({ name: i.name, default: i.default || '' })),
      steps: (doc.steps || []).map(s => ({
        id: s.id, plugin: s.plugin,
        pos: Array.isArray(s.pos) && s.pos.length === 2 ? [s.pos[0], s.pos[1]] : [20 * (1 + Math.random() * 8), 20 * (1 + Math.random() * 6)],
        on_error: s.on_error || 'stop', timeout: s.timeout || '',
        bind: s.bind || {}, form: s.form || [], actions: s.actions || [], on_reject: s.on_reject || '',
        when: s.when || null,
        foreach: s.foreach || '', foreach_item: s.foreach_item || '',
        after_foreach: !!s.after_foreach, parallel_group: s.parallel_group || '',
        retry: s.retry || null,
      })),
      foreach: doc.foreach || '', foreach_item: doc.foreach_item || '',
      item_type: doc.item_type || '', item_format: doc.item_format || '',
      secrets: doc.secrets || [], network: doc.network || '',
    };
    state.unsupported = doc.unsupported || [];
    state.sel = null;
    $('#file-name').value = file;
    renderAll();
  } catch (e) {
    alert('Не удалось открыть: ' + e.message);
  }
}

// ── YAML-просмотр ─────────────────────────────────────────────────────────
async function showYaml() {
  try {
    const res = await api('/api/serialize/pipeline', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(state.doc),
    });
    state.yaml = res.yaml || '';
    $('#yaml-pre').textContent = state.yaml +
      (res.errors && res.errors.length ? '\n\nошибки валидации:\n' + res.errors.join('\n') : '');
    $('#yaml-view').style.display = 'block';
  } catch (e) {
    $('#yaml-pre').textContent = 'ошибка сериализации: ' + e.message;
    $('#yaml-view').style.display = 'block';
  }
}
window.closeYaml = () => { $('#yaml-view').style.display = 'none'; };

// ── init ──────────────────────────────────────────────────────────────────
async function init() {
  try { state.plugins = await api('/api/plugins'); } catch (e) { console.error(e); }
  renderPalette();
  initCanvas();
  await loadFileList();
  $('#file-open').onchange = e => { if (e.target.value) openFile(e.target.value); };
  $('#btn-save').onclick = save;
  $('#btn-yaml').onclick = showYaml;
  $('#btn-undo').onclick = undo;
  $('#btn-redo').onclick = redo;
  renderAll();
}
init();
