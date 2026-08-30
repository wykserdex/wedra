// v0.25 — редактор пайплайнов: палитра → холст (сетка 20px), bind-связи,
// undo/redo, валидация и сериализация через ядро (Go), сохранение PUT.
// Честный скоуп среза: when/foreach/parallel/retry/secrets/network не
// управляются — такие пайплайны открываются с баннером и без сохранения.
const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>\"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','\"':'&quot;'}[c]));
const GRID = 20;
const snap = v => Math.round(v / GRID) * GRID;

let state = {
  plugins: [],
  doc: { name: 'new_pipeline', file: 'new_pipeline.yaml', input: [], steps: [] },
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
    on_error: 'stop', timeout: '', bind: {},
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
    return `<div class="plug" draggable="true" data-plugin="${esc(p.id)}">
      <b>${esc(p.id)}</b>
      <small>${esc(p.description || '')}</small>
      <div class="io">in: ${ins} · out: ${outs}</div>
    </div>`;
  }).join('') || '<div class="empty">плагинов нет</div>';
  el.querySelectorAll('.plug').forEach(el2 => {
    el2.addEventListener('dragstart', e => e.dataTransfer.setData('plugin', el2.dataset.plugin));
  });
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
      <div class="foot"><span>on_err: ${esc(st.on_error || 'stop')}</span>${st.timeout ? `<span>${esc(st.timeout)}</span>` : ''}</div>`;
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
function renderProps() {
  const el = $('#props');
  let html = '';
  if (state.unsupported.length) {
    html += `<div id="banner" style="display:block">Пайплайн содержит поля, которые редактор v0.25 не управляет:
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
          <select data-serr><option value="stop"${st.on_error === 'stop' ? ' selected' : ''}>stop</option><option value="skip"${st.on_error === 'skip' ? ' selected' : ''}>skip</option></select></div>
      </div>
      <div class="pblock"><label>плагин</label><input value="${esc(st.plugin)}" readonly style="color:var(--dim)"/></div>
      <div class="pblock"><label>timeout (пусто = 60s): 10s, 1m30s, …</label><input data-stimeout value="${esc(st.timeout || '')}" placeholder="60s" spellcheck="false"/></div>
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
      <div class="hint">Шагов: ${state.doc.steps.length}. Перетащи плагин слева на холст, чтобы добавить.
      Когда/foreach/parallel — пока в YAML вручную (редактор их не трогает и такие файлы не сохраняет).</div>`;
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
  if (serr) serr.onchange = () => { pushUndo(); st.on_error = serr.value; renderAll(); };
  const sto = el.querySelector('[data-stimeout]');
  if (sto) sto.onchange = () => { pushUndo(); st.timeout = sto.value.trim(); renderAll(); };
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
  const inadd = el.querySelector('[data-inadd]');
  if (inadd) inadd.onclick = () => { pushUndo(); state.doc.input.push({ name: 'field' + (state.doc.input.length + 1), default: '' }); renderAll(); };
}

function renderAll() {
  renderNodes();
  renderProps();
  scheduleValidate();
}

// ── канвас: drop из палитры, drag узлов ───────────────────────────────────
function initCanvas() {
  const wrap = $('#canvas-wrap'), canvas = $('#canvas');
  wrap.addEventListener('dragover', e => e.preventDefault());
  wrap.addEventListener('drop', e => {
    e.preventDefault();
    const plugin = e.dataTransfer.getData('plugin');
    if (!plugin) return;
    const cRect = canvas.getBoundingClientRect();
    addStep(plugin, e.clientX - cRect.left - 115, e.clientY - cRect.top - 20);
  });
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
      name: doc.name, file,
      input: (doc.input || []).map(i => ({ name: i.name, default: i.default || '' })),
      steps: (doc.steps || []).map(s => ({
        id: s.id, plugin: s.plugin,
        pos: Array.isArray(s.pos) && s.pos.length === 2 ? [s.pos[0], s.pos[1]] : [20 * (1 + Math.random() * 8), 20 * (1 + Math.random() * 6)],
        on_error: s.on_error || 'stop', timeout: s.timeout || '',
        bind: s.bind || {}, form: s.form || [], actions: s.actions || [], on_reject: s.on_reject || '',
      })),
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
