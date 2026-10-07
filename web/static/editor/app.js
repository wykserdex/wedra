// v0.25 — редактор пайплайнов: палитра → холст (сетка 20px), bind-связи,
// undo/redo, валидация и сериализация через ядро (Go), сохранение PUT.
// v0.27: when — условие шага (path/op/value, 10 операторов ядра).
// v0.28: foreach/parallel_group/after_foreach на шаге + foreach-батч
// пайплайна (foreach/foreach_item/item_type/item_format).
// v0.29: retry (on_error: retry + retry{attempts, delay, backoff}).
// v0.30: input schema (type/required/default/format/description), step loops,
// round-trip всех полей цикла и работающий Save As по имени файла.
// v0.5: secrets (pipeline.secrets — env-ключи плагинов) в UI (чипы в
// блоке «Пайплайн» + подсказка в шаге, какой ключ просит плагин).
// v0.6: network (pipeline.network — политика allow/deny) в UI: чекбокс
// «deny» + подсказка в шаге, какую сеть просит плагин (и не запрещено ли).
// v0.8a: перетаскивание палитра→холст на mouse-событиях (ghost) + клик =
// добавить на свободное место. Нативный HTML5 DnD не работает в sandboxed
// iframe (превью) — mouse-события работают везде.
const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"'`]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;','`':'&#96;'}[c]));
const inputText = value => value === null || Array.isArray(value) || (typeof value === 'object')
  ? JSON.stringify(value, null, 2) : (value === undefined ? '' : String(value));
const parseInputText = (text, current, expectedType = '') => {
  if (expectedType === 'array' || (!expectedType && Array.isArray(current))) {
    const value = JSON.parse(text);
    if (!Array.isArray(value)) throw new Error('ожидается массив JSON');
    return value;
  }
  if (expectedType === 'object' || (!expectedType && current !== null && typeof current === 'object')) {
    const value = JSON.parse(text);
    if (value === null || Array.isArray(value) || typeof value !== 'object') throw new Error('ожидается объект JSON');
    return value;
  }
  if (expectedType === 'number' || (!expectedType && typeof current === 'number')) {
    const value = Number(text);
    if (text.trim() === '' || !Number.isFinite(value)) throw new Error('ожидается число');
    return value;
  }
  if (expectedType === 'boolean' || (!expectedType && typeof current === 'boolean')) {
    if (text !== 'true' && text !== 'false') throw new Error('ожидается true или false');
    return text === 'true';
  }
  if (expectedType === 'string') return text;
  if (current === null) return JSON.parse(text);
  return text;
};
const INPUT_TYPES = ['string', 'number', 'boolean', 'array', 'object'];
const defaultForInputType = type => ({string: '', number: 0, boolean: false, array: [], object: {}}[type] ?? '');
const valueMatchesInputType = (value, type) => {
  if (type === 'array') return Array.isArray(value);
  if (type === 'object') return value !== null && typeof value === 'object' && !Array.isArray(value);
  if (type === 'number') return typeof value === 'number' && Number.isFinite(value);
  return typeof value === type;
};
const inputValueEditor = (input, idx) => {
  const value = input.default;
  const type = input.typed ? input.type : '';
  const jsonValue = value === null || Array.isArray(value) || typeof value === 'object';
  if (type === 'array' || type === 'object' || (!type && jsonValue)) {
    return `<textarea data-indef="${idx}" rows="3" spellcheck="false">${esc(inputText(value))}</textarea>`;
  }
  if (type === 'boolean' || (!type && typeof value === 'boolean')) {
    const valid = typeof value === 'boolean';
    return `<select data-indef="${idx}">${valid ? '' : '<option value="" selected disabled>исправь default</option>'}<option value="true"${value === true ? ' selected' : ''}>true</option><option value="false"${value === false ? ' selected' : ''}>false</option></select>`;
  }
  const number = type === 'number' || (!type && typeof value === 'number');
  const display = type === 'string' && typeof value !== 'string' && value != null ? JSON.stringify(value) : inputText(value);
  return `<input data-indef="${idx}" type="${number ? 'number' : 'text'}"${number ? ' step="any"' : ''} value="${esc(display)}" placeholder="значение по умолчанию" spellcheck="false"/>`;
};
const inputEditor = (input, idx) => {
  const typeOpts = INPUT_TYPES.map(v => `<option value="${v}"${input.type === v ? ' selected' : ''}>${v}</option>`).join('');
  const required = input.required === true ? 'true' : input.required === false ? 'false' : '';
  const mismatch = input.typed && input.has_default && !valueMatchesInputType(input.default, input.type);
  const value = input.has_default ? `<div class="input-value">${inputValueEditor(input, idx)}</div>` : '';
  return `<div class="input-editor">
    <div class="input-row">
      <input data-iname="${idx}" value="${esc(input.name)}" placeholder="имя" spellcheck="false"/>
      <select data-ikind="${idx}" title="формат входа"><option value="value"${input.typed ? '' : ' selected'}>значение</option><option value="schema"${input.typed ? ' selected' : ''}>схема</option></select>
      <button data-indel="${idx}" title="удалить вход">×</button>
    </div>
    ${input.typed ? `<div class="input-schema">
      <div class="input-meta-grid">
        <label>тип<select data-itype="${idx}">${typeOpts}</select></label>
        <label>обязательность<select data-irequired="${idx}">
          <option value=""${required === '' ? ' selected' : ''}>не задана</option>
          <option value="true"${required === 'true' ? ' selected' : ''}>обязательный</option>
          <option value="false"${required === 'false' ? ' selected' : ''}>необязательный</option>
        </select></label>
      </div>
      <div class="input-meta-grid">
        <label>format<input data-iformat="${idx}" value="${esc(input.format || '')}" placeholder="email, url…" spellcheck="false"/></label>
        <label>описание<input data-idescription="${idx}" value="${esc(input.description || '')}" placeholder="для чего поле" spellcheck="false"/></label>
      </div>
      <label class="input-default-toggle"><input type="checkbox" data-ihasdefault="${idx}"${input.has_default ? ' checked' : ''}/> значение по умолчанию</label>
      ${value}
      ${mismatch ? `<div class="hint" style="color:var(--err)">default не соответствует type: ${esc(input.type)} — исправь значение или тип.</div>` : ''}
    </div>` : value}
  </div>`;
};
const GRID = 20;
const snap = v => Math.round(v / GRID) * GRID;

let state = {
  plugins: [],
  doc: { name: 'new_pipeline', file: 'new_pipeline.yaml', format_version: '', input: [], steps: [],
    secrets: [], network: '', gates: '',
    foreach: '', foreach_item: '', item_type: '', item_format: '' },
  unsupported: [],
  sel: null,
  // Связь хранится ГОТОВЫМ значением bind, а не парой {step, field}: источником
  // может быть не только выход шага (steps.a.lines), но и вход пайплайна
  // (input.photo). С жёсткой формой steps.X.Y артефакт было бы не к чему
  // привязать, и пришлось бы городить отдельный путь.
  link: null, // {value, label, step, field}
  // v0.37: протяжка — {step, field|null, x1, y1, x, y}. field=null означает
  // «зажат узел целиком»: выход выбирается сам, если он один.
  // v0.38: + value/label — для протяжки от карточки артефакта (input.<имя>).
  linkDrag: null,
  // v0.37: неоднозначный дроп — пары-кандидаты ждут щелчка в панели связи.
  linkCandidates: [],
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
  if (!r.ok) {
    // H4: 401 — нет сессии человека (cookie выдаётся обменом одноразового кода)
    if (r.status === 401) { sessionOverlay(); throw new Error('нет сессии: нужен одноразовый код из терминала wedra'); }
    throw new Error(typeof body === 'string' ? body : JSON.stringify(body));
  }
  return body;
}
async function apiRaw(path, opts = {}) {
  const r = await fetch(path, opts);
  const text = await r.text();
  if (!r.ok) {
    if (r.status === 401) { sessionOverlay(); throw new Error('нет сессии: нужен одноразовый код из терминала wedra'); }
    throw new Error(text);
  }
  return text;
}

// ── H4: сессия человека ──────────────────────────────────────────────────
// Всё под /api/* (кроме /api/health) требует cookie. Cookie выдаётся обменом
// ОДНОРАЗОВОГО кода, напечатанного в терминале, где запущен wedra. Пока cookie
// нет, редактор показывает поле ввода кода вместо пустого холста: тот же путь,
// что и у `wedra gui --open` (там код едет в ссылке ?c= и ставит cookie сразу).
function sessionOverlay() {
  if (document.getElementById('wedra-session-box')) return;
  const box = document.createElement('div');
  box.id = 'wedra-session-box';
  // стили — в editor/index.html (#wedra-session-box): разметка не тащит цвета в JS
  box.innerHTML =
    '<div class="sbox">' +
    '<h2>Вход в WEDRA</h2>' +
    '<p>Код входа одноразовый и напечатан в терминале, где запущен wedra. Он нужен один раз: ' +
    'обменяется на cookie, и редактор перезагрузится сам.</p>' +
    '<input id="wedra-code" placeholder="XXXX-XXXX-XXXX" autocomplete="off"/>' +
    '<div id="wedra-code-err" class="serr"></div>' +
    '<button id="wedra-code-go">Войти</button>' +
    '</div>';
  document.body.appendChild(box);
  const input = box.querySelector('#wedra-code');
  const err = box.querySelector('#wedra-code-err');
  const submit = async () => {
    const code = input.value.trim();
    if (!code) return;
    err.textContent = '';
    const r = await fetch('/api/session', {
      method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({code}),
    });
    if (r.ok) { location.reload(); return; }
    const d = await r.json().catch(() => ({}));
    err.textContent = d.error || 'код не подошёл';
    input.select();
  };
  box.querySelector('#wedra-code-go').onclick = submit;
  input.onkeydown = e => { if (e.key === 'Enter') submit(); };
  input.focus();
}

// wedraSession — можно ли работать. Сессия выключена (--no-session) или уже
// есть — сразу true; иначе показываем поле кода и ждём перезагрузки.
async function wedraSession() {
  let st = null;
  try { st = await (await fetch('/api/session')).json(); } catch (e) { return true; }
  if (!st || !st.required || st.authenticated) return true;
  sessionOverlay();
  return false;
}

// ── manifest-хелперы ─────────────────────────────────────────────────────
// Сопоставление шага пайплайна с плагином из /api/plugins.
//
// Разделители путей приводятся к одному виду: шаг в YAML записан как
// `plugins/community/text_analyzer`, а API на Windows отдаёт `dir` с
// обратными слэшами (`plugins\community\text_analyzer`). Без нормализации
// сравнение не сходилось НИГДЕ — узлы оставались без входов и выходов, списки
// bind были пустыми, то есть редактор на Windows был слепым.
const slash = s => String(s || '').replace(/\\/g, '/');

function pluginInfo(id) {
  if (id === 'core/human_gate') return { id, input: {}, output: {}, description: 'человек в петле' };
  const want = slash(id);
  const tail = want.slice(want.lastIndexOf('/') + 1);
  return state.plugins.find(p => p.id === id || p.id === tail || slash(p.dir).endsWith('/' + want) || slash(p.dir).endsWith('/' + tail)) || null;
}
function inFields(p) {
  const info = pluginInfo(p);
  return info ? Object.keys(info.input || {}) : [];
}
function outFields(p) {
  const info = pluginInfo(p);
  return info ? Object.keys(info.output || {}) : [];
}
// Поиск шага и его узла на холсте — сносок для протяжки.
function stepById(id) {
  return state.doc.steps.find(s => s.id === id) || null;
}
function nodeById(id) {
  return document.querySelector(`.node[data-id="${id}"]`);
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
  // снимок не знает ни про вооружение, ни про протяжку, ни про кандидатов:
  // после undo/redo источник связи мог исчезнуть или сменить выход.
  state.link = null;
  state.linkDrag = null;
  state.linkCandidates = [];
  const fileInput = $('#file-name');
  if (fileInput) fileInput.value = state.doc.file || '';
  const fileSelect = $('#file-open');
  if (fileSelect) fileSelect.value = state.doc.file || '';
  renderAll();
}
function redo() {
  if (!state.redo.length) return;
  state.undo.push(JSON.stringify(state.doc));
  state.doc = JSON.parse(state.redo.pop());
  state.sel = null;
  state.link = null;
  const fileInput = $('#file-name');
  if (fileInput) fileInput.value = state.doc.file || '';
  const fileSelect = $('#file-open');
  if (fileSelect) fileSelect.value = state.doc.file || '';
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
    foreach: '', foreach_item: '', after_foreach: false, parallel_group: '',
    loop: '', loop_condition: '', max_iterations: 0, retry: null,
  };
  if (plugin === 'core/human_gate') {
    st.form = []; st.actions = ['accept', 'reject']; st.on_reject = 'stop'; st.approval = '';
  }
  state.doc.steps.push(st);
  state.sel = st.id;
  renderAll();
  return st; // v0.38: подсказкам цепочки нужен созданный шаг, чтобы связать его
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
  //
  // Область сбора — вся палитра (#palette), а не только #plugin-list. Карточка
  // core/human_gate лежит в разметке РЯДОМ с #plugin-list (index.html), и
  // раньше подписывалась только #plugin-list — то есть гейт нельзя было ни
  // перетащить, ни добавить кликом, хотя выглядел он как остальные.
  document.querySelectorAll('#palette .plug').forEach(el2 => {
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
  // Подсказку показываем только на пустом холсте: дальше она перекрывала бы
  // узлы и мешала работать.
  const hint = $('#canvas-hint');
  if (hint) hint.classList.toggle('hidden', state.doc.steps.length > 0);
  const link = state.link;
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
          const src = (st.bind || {})[f] || '';
          // Пока оружие «связать» заряжено, поля-цели подсвечиваются, а поля
          // ЭТОГО шага — нет: связать шаг с самим собой бессмысленно.
          const isTarget = link && link.step !== st.id;
          const cls = isTarget ? ' linkable' : '';
          return `<div class="inrow${cls}" data-field="${esc(f)}" title="${isTarget ? 'принять связь в это поле' : ''}"><span>${esc(f)}</span><span class="src ${src ? 'set' : ''}">${esc(src || '—')}</span></div>`;
        }).join('') || '<div class="inrow"><span>входов нет</span></div>');
    const outs = gate
      ? '<span class="outchip" title="выходы = поля формы">поля формы</span>'
      : (outFields(st.plugin).map(o => {
          const armed = link && link.step === st.id && link.field === o;
          return `<span class="outchip${armed ? ' armed' : ''}" data-out="${esc(o)}" title="зажми и веди к другому шагу — или щелчок, чтобы вооружить связь">${esc(o)}</span>`;
        }).join('') || '<span class="outchip">нет</span>');
    node.innerHTML = `
      <div class="nh"><span class="nid">${esc(st.id)}</span><span class="nplug">${gate ? 'human_gate' : esc((info && (info.id === st.plugin ? st.plugin.split('/').pop() : st.plugin)) || st.plugin)}</span></div>
      <div class="body">${ins}<div style="margin-top:6px">${outs}</div></div>
      <div class="foot"><span>on_err: ${esc(st.on_error || 'stop')}</span>${st.when ? `<span title="when: ${esc(st.when.path || '')}">⚖ when:${esc(st.when.op || '')}</span>` : ''}${st.foreach ? '<span title="foreach: ' + esc(st.foreach) + '">⤨ foreach</span>' : ''}${st.parallel_group ? `<span title="parallel_group">∥ ${esc(st.parallel_group)}</span>` : ''}${st.after_foreach ? `<span title="after_foreach">⤓ post</span>` : ''}${st.loop ? `<span title="цикл шага">⟳ loop</span>` : ''}${st.timeout ? `<span>${esc(st.timeout)}</span>` : ''}</div>`;
    canvas.appendChild(node);
    node.addEventListener('mousedown', e => startNodeDrag(e, st));
    node.addEventListener('click', e => { e.stopPropagation(); state.sel = st.id; renderAll(); });
    // Связывание: щелчок по выходу вооружает связь, щелчок по полю входа
    // другого шага — принимает её. Всё остальное (перетаскивание, выделение)
    // остаётся как было.
    node.querySelectorAll('.outchip[data-out]').forEach(chip => {
      // Зажали на выходе — поехали протяжка. Отпустили тут же, без движения:
      // сработает click ниже и связь просто вооружится вторым щелчком.
      chip.addEventListener('mousedown', e => startLinkDrag(st, chip.dataset.out, e));
      chip.addEventListener('click', e => {
        e.stopPropagation();
        if (state.linkCandidates.length) return;
        armLink(st, chip.dataset.out);
      });
    });
    // Зажали на самом узле (не на шапке — там перетаскивание узла) — тянем
    // связь. Источник выхода выберется сам, если он один.
    node.addEventListener('mousedown', e => {
      if (e.target.closest('.nh') || e.target.closest('.inrow[data-field]')) return;
      startLinkDrag(st, null, e);
    });
    if (link) {
      node.querySelectorAll('.inrow[data-field]').forEach(row => {
        if (st.id === link.step) return;
        row.addEventListener('click', e => {
          e.stopPropagation();
          applyLink(st, link, row.dataset.field);
        });
      });
    }
  }
  renderLinkBar();
  renderEdges();
}

// armLink — вооружить связь из (шаг, поле). Повторный щелчок по тому же выходу
// и щелчок по другому выходу переключают источник; повторный по тому же —
// отмена. Связь со своим шагом невозможна, поэтому клик по выходу шага,
// который уже вооружён, просто переставляет источник.
function armLink(st, field) {
  const cur = state.link;
  state.link = (cur && cur.step === st.id && cur.field === field)
    ? null
    : { value: `steps.${st.id}.${field}`, label: `${st.id}.${field}`, step: st.id, field };
  renderAll();
}

function cancelLink() {
  if (!state.link && !state.linkDrag && !state.linkCandidates.length) return;
  state.link = null;
  state.linkDrag = null;
  state.linkCandidates = [];
  renderAll();
}

// ─── v0.37: протяжка, как в канвасе Obsidian ────────────────────────────────
//
// Зажал на плагине (или на конкретном выходе) → ведёшь резиновую линию →
// отпустил на другом шаге. Имени поля нигде вводить не надо.
//
// Формат пайплайна не умеет «просто связать два узла»: в `bind` обязано быть
// имя поля-приёмника, поэтому либо источник, либо приёмник приходится выбрать.
// Отсюда правило: тянем с выхода — попадаем в поле (точная связь); тянем с узла
// целиком — выход выбирается сам, если он один, иначе в панели связи появляется
// список пар-кандидатов, где нужно щёлкнуть одну. Ни ввода, ни угадывания.

// Канвас-координаты из клиентских: узлы лежат внутри #canvas со скроллом.
function canvasXY(ev) {
  const cRect = $('#canvas').getBoundingClientRect();
  return { x: ev.clientX - cRect.left, y: ev.clientY - cRect.top };
}

function startLinkDrag(st, field, ev) {
  ev.preventDefault();
  ev.stopPropagation();
  const node = nodeById(st.id);
  const host = field
    ? node.querySelector(`.outchip[data-out="${field}"]`)
    : node;
  const r = host.getBoundingClientRect();
  const cRect = $('#canvas').getBoundingClientRect();
  const p = canvasXY(ev);
  state.linkDrag = {
    step: st.id,
    field: field || null,
    x1: (field ? r.right : r.left) - cRect.left,
    y1: r.top + r.height / 2 - cRect.top,
    x: p.x,
    y: p.y,
  };
  state.linkCandidates = [];
  renderAll();
  const move = e => {
    const q = canvasXY(e);
    state.linkDrag.x = q.x;
    state.linkDrag.y = q.y;
    drawGhost();
    // Подсветка цели: без неё тянуть приходится на ощупь.
    const el = document.elementFromPoint(e.clientX, e.clientY);
    const hit = el && el.closest ? el.closest('.inrow[data-field],.node[data-id]') : null;
    const prev = $('#canvas').querySelector('.hovered');
    if (prev) prev.classList.remove('hovered');
    if (hit && hit.closest('.node') !== nodeById(st.id)) hit.classList.add('hovered');
  };
  const up = e => {
    document.removeEventListener('mousemove', move);
    document.removeEventListener('mouseup', up);
    const h = $('#canvas').querySelector('.hovered');
    if (h) h.classList.remove('hovered');
    dropLink(e);
  };
  document.addEventListener('mousemove', move);
  document.addEventListener('mouseup', up);
}

// Резиновая линия. Обновляем только свой <path>, а не весь svg: mousemove
// идёт на каждый кадр, перерисовывать все рёбра там незачем.
function drawGhost() {
  const d = state.linkDrag;
  const svg = document.getElementById('edges');
  if (!svg) return;
  let g = svg.querySelector('#link-ghost');
  if (!d) { if (g) g.remove(); return; }
  if (!g) {
    g = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    g.id = 'link-ghost';
    svg.appendChild(g);
  }
  const dx = Math.max(34, Math.abs(d.x - d.x1) * 0.45);
  g.setAttribute('d',
    `M ${d.x1} ${d.y1} C ${d.x1 + dx} ${d.y1}, ${d.x - dx} ${d.y}, ${d.x} ${d.y}`);
  g.setAttribute('class', 'ghost');
}

// Что под курсором в момент отпускания. Прямое наведение на строку входа даёт
// самую точную связь, поэтому проверяем её раньше узла.
function dropLink(ev) {
  const d = state.linkDrag;
  state.linkDrag = null;
  if (!d) return;
  const el = document.elementFromPoint(ev.clientX, ev.clientY);
  const node = el && el.closest ? el.closest('.node') : null;
  // Мимо узла: не перерисовываем. Иначе пересозданный чит/строка потеряет
  // событие click, и двукличковая привязка перестанет работать.
  if (!node || !node.dataset || !node.dataset.id) { drawGhost(); return; }
  const toStep = node.dataset.id;
  if (toStep === d.step) { drawGhost(); return; } // в самого себя — нельзя
  const st = stepById(toStep);
  if (!st) { drawGhost(); return; }


  // Тянем с узла целиком, а выход у него один — связываем не раздумывая.
  const outs = d.field ? [d.field] : outFields(stepById(d.step).plugin);
  const row = el.closest ? el.closest('.inrow[data-field]') : null;
  const ins = row && row.closest('.node') === node
    ? [row.dataset.field]
    : inFields(st.plugin);

  if (outs.length === 1 && ins.length === 1) {
    applyLink(st, { value: `steps.${d.step}.${outs[0]}`, label: `${d.step}.${outs[0]}` }, ins[0]);
    return;
  }
  // Неоднозначно: честно перечисляем пары и ждём щелчка, вместо того чтобы
  // молча угадать и связать не то.
  state.linkCandidates = [];
  for (const o of outs) for (const f of ins) {
    state.linkCandidates.push({
      step: toStep, field: f, fromStep: d.step, fromField: o,
      value: `steps.${d.step}.${o}`, label: `${d.step}.${o}`,
    });
  }
  renderAll();
}

function applyLink(st, link, field) {
  pushUndo();
  st.bind = st.bind || {};
  st.bind[field] = link.value;
  state.link = null;
  state.linkDrag = null;
  state.linkCandidates = [];
  state.sel = st.id;
  renderAll();
}

function renderLinkBar() {
  const bar = $('#link-bar');
  if (!bar) return;
  const link = state.link;
  const drag = state.linkDrag;
  const cands = state.linkCandidates;
  bar.classList.toggle('on', !!(link || drag || cands.length));
  const src = $('#link-src');
  if (drag) {
    // Идёт протяжка: показываем, что именно тянем — с узла это «весь шаг».
    src.textContent = drag.label
      || (drag.field ? `steps.${drag.step}.${drag.field}` : `шаг ${drag.step} (выход выберется сам)`);
    $('#link-hint').textContent = 'отпусти на другом шаге · Esc — отмена';
    return;
  }
  if (cands.length) {
    // Неоднозначный дроп: перечисляем пары и просим щёлкнуть одну. Ничего
    // вводить не нужно — только выбрать из списка.
    src.textContent = `${cands[0].label} → шаг ${cands[0].step}: выбери пару`;
    $('#link-hint').innerHTML = cands.map((c, i) =>
      `<button class="cand" data-i="${i}" title="связать ${esc(c.fromField)} → ${esc(c.field)}">${esc(c.fromField)} → ${esc(c.field)}</button>`
    ).join('');
    $('#link-hint').querySelectorAll('.cand').forEach(b => {
      b.addEventListener('click', () => {
        const c = cands[+b.dataset.i];
        applyLink(stepById(c.step), c, c.field);
      });
    });
    return;
  }
  if (link) $('#link-src').textContent = link.label || link.value;
}

// Маркеры-стрелки едут вместе с рёбрами: `svg.innerHTML = s` ниже сносит
// содержимое #edges целиком, поэтому <defs>, объявленные в HTML, исчезали бы
// после первого же рендера.
const EDGE_DEFS =
  '<defs>' +
  '<marker id="edge-arr" viewBox="0 0 9 9" refX="8" refY="4.5" markerWidth="6.5" markerHeight="6.5" orient="auto">' +
  '<path d="M0.5,0.5 L8.5,4.5 L0.5,8.5 z" fill="rgba(233,229,221,.55)"/></marker>' +
  '<marker id="edge-arr-hi" viewBox="0 0 9 9" refX="8" refY="4.5" markerWidth="6.5" markerHeight="6.5" orient="auto">' +
  '<path d="M0.5,0.5 L8.5,4.5 L0.5,8.5 z" fill="#9b8cb8"/></marker>' +
  '</defs>';

// edgesTouch — есть ли у шага хоть одно ребро: входящее (его bind ссылается на
// другой шаг или на input.*) или исходящее (на него ссылается чужой bind).
// Если рёбер нет, при перетаскивании пересчитывать нечего.
function edgesTouch(id) {
  const self = state.doc.steps.find(s => s.id === id);
  if (self && Object.values(self.bind || {}).some(v => v)) return true;
  const tag = `steps.${id}.`;
  return state.doc.steps.some(s =>
    Object.values(s.bind || {}).some(v => String(v || '').startsWith(tag)));
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
      // Точка входа — левый край шага, а не строка поля внутри него: узел
      // непрозрачный и нарисован поверх рёбер, поэтому конец линии (и стрелка)
      // в поле входа целиком уходили под корпус, и связь была видна только
      // обрывком в зазоре между шагами. По y остаётся центр поля входа — по
      // нему и видно, в какое именно поле пришла связь.
      const dstNode = dstEl.closest('.node');
      const nRect = dstNode ? dstNode.getBoundingClientRect() : dRect;
      const x2 = nRect.left - cRect.left - 6, y2 = dRect.top - cRect.top + dRect.height / 2;
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
      s += `<path class="edge${ok ? '' : ' hi'}" d="${path(x1, y1, x2, y2)}"`
        + ` marker-end="url(#${ok ? 'edge-arr' : 'edge-arr-hi'})"/>`;
    }
  }
  svg.innerHTML = EDGE_DEFS + s;
  // Резиновая линия протяжки живёт в этом же svg, а innerHTML его снёс.
  // Восстанавливаем, если протяжка ещё идёт.
  drawGhost();
}

// ── панель свойств ────────────────────────────────────────────────────────
const FORM_TYPES = ['', 'string', 'number', 'boolean', 'array', 'object'];
const FORM_FORMATS = ['', 'text', 'email', 'url', 'ip', 'file_ref'];
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

function loopBlock(st) {
  const on = !!st.loop;
  if (!on) {
    return `<div class="pblock"><label class="loop-toggle"><input type="checkbox" data-sloop-toggle/> цикл шага — повторять до условия</label></div>`;
  }
  return `
  <div class="pblock"><label class="loop-toggle"><input type="checkbox" data-sloop-toggle checked/> цикл шага — повторять до условия</label>
    <div class="hint">Шаг запускается повторно, пока условие истинно. Пустое условие использует steps.&lt;id&gt;.continue; пустой лимит — 100 итераций.</div>
  </div>
  <div class="prow">
    <div class="pblock" style="flex:2"><label>loop_condition — путь к boolean</label>
      <input data-sloop-condition value="${esc(st.loop_condition || '')}" placeholder="steps.${esc(st.id)}.continue" spellcheck="false"/></div>
    <div class="pblock" style="flex:1"><label>max_iterations (1–100; пусто = 100)</label>
      <input data-sloop-max type="number" min="1" max="100" value="${esc(st.max_iterations || '')}" placeholder="100" spellcheck="false"/></div>
  </div>`;
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
    html += `<div id="banner" style="display:block">Пайплайн содержит поля, которые редактор не умеет сохранять:
      <b>${state.unsupported.map(esc).join(', ')}</b>. Сохранение отключено, чтобы не потерять данные; открой файл в YAML-режиме.</div>`;
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
      ${(st.form || []).map((f, idx) => {
        const type = f.type || '', format = f.format || '';
        const typeOpts = FORM_TYPES.map(v => `<option value="${v}"${v === type ? ' selected' : ''}>${v || '— type —'}</option>`).join('') +
          (!FORM_TYPES.includes(type) ? `<option value="${esc(type)}" selected>${esc(type)} (вручную)</option>` : '');
        const formatOpts = FORM_FORMATS.map(v => `<option value="${v}"${v === format ? ' selected' : ''}>${v || '— format —'}</option>`).join('') +
          (!FORM_FORMATS.includes(format) ? `<option value="${esc(format)}" selected>${esc(format)} (вручную)</option>` : '');
        return `<div class="prow">
          <div class="pblock"><label>form[${idx + 1}] type — ${esc(f.field)}</label><select data-gtype="${idx}">${typeOpts}</select></div>
          <div class="pblock"><label>format</label><select data-gformat="${idx}">${formatOpts}</select></div>
        </div>`;
      }).join('')}
      <div class="pblock"><label>actions — по одному на строку</label>
        <textarea data-gactions rows="3" spellcheck="false">${esc((st.actions || []).join('\n'))}</textarea></div>
      <div class="pblock"><label>on_reject</label>
        <select data-gonreject>
          <option value="stop"${(st.on_reject || 'stop') === 'stop' ? ' selected' : ''}>stop (ран остановлен)</option>
          <option value=""${!st.on_reject ? ' selected' : ''}>continue (идём дальше)</option>
        </select></div>
      <div class="pblock"><label>approval</label>
        <select data-gapproval>
          <option value=""${!st.approval ? ' selected' : ''}>по умолчанию (any)</option>
          <option value="any"${st.approval === 'any' ? ' selected' : ''}>any</option>
          <option value="human"${st.approval === 'human' ? ' selected' : ''}>human — только человек</option>
        </select></div>` : '';
    html += `
      <h3 style="display:flex;align-items:center;justify-content:space-between">Шаг<button data-select-pipeline>‹ настройки пайплайна</button></h3>
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
      ${loopBlock(st)}
      ${retryBlock(st)}
      ${ins}
      ${gateBlock}
      <button class="danger" data-del>удалить шаг</button>
      <div class="hint">Двигай шаг за заголовок. Связи — в «bind» (список источников) — рёбра перерисуются сами.</div>`;
  } else {
    const rows = state.doc.input.map(inputEditor).join('');
    html += `
      <h3>Пайплайн (выбери шаг для правки шага)</h3>
      <div class="pblock"><label>имя</label><input data-pname value="${esc(state.doc.name)}" spellcheck="false"/></div>
      <div class="pblock"><label>gates — политика гейтов</label>
        <select data-pgates>
          <option value=""${!state.doc.gates ? ' selected' : ''}>по шагам (approval)</option>
          <option value="any"${state.doc.gates === 'any' ? ' selected' : ''}>any</option>
          <option value="human_only"${state.doc.gates === 'human_only' ? ' selected' : ''}>human_only — все гейты только человеком</option>
        </select></div>
      <div class="pblock"><label>входы input.* — значения или schema</label>${rows || '<div class="hint">нет входов</div>'}
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
      Обычные входы — фактические значения input.*; «схема» — декларация type, required, format, description и default.</div>`;
  }
  el.innerHTML = html;
  wireProps(st);
}

function wireProps(st) {
  const el = $('#props');
  const selectPipeline = el.querySelector('[data-select-pipeline]');
  if (selectPipeline) selectPipeline.onclick = () => { state.sel = null; renderAll(); };
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
  const loopToggle = el.querySelector('[data-sloop-toggle]');
  if (loopToggle) loopToggle.onchange = () => {
    pushUndo();
    if (loopToggle.checked) st.loop = st.loop || 'repeat';
    else { st.loop = ''; st.loop_condition = ''; st.max_iterations = 0; }
    renderAll();
  };
  const loopCondition = el.querySelector('[data-sloop-condition]');
  if (loopCondition) loopCondition.onchange = () => { pushUndo(); st.loop_condition = loopCondition.value.trim(); renderAll(); };
  const loopMax = el.querySelector('[data-sloop-max]');
  if (loopMax) loopMax.onchange = () => {
    const raw = loopMax.value.trim();
    const value = raw === '' ? 0 : Number(raw);
    if (!Number.isInteger(value) || (raw !== '' && value < 1) || value > 100) {
      alert('max_iterations должен быть от 1 до 100; пустое поле использует 100.');
      renderAll();
      return;
    }
    pushUndo(); st.max_iterations = value; renderAll();
  };
  const gform = el.querySelector('[data-gform]');
  if (gform) gform.onchange = () => {
    const previous = st.form || [];
    const lines = gform.value.split('\n').map(l => l.trim()).filter(Boolean);
    const used = new Set();
    pushUndo();
    st.form = lines.map((l, idx) => {
      const editable = l.startsWith('e:');
      const field = editable ? l.slice(2) : l;
      let oldIdx = previous.findIndex((f, i) => !used.has(i) && !!f.editable === editable && f.field === field);
      if (oldIdx < 0 && lines.length === previous.length && !used.has(idx)) oldIdx = idx;
      if (oldIdx >= 0) used.add(oldIdx);
      const old = previous[oldIdx] || {};
      return { field, editable, type: old.type || '', format: old.format || '' };
    });
    renderAll();
  };
  el.querySelectorAll('[data-gtype]').forEach(sel => {
    sel.onchange = () => {
      const field = (st.form || [])[+sel.dataset.gtype];
      if (!field) return;
      pushUndo();
      field.type = sel.value;
      renderAll();
    };
  });
  el.querySelectorAll('[data-gformat]').forEach(sel => {
    sel.onchange = () => {
      const field = (st.form || [])[+sel.dataset.gformat];
      if (!field) return;
      pushUndo();
      field.format = sel.value;
      renderAll();
    };
  });
  const gact = el.querySelector('[data-gactions]');
  if (gact) gact.onchange = () => {
    pushUndo();
    st.actions = gact.value.split('\n').map(l => l.trim()).filter(Boolean);
    renderAll();
  };
  const gonr = el.querySelector('[data-gonreject]');
  if (gonr) gonr.onchange = () => { pushUndo(); st.on_reject = gonr.value; renderAll(); };
  const gapproval = el.querySelector('[data-gapproval]');
  if (gapproval) gapproval.onchange = () => { pushUndo(); st.approval = gapproval.value; renderAll(); };
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
  const pgates = el.querySelector('[data-pgates]');
  if (pgates) pgates.onchange = () => { pushUndo(); state.doc.gates = pgates.value; renderAll(); };
  el.querySelectorAll('[data-iname]').forEach(control => control.onchange = () => {
    const idx = +control.dataset.iname;
    const input = state.doc.input[idx];
    const name = control.value.trim();
    if (!name || state.doc.input.some((other, i) => i !== idx && other.name === name)) {
      alert(!name ? 'Имя входа не может быть пустым.' : 'Такое имя входа уже есть.');
      renderAll();
      return;
    }
    pushUndo(); input.name = name; renderAll();
  });
  el.querySelectorAll('[data-ikind]').forEach(control => control.onchange = () => {
    const input = state.doc.input[+control.dataset.ikind];
    if (control.value === 'schema') {
      pushUndo();
      input.typed = true;
      input.type = input.type || (Array.isArray(input.default) ? 'array' : input.default === null ? 'string' : typeof input.default === 'object' ? 'object' : typeof input.default);
      if (!INPUT_TYPES.includes(input.type)) input.type = 'string';
      input.required = input.required ?? null;
      input.format = input.format || '';
      input.description = input.description || '';
      input.has_default = input.has_default !== false;
      if (!input.has_default || !valueMatchesInputType(input.default, input.type)) input.default = defaultForInputType(input.type);
      renderAll();
      return;
    }
    if (input.typed && (input.required !== undefined && input.required !== null || input.format || input.description || !input.has_default) &&
        !confirm('Переключить на обычное значение? Параметры type/required/format/description будут удалены.')) {
      renderAll();
      return;
    }
    pushUndo();
    input.typed = false;
    input.default = input.has_default ? input.default : '';
    input.has_default = true;
    delete input.type; delete input.required; delete input.format; delete input.description;
    renderAll();
  });
  el.querySelectorAll('[data-itype]').forEach(control => control.onchange = () => {
    const input = state.doc.input[+control.dataset.itype];
    pushUndo();
    input.type = control.value;
    if (!input.has_default || !valueMatchesInputType(input.default, input.type)) input.default = defaultForInputType(input.type);
    renderAll();
  });
  el.querySelectorAll('[data-irequired]').forEach(control => control.onchange = () => {
    const input = state.doc.input[+control.dataset.irequired];
    pushUndo();
    if (control.value === '') delete input.required;
    else input.required = control.value === 'true';
    renderAll();
  });
  el.querySelectorAll('[data-iformat]').forEach(control => control.onchange = () => {
    const input = state.doc.input[+control.dataset.iformat];
    pushUndo(); input.format = control.value.trim(); renderAll();
  });
  el.querySelectorAll('[data-idescription]').forEach(control => control.onchange = () => {
    const input = state.doc.input[+control.dataset.idescription];
    pushUndo(); input.description = control.value.trim(); renderAll();
  });
  el.querySelectorAll('[data-ihasdefault]').forEach(control => control.onchange = () => {
    const input = state.doc.input[+control.dataset.ihasdefault];
    pushUndo();
    input.has_default = control.checked;
    if (control.checked && input.default === undefined) input.default = defaultForInputType(input.type || 'string');
    renderAll();
  });
  el.querySelectorAll('[data-indef]').forEach(control => {
    control.onchange = () => {
      const input = state.doc.input[control.dataset.indef];
      try {
        const value = parseInputText(control.value, input.default, input.typed ? input.type : '');
        if (input.typed && !valueMatchesInputType(value, input.type)) throw new Error('значение не соответствует type: ' + input.type);
        pushUndo();
        input.default = value;
        renderAll();
      } catch (e) {
        alert('Некорректное значение input.' + input.name + ': ' + e.message);
        renderAll();
      }
    };
  });
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
  if (inadd) inadd.onclick = () => {
    let n = state.doc.input.length + 1;
    while (state.doc.input.some(input => input.name === 'field' + n)) n++;
    pushUndo();
    state.doc.input.push({ name: 'field' + n, default: '', typed: false, has_default: true });
    renderAll();
  };
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
  // Стрелки связей живут в отдельном SVG и не перерисуются сами: без этого
  // вызова после загрузки, связывания или любого ререндера связей не видно,
  // хотя данные в bind уже есть.
  renderEdges();
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
    // Esc снимает связывание раньше всего: оно вооружено мышью, иначе
    // отменить его можно только повторным щелчком по тому же выходу.
    if (e.key === 'Escape' && (state.link || state.linkDrag || state.linkCandidates.length)) { e.preventDefault(); cancelLink(); return; }
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
        if (state.link && state.link.step === st.id) state.link = null;
        if (state.linkDrag && state.linkDrag.step === st.id) state.linkDrag = null;
        state.linkCandidates = state.linkCandidates.filter(c => c.step !== st.id && c.fromStep !== st.id);
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
  // Рёбра цепляются к узлу живьём. Раньше при протяжке менялись только
  // left/top самого узла, а renderEdges() вызывался лишь на отпускании:
  // линия всё время перетаскивания стояла на старом месте и «отрывалась» от
  // шага. Рисуем не чаще кадра — mousemove приходит чаще, чем браузер рисует.
  const hasEdges = edgesTouch(st.id);
  let frame = 0;
  const scheduleEdges = () => {
    if (frame) return;
    frame = typeof requestAnimationFrame === 'function'
      ? requestAnimationFrame(() => { frame = 0; renderEdges(); })
      : setTimeout(() => { frame = 0; renderEdges(); }, 16);
  };
  const mm = ev => {
    const dx = ev.clientX - startX, dy = ev.clientY - startY;
    if (Math.abs(dx) + Math.abs(dy) > 3) moved = true;
    node.style.left = (ox + dx) + 'px';
    node.style.top = (oy + dy) + 'px';
    if (hasEdges) scheduleEdges();
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

function normalizePipelineFilename(raw) {
  let file = String(raw || '').trim();
  if (!file || file.includes('/') || file.includes('\\') || /[\x00-\x1f<>:"|?*]/.test(file) || file === '.' || file === '..') return '';
  file = file.replace(/\.ya?ml$/i, '');
  if (!file || file === '.' || file === '..') return '';
  return file + '.yaml';
}

function rebaseHistoryFilename(file) {
  for (const history of [state.undo, state.redo]) {
    for (let i = 0; i < history.length; i++) {
      const snapshot = JSON.parse(history[i]);
      if (Object.prototype.hasOwnProperty.call(snapshot, 'file')) {
        snapshot.file = file;
        history[i] = JSON.stringify(snapshot);
      }
    }
  }
}

// save() возвращает true только когда файл действительно записан на диск.
// Кнопке запуска это нужно, чтобы не стартовать ран по несохранённому YAML:
// иначе человек правит связи, жмёт «Запустить» и получает старый результат.
async function save() {
  const file = normalizePipelineFilename($('#file-name').value || state.doc.file);
  const status = $('#save-status');
  if (!file) {
    status.textContent = 'некорректное имя файла'; status.className = 'badge err';
    $('#file-name').focus();
    return false;
  }
  $('#file-name').value = file;
  try {
    await doValidate();
  } catch {}
  if (state.validating) return;
  if (state.unsupported.length) { alert('Нельзя сохранить: редактор не управляет полями: ' + state.unsupported.join(', ')); return false; }
  if (state.valErrs.length) { alert('Сначала исправь ошибки:\n' + state.valErrs.join('\n')); return false; }
  const st = $('#save-status');
  try {
    await apiRaw('/api/pipelines/' + encodeURIComponent(file), { method: 'PUT', body: state.yaml });
    state.doc.file = file;
    rebaseHistoryFilename(file);
    st.textContent = 'сохранено: ' + file; st.className = 'badge ok';
    await loadFileList();
    return true;
  } catch (e) {
    st.textContent = 'ошибка'; st.className = 'badge err';
    alert('Сохранение: ' + e.message);
    return false;
  }
}

// runFromEditor — сохранить и запустить, затем уйти в консоль на этот ран.
//
// Сначала save(): запускать несохранённый YAML — это получить результат старой
// версии файла и потом гадать, почему правки не подействовали. Гейт оставляем
// человеку (yes: false): редактор — место подготовки, решение всё равно за
// человеком; автоматический прогон живёт в консоли.
async function runFromEditor() {
  const status = $('#save-status');
  const btn = $('#btn-run');
  if (!(await save())) return;
  const file = state.doc.file;
  btn.disabled = true;
  status.textContent = 'запуск…'; status.className = 'badge run';
  try {
    const res = await api('/api/run', { method: 'POST', body: JSON.stringify({ file, yes: false }) });
    if (res && res.run) { location.href = '/?run=' + encodeURIComponent(res.run); return; }
    status.textContent = 'ран без id'; status.className = 'badge err';
  } catch (e) {
    status.textContent = 'запуск не удался'; status.className = 'badge err';
    alert('Запуск: ' + e.message);
  } finally {
    btn.disabled = false;
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
      input: (doc.input || []).map(i => ({
        name: i.name,
        default: i.has_default ? i.default : undefined,
        typed: !!i.typed,
        type: i.type || '',
        required: i.required,
        format: i.format || '',
        description: i.description || '',
        has_default: !!i.has_default,
      })),
      steps: (doc.steps || []).map(s => ({
        id: s.id, plugin: s.plugin,
        pos: Array.isArray(s.pos) && s.pos.length === 2 ? [s.pos[0], s.pos[1]] : [20 * (1 + Math.random() * 8), 20 * (1 + Math.random() * 6)],
        on_error: s.on_error || 'stop', timeout: s.timeout || '',
        bind: s.bind || {},
        form: (s.form || []).map(f => ({ field: f.field, editable: !!f.editable, type: f.type || '', format: f.format || '' })),
        actions: s.actions || [], on_reject: s.on_reject || '', approval: s.approval || '',
        when: s.when || null,
        foreach: s.foreach || '', foreach_item: s.foreach_item || '',
        after_foreach: !!s.after_foreach, parallel_group: s.parallel_group || '',
        loop: s.loop || '', loop_condition: s.loop_condition || '',
        max_iterations: Number(s.max_iterations) || 0,
        retry: s.retry || null,
      })),
      foreach: doc.foreach || '', foreach_item: doc.foreach_item || '',
      item_type: doc.item_type || '', item_format: doc.item_format || '',
      secrets: doc.secrets || [], network: doc.network || '', gates: doc.gates || '',
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
  // H4: без сессии /api/* закрыт, поэтому сначала вход, потом редактор
  if (!await wedraSession()) return;
  try { state.plugins = await api('/api/plugins'); } catch (e) { console.error(e); }
  renderPalette();
  initCanvas();
  await loadFileList();
  $('#file-name').value = state.doc.file;
  $('#file-name').onkeydown = e => { if (e.key === 'Enter') { e.preventDefault(); save(); } };
  $('#file-open').onchange = e => { if (e.target.value) openFile(e.target.value); };
  $('#btn-save').onclick = save;
  $('#btn-run').onclick = runFromEditor;
  $('#link-cancel').onclick = cancelLink;
  $('#btn-yaml').onclick = showYaml;
  $('#close-yaml').onclick = () => { $('#yaml-view').style.display = 'none'; };
  $('#btn-undo').onclick = undo;
  $('#btn-redo').onclick = redo;
  // v0.38: режим артефактов передаёт имя собранного пайплайна через
  // sessionStorage — параметра ?open= у редактора нет, а заводить ради
  // одного перехода второй способ открытия незачем.
  let openNext = '';
  try { openNext = sessionStorage.getItem('wedra.open') || ''; } catch (e) { /* приватный режим */ }
  if (openNext) {
    try { sessionStorage.removeItem('wedra.open'); } catch (e) { /* пусто */ }
    // loadFileList() не возвращает признака успеха, а openFile() сам
    // разбирается с ошибкой. Поэтому просто открываем.
    await openFile(openNext);
    return;
  }
  renderAll();
}
init();
