// v0.22 — консоль WEDRA: раны, live-терминал, контекст, DAG.
// Без внешних зависимостей (офлайн: всё встроено).
const $ = s => document.querySelector(s);
const esc = s => String(s ?? '').replace(/[&<>"'`]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;','`':'&#96;'}[c]));

// errText — текст ошибки из журнала. В journal.jsonl error приходит ОБЪЕКТОМ
// {code, message, retryable}, и печать его напрямую давала «[object object]».
// Показываем сообщение, а при его отсутствии — код.
function errText(e) {
  if (e == null) return '';
  if (typeof e === 'string') return e;
  if (typeof e === 'object') return e.message || e.err_msg || e.code || JSON.stringify(e);
  return String(e);
}

let state = {
  tab: 'menu',
  runs: [],
  currentRun: null,
  journal: { events: [], since: 0, total: 0 },
  autoScroll: true,
  pipelines: [],
  currentPipe: null,
  presets: [],
  timers: {},
};

async function api(path, opts = {}) {
  const r = await fetch(path, opts);
  const ct = r.headers.get('content-type') || '';
  const body = ct.includes('json') ? await r.json() : await r.text();
  if (!r.ok) {
    // H4: 401 — нет сессии человека. Cookie ставится обменом одноразового кода
    // из терминала wedra; показываем поле ввода, а не текст в консоли.
    if (r.status === 401) { sessionOverlay(); throw new Error('нет сессии: нужен одноразовый код из терминала wedra'); }
    // 400 {issues} — валидация до запуска: показываем коды, а не сырой JSON
    if (body && Array.isArray(body.issues)) {
      const errs = body.issues.filter(i => i.severity === 'error');
      throw new Error(errs.map(i => i.code + ': ' + i.message).join('\n') || JSON.stringify(body));
    }
    throw new Error(typeof body === 'string' ? body : (body.error || JSON.stringify(body)));
  }
  return body;
}

// ── H4: сессия человека ──────────────────────────────────────────────────
// Всё под /api/* (кроме /api/health) требует cookie. Cookie выдаётся обменом
// ОДНОРАЗОВОГО кода, напечатанного в терминале, где запущен wedra. Пока cookie
// нет, страница показывает поле ввода кода вместо пустых списков: тот же путь,
// что и у `wedra gui --open` (там код едет в ссылке ?c= и ставит cookie сразу).
function sessionOverlay() {
  if (document.getElementById('wedra-session-box')) return;
  const box = document.createElement('div');
  box.id = 'wedra-session-box';
  // стили — в index.html (#wedra-session-box): разметка не тащит цвета в JS
  box.innerHTML =
    '<div class="sbox">' +
    '<h2>Вход в WEDRA</h2>' +
    '<p>Код входа одноразовый и напечатан в терминале, где запущен wedra. Он нужен один раз: ' +
    'обменяется на cookie, и страница перезагрузится сама.</p>' +
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

// ── header ───────────────────────────────────────────────────────────────
async function init() {
  // H4: без сессии /api/* закрыт, поэтому сначала вход, потом интерфейс
  if (!await wedraSession()) return;
  try {
    const h = await api('/api/health');
    $('#ver').textContent = 'v' + h.version;
    state.version = h.version;
  } catch { $('#ver').textContent = 'оффлайн'; state.version = ''; }
  $('#tab-menu').onclick = () => setTab('menu');
  $('#tab-runs').onclick = () => setTab('runs');
  $('#tab-pipelines').onclick = () => setTab('pipelines');
  $('#tab-plugins').onclick = () => { setTab('plugins'); loadPluginsTab(); };
  $('#run-btn').onclick = startRun;
  // v0.35: меню считает пайплайны («из N запустить»), поэтому список должен
  // прийти до отрисовки, а не гонкой с ним.
  await loadPipelines();
  tickRuns();
  state.timers.runs = setInterval(tickRuns, 2500);
  // v0.35: по умолчанию — главное меню, а не список ранов: первый вопрос
  // новичка «с чего начать», и ответ на него должен быть на экране.
  const runParam = new URLSearchParams(location.search).get('run');
  // Разделы консоли — вкладки одной страницы, поэтому из редактора и режима
  // артефактов на них приходится приходить адресом: /#runs открывает «Раны».
  const hash = (location.hash || '').replace('#', '');
  if (runParam) { setTab('runs'); openRunDetail(runParam, true); }
  else if (['runs', 'pipelines', 'plugins', 'menu'].includes(hash)) {
    setTab(hash);
    if (hash === 'menu') renderMenu();
    else if (hash === 'plugins') loadPluginsTab();
  } else { setTab('menu'); renderMenu(); }
}

function setTab(t) {
  state.tab = t;
  $('#tab-menu').classList.toggle('active', t === 'menu');
  $('#tab-runs').classList.toggle('active', t === 'runs');
  $('#tab-pipelines').classList.toggle('active', t === 'pipelines');
  $('#tab-plugins').classList.toggle('active', t === 'plugins');
  $('#menu').style.display = t === 'menu' ? '' : 'none';
  $('#runs-aside').style.display = t === 'runs' ? '' : 'none';
  $('#pip-aside').style.display = t === 'pipelines' ? '' : 'none';
  $('#plug-aside').style.display = t === 'plugins' ? '' : 'none';
  $('#detail').style.display = t === 'runs' ? '' : 'none';
  $('#pdetail').style.display = t === 'pipelines' ? '' : 'none';
  $('#plugdetail').style.display = t === 'plugins' ? '' : 'none';
}

// ── главное меню (v0.35) ──────────────────────────────────────────────────
// Собирается из уже существующих /api/* — своего состояния у меню нет.
// Порядок карточек — по частоте вопросов: продолжить, начать, готовое,
// собрать, каталог.
async function renderMenu() {
  const [presets, runs, plugins] = await Promise.all([
    api('/api/presets').catch(() => ({available: false, reason: 'нет связи с ядром', presets: []})),
    api('/api/runs').catch(() => []),
    api('/api/plugins').catch(() => []),
  ]);
  state.presets = presets.presets || [];
  const lastRun = runs.find(r => r.status === 'running' || r.status === 'waiting')
    || runs[0];

  const cards = [];
  cards.push({
    ic: '▶', tone: 'acc',
    k: 'Продолжить',
    d: lastRun
      ? `Ран «${lastRun.pipeline || lastRun.id}» — ${lastRun.status || '?'}. Открыть таймлайн, контекст и журнал.`
      : 'Ранов пока нет. Запусти первый — журнал каждого шага будет здесь.',
    go: lastRun ? 'открыть ран →' : 'начать новый ↓',
    act: () => { setTab('runs'); if (lastRun) openRunDetail(lastRun.id, true); },
  });
  cards.push({
    ic: '+', tone: 'ok',
    k: 'Начать новый запуск',
    // count берём только если список действительно загружен: иначе на экране
    // появилось бы «из 0 пайплайнов» — правдоподобная, но ложная цифра.
    d: (state.pipelines.length
      ? `Выбрать пайплайн из ${state.pipelines.length} и запустить. `
      : 'Выбрать пайплайн и запустить. ')
      + 'Гейт можно оставить человеку или принять автоматически.',
    go: 'к списку пайплайнов →',
    act: () => setTab('pipelines'),
  });
  cards.push({
    ic: '▤', tone: 'par',
    k: 'Готовые сценарии',
    d: presets.available
      ? `${state.presets.length} пресетов из реестра с описанием. Открыть можно готовый или взять как основу.`
      : `Реестр недоступен (${presets.reason || 'причина неизвестна'}). Пресеты показывать не из чего.`,
    go: 'список ниже ↓',
    act: () => {},
  });
  cards.push({
    ic: '✎', tone: 'skip',
    k: 'Собрать пайплайн',
    d: 'Визуальный редактор: перетаскивание шагов, связи, входы, циклы и гейты. Round-trip через ядро, YAML не теряется.',
    go: 'открыть редактор →',
    act: () => { location.href = '/editor/'; },
  });

  const presetCards = state.presets.map(p => `
    <div class="pcard">
      <div class="pname" data-preset-open="${esc(p.file)}">${esc(p.file.replace(/\.ya?ml$/i, ''))}</div>
      <div class="pdesc">${esc(p.description || 'без описания')}</div>
      <div class="pacts">
        <button class="mini" data-preset-open="${esc(p.file)}">открыть</button>
        <button class="mini go" data-preset-run="${esc(p.file)}"
          ${p.installed ? '' : 'disabled title="файла нет в examples/"'}>запустить</button>
      </div>
    </div>`).join('');

  const trustedN = (plugins || []).filter(p => p.trusted).length;
  const stat = (v, l) => `<span class="stat"><b>${v}</b>${l}</span>`;
  const heroStats = stat(esc(state.version ? 'v' + state.version : '—'), 'ядро')
    + stat(`${trustedN}/${plugins.length}`, 'доверенных')
    + stat(`${state.presets.length}`, 'пресетов')
    + stat(`${(runs || []).length}`, 'ранов');

  $('#menu').innerHTML = `
    <div class="hero">
      <div class="htext">
        <p class="lead">WEDRA · консоль контрактных цепочек</p>
        <p class="sub">Плагины — отдельные процессы, данные — между шагами, человек — в гейте.
          Выбери готовое, начни с нуля или открой редактор.</p>
        <div class="hstats">${heroStats}</div>
      </div>
    </div>
    <div class="cards">${cards.map((c, i) => `
      <button class="card-btn" data-card="${i}">
        <span class="ic ${esc(c.tone || '')}">${esc(c.ic || '·')}</span>
        <span class="cb">
          <div class="k">${esc(c.k)}</div>
          <div class="d">${esc(c.d)}</div>
          <div class="go">${esc(c.go)}</div>
        </span>
      </button>`).join('')}</div>

    <h4>Готовые сценарии${presets.available ? ` · реестр, ${plugins.length} плагинов` : ''}</h4>
    ${presets.available
      ? (presetCards ? `<div class="pcards">${presetCards}</div>` : '<div class="empty">В реестре нет пресетов</div>')
      : '<div class="empty">Реестр недоступен — поставь WEDRA из исходников, чтобы увидеть пресеты</div>'}

    <button class="sectbtn" id="goto-plugins">
      <span>Каталог плагинов</span><span class="cnt">${plugins.length}</span>
      <span class="tsub">${trustedN} доверенных · поиск, манифест, установка зависимостей</span><span class="chev">→</span>
    </button>
    <div class="note">Плагины — исполняемый код. Перед установкой проверяй источник и объявленные
        в манифесте права: <span style="font-family:var(--mono)">network</span>,
        <span style="font-family:var(--mono)">filesystem</span>,
        <span style="font-family:var(--mono)">secrets</span>.</div>`;

  cards.forEach((c, i) => { const el = $(`[data-card="${i}"]`); if (el) el.onclick = c.act; });
  const gotoPl = $('#goto-plugins');
  if (gotoPl) gotoPl.onclick = () => { setTab('plugins'); loadPluginsTab(); };
  $('#menu').querySelectorAll('[data-preset-run]').forEach(b => {
    b.onclick = () => runPreset(b.dataset.presetRun);
  });
  $('#menu').querySelectorAll('[data-preset-open]').forEach(el => {
    el.onclick = () => { setTab('pipelines'); openPipeline(el.dataset.presetOpen); };
  });
}

// Запуск пресета из меню: тот же путь, что у кнопки «Запустить» на вкладке
// ранов, только без прыжка по вкладкам — human_gate всё равно нужен на экране.
async function runPreset(file) {
  setTab('runs');
  const sel = $('#run-select');
  if (sel && ![...sel.options].some(o => o.value === file)) {
    // файла нет рядом — честно говорим, вместо того чтобы молча ничего не
    // сделать: пресет из реестра ставится через `wedra pipeline install`.
    $('#run-status').textContent = `файла ${file} нет в каталоге пайплайнов — поставь: wedra pipeline install <имя>`;
    return;
  }
  if (sel) sel.value = file;
  $('#run-gate').checked = true;
  await startRun();
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
    return `<div class="run-item ${state.currentRun === r.id ? 'active' : ''}" data-run-id="${esc(r.id)}">
      <div class="top"><span class="pipeline">${esc(r.pipeline || '?')}</span><span class="badge ${st}">${esc(label)}</span></div>
      <div class="meta">${esc(r.id)} · ${esc(r.steps)} ш. · ${esc(t)}</div>
    </div>`;
  }).join('');
  el.querySelectorAll('[data-run-id]').forEach(item => item.addEventListener('click', () => openRunDetail(item.dataset.runId, true)));
  $('#runs-count').textContent = `(${state.runs.length})`;
}

// ── запуск из браузера ───────────────────────────────────────────────────
async function loadPipelines() {
  try {
    state.pipelines = await api('/api/pipelines');
    const sel = $('#run-select');
    sel.innerHTML = state.pipelines
      .filter(p => !p.error)
      .map(p => `<option value="${esc(p.file)}">${esc(p.name || p.file)} — ${esc(p.steps)} ш.${p.foreach ? ' · foreach' : ''}</option>`)
      .join('');
    renderPipList();
  } catch (e) { console.error(e); }
}

function renderPipList() {
  const el = $('#pip-list');
  el.innerHTML = state.pipelines.map(p =>
    `<div class="pitem ${state.currentPipe === p.file ? 'active' : ''}" data-pipeline-file="${esc(p.file)}">
      ${esc(p.name || p.file)} <small>${esc(p.steps)} ш.${p.foreach ? ' · foreach ' + esc(p.foreach) : ''}${p.error ? ' · error' : ''}</small>
    </div>`).join('');
  el.querySelectorAll('[data-pipeline-file]').forEach(item => item.addEventListener('click', () => openPipeline(item.dataset.pipelineFile)));
  $('#pip-count').textContent = `(${state.pipelines.length})`;
}

// ── вкладка «Плагины»: полный каталог, кликабельная деталка, установка зависимостей ──
async function loadPluginsTab() {
  try {
    state.pluginsFull = await api('/api/plugins');
  } catch (e) {
    $('#plug-tab-list').innerHTML = '<div class="empty">' + esc(e.message) + '</div>';
    return;
  }
  renderPluginsTabList('');
  const s = $('#plug-tab-search');
  if (s) s.oninput = () => renderPluginsTabList(s.value);
  $('#plug-tab-count').textContent = `(${state.pluginsFull.length})`;
}

function renderPluginsTabList(q) {
  const needle = (q || '').trim().toLowerCase();
  const hit = !needle ? state.pluginsFull : state.pluginsFull.filter(p =>
    (p.id || '').toLowerCase().includes(needle) ||
    (p.description || '').toLowerCase().includes(needle));
  $('#plug-tab-list').innerHTML = hit.map(p => {
    const dot = p.trusted ? '<span class="tdot ok"></span>' : '<span class="tdot"></span>';
    return `<div class="pitem" data-plugin-id="${esc(p.id)}">${dot} ${esc(p.id)}`
      + ` <small>${esc(p.version || '')}${p.trusted ? '' : ' · вне allow-list'}</small></div>`;
  }).join('') || '<div class="empty">Ничего не найдено</div>';
  $('#plug-tab-list').querySelectorAll('[data-plugin-id]').forEach(el =>
    el.addEventListener('click', () => openPluginDetail(el.dataset.pluginId)));
}

async function openPluginDetail(id) {
  document.querySelectorAll('#plug-tab-list .pitem').forEach(el =>
    el.classList.toggle('active', el.dataset.pluginId === id));
  $('#plugdetail').innerHTML = '<div class="empty">загрузка…</div>';
  let d;
  try {
    d = await api('/api/plugins/' + encodeURIComponent(id));
  } catch (e) {
    $('#plugdetail').innerHTML = '<div class="empty">' + esc(e.message) + '</div>';
    return;
  }
  const reqs = (((d.runtime || {}).requires) || []);
  const net = ((d.permissions || {}).network || []);
  const trustLine = d.trusted
    ? '<span class="badge ok">доверен</span>'
    : '<span class="badge err">вне allow-list</span>'
      + (d.blocked_reason ? `<div class="hint">${esc(d.blocked_reason)}</div>` : '');
  const depsBlock = reqs.length
    ? `<div class="dbox"><div class="dlist">${reqs.map(esc).join('<br>')}</div>
       <div class="dsub">точные пины из манифеста — ставится только это</div>
       <button class="mini" id="deps-install">установить</button></div>
       <div class="hint" id="deps-status"></div>
       <pre class="jnl" id="deps-out" style="display:none;max-height:220px"></pre>`
    : '<div class="hint">Зависимостей не объявлено (runtime.requires пуст) — ставить нечего.</div>';
  $('#plugdetail').innerHTML = `
    <div class="dhead"><h2 class="mono">${esc(d.id)}</h2>${trustLine}</div>
    <div class="sub mono dim">${esc(d.version || '')} · ${esc(d.author || '')}</div>
    <p style="line-height:1.6">${esc(d.description || 'без описания')}</p>
    <h4>Права</h4>
    <div class="hint">сеть: ${net.length ? esc(net.map(n => n.any_host ? 'any_host' : (n.host + ':' + n.port)).join(', ')) : '—'};
      файлы: ${esc((d.permissions || {}).filesystem || '—')};
      секреты: ${esc(((d.permissions || {}).secrets || []).join(', ') || '—')}</div>
    <h4 style="margin-top:16px">Зависимости Python</h4>
    ${depsBlock}`;
  const btn = $('#deps-install');
  if (btn) btn.onclick = () => installPluginDeps(d.id, reqs, btn);
}

async function installPluginDeps(id, reqs, btn) {
  const cmd = `python -m pip install ${reqs.join(' ')}`;
  if (!confirm(`Установить зависимости плагина ${id}?\n\n${cmd}\n\nСтавятся только точные пины из манифеста.`)) return;
  btn.disabled = true;
  btn.textContent = 'ставится…';
  const st = $('#deps-status'), out = $('#deps-out');
  try {
    const r = await api('/api/plugins-install-deps?id=' + encodeURIComponent(id), {method: 'POST'});
    if (st) st.textContent = r.ok ? 'Готово.' : ('Ошибка: ' + (r.error || ''));
    if (out && r.output) { out.style.display = ''; out.textContent = r.output.slice(-3000); }
  } catch (e) {
    if (st) st.textContent = 'Ошибка: ' + e.message;
  } finally {
    btn.disabled = false;
    btn.textContent = 'установить';
  }
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
    const res = await api('/api/run', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({file, yes}) });
    if (res && res.run) { state.timers.pendingNew = false; setTimeout(() => openRunDetail(res.run, true), 400); }
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
  const st = d.status === 'ok' ? 'ok' : d.status === 'running' ? 'run' : d.status === 'cancelled' ? 'skip' : 'err';
  const label = d.status === 'running' ? 'идёт…' : d.status === 'cancelled' ? 'отменён' : d.status;
  const cancelBtn = d.status === 'running' ? `<button class="btn" id="cancel-btn" data-action="cancel" data-run-id="${esc(id)}">■ отменить</button>` : '';
  const ctx = d.context || {};
  const steps = (ctx.steps && Object.keys(ctx.steps).length) || 0;
  const inp = ctx.input ? Object.keys(ctx.input).length : 0;
  // деталка отдаёт окно журнала (хвост, если журнал больше потолка ответа)
  const total = (typeof d.total === 'number') ? d.total : d.events.length;
  const trunc = d.truncated
    ? `<span class="sub">журнал обрезан: показаны последние ${d.events.length} из ${total} событий</span>`
    : '';

  $('#detail').innerHTML = `
    <div id="gate-card" style="display:none"></div>
    <div class="dhead">
      <button class="btn" data-action="close">←</button>
      <h2>${esc(d.pipeline || '?')}</h2>
      <span class="badge ${st}">${esc(label)}</span>
      ${cancelBtn}
      <span class="sub">${esc(id)} · контекст: input ${inp} полей, steps ${steps}</span>
      ${trunc}
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
            <button id="rt-ctx" class="active" data-action="tab" data-tab="ctx">контекст</button>
            <button id="rt-jnl" data-action="tab" data-tab="jnl">журнал <span id="jnl-n"></span></button>
          </span>
        </h3>
        <div class="ctx" id="rt-ctx-body">${renderCtx(ctx)}</div>
        <div class="jnl" id="rt-jnl-body" style="display:none">
          <div style="margin-bottom:8px"><span class="autochip on" id="autochip" data-action="auto">авто-скролл</span>
          <span style="font-size:10px;color:var(--dim);margin-left:8px">live: обновление каждые 2 c</span></div>
          <div id="jnl-lines"></div>
        </div>
      </div>
    </div>`;
  wireRunDetail(id);
  // журнал: окно событий + polling хвоста по курсору next/total
  state.journal = { events: d.events, since: total, total: total, truncated: !!d.truncated };
  renderJournal(d.events);
  updateGateCard(id);
  clearInterval(state.timers.journal);
  state.timers.journal = setInterval(async () => {
    if (state.currentRun !== id || state.tab !== 'runs') { clearInterval(state.timers.journal); return; }
    try {
      const tail = await api(`/api/runs/${id}/journal?since=${state.journal.since}`);
      if (tail.events.length) {
        // курсор: next — точное место конца окна (truncated=true), иначе total
        state.journal.since = (typeof tail.next === 'number') ? tail.next : tail.total;
        state.journal.events = state.journal.events.concat(tail.events);
        const tl = $('#tl'); if (tl) tl.innerHTML = renderTimeline(state.journal.events);
        renderJournal(tail.events, true);
        updateGateCard(id);
        const d2 = state.journal.events[state.journal.events.length - 1];
        if (d2 && (d2.type === 'run_end' || d2.type === 'run_failed' || d2.type === 'run_cancelled')) {
          const cb = $('#cancel-btn'); if (cb) cb.remove();
          const dd = await api('/api/runs/' + id);
          state.detailStatus = dd.status;
          // Деталка, открытая во время рана, показывает пустой контекст и
          // старый бейдж: контекст грузился один раз при открытии. При
          // завершении подтягиваем и то, и другое из свежей деталки.
          refreshRunHead(dd);
          const cb2 = $('#rt-ctx-body');
          if (cb2) cb2.innerHTML = renderCtx(dd.context || {});
          tickRuns();
        }
      }
      const jn = $('#jnl-n'); if (jn) jn.textContent = `(${state.journal.total})`;
    } catch {}
  }, 2000);
}

function wireRunDetail(id) {
  const root = $('#detail');
  const close = root.querySelector('[data-action="close"]');
  if (close) close.addEventListener('click', () => closeDetail());
  const cancel = root.querySelector('[data-action="cancel"]');
  if (cancel) cancel.addEventListener('click', () => cancelRun(id));
  root.querySelectorAll('[data-action="tab"]').forEach(button => button.addEventListener('click', () => showRTab(button.dataset.tab)));
  const auto = root.querySelector('[data-action="auto"]');
  if (auto) auto.addEventListener('click', () => toggleAuto());
}

// v0.9: отмена рана (POST /cancel; требует сессию человека)
window.cancelRun = async (id) => {
  const b = $('#cancel-btn'); if (b) { b.disabled = true; b.textContent = 'отмена…'; }
  try { await api('/api/runs/' + id + '/cancel', { method: 'POST' }); }
  catch (e) { if (b) { b.disabled = false; b.textContent = '■ отменить'; } alert(e.message); }
};

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
      html += `<div class="item"><div class="hd">элемент ${esc(String(e.item_index))}${e.item != null ? ' · ' + esc(JSON.stringify(e.item)) : ''}</div>`;
      continue;
    }
    if (type === 'item_end' || type === 'item_aborted') {
      if (inItem >= 0) { html += '</div>'; inItem = -1; }
      const bad = type === 'item_aborted';
      html += ev(t(e), bad ? 'err' : 'ok', `${bad ? 'элемент прерван' : 'элемент завершён'}`, esc(String(e.status || '')));
      continue;
    }
    if (type === 'parallel_start') { inPar = true; html += `<div class="par"><div class="hd">‖ параллельно: ${esc((e.steps || []).join(', '))}</div>`; continue; }
    if (type === 'parallel_end') { if (inPar) html += '</div>'; inPar = false; html += ev(t(e), 'par', `‖ группа завершена за ${esc(String(e.duration_ms ?? '?'))} мс`, e.statuses ? esc(JSON.stringify(e.statuses)) : ''); continue; }
    if (type === 'foreach_item_start') { html += ev(t(e), 'dim', `⤷ ${esc(e.step)} · элемент ${esc(String(e.item_index))}`, ''); continue; }
    if (type === 'foreach_item_end') { html += ev(t(e), 'dim', `⤷ ${esc(e.step)} · элемент ${esc(String(e.item_index))} → ${esc(e.status)}`, e.duration_ms != null ? esc(String(e.duration_ms)) + ' мс' : ''); continue; }
    if (type === 'step_start') { html += ev(t(e), 'run', `▶ ${esc(e.step)}`, e.attempt > 1 ? `повтор ${esc(String(e.attempt))}` : ''); continue; }
    if (type === 'step_end') {
      const cls = e.status === 'ok' ? 'ok' : 'err';
      html += ev(t(e), cls, `${e.status === 'ok' ? '✓' : '✗'} ${esc(e.step)}`, `${esc(String(e.duration_ms ?? '?'))} мс · exit ${esc(String(e.exit_code))}${e.error ? ' · ' + esc(errText(e.error)) : ''}`);
      continue;
    }
    if (type === 'step_failed') { html += ev(t(e), 'err', `✗ ${esc(e.step)}: ${esc(errText(e.error) || 'ошибка')}`, ''); continue; }
    if (type === 'step_skipped') { html += ev(t(e), 'skip', `↷ ${esc(e.step)} пропущен`, e.reason ? `reason: ${esc(e.reason)}${e.condition ? ' · ' + esc(e.condition) : ''}` : ''); continue; }
    if (type === 'gate_wait') { html += ev(t(e), 'run', `👤 гейт ${esc(e.step)}: ожидает решение (в браузере)`, esc((e.actions || []).map(a => String(a)).join('/'))); continue; }
    if (type === 'gate_retry') { html += ev(t(e), 'skip', `⚠ гейт ${esc(e.step)}: ${esc(e.reason || 'переспрос')}`, 'попытка ' + esc(String(e.attempt || '?'))); continue; }
    if (type === 'gate_decision') { html += ev(t(e), e.action === 'accept' ? 'ok' : 'skip', `👤 гейт ${esc(e.step)}: ${esc(e.action)}${e.auto ? ' (авто --yes)' : ''}${e.source ? ' · ' + esc(e.source) : ''}`, e.materialized ? esc(JSON.stringify(e.materialized)) : ''); continue; }
    if (type === 'run_start') { html += ev(t(e), 'dim', `ран: ${esc(e.pipeline || '?')}${e.foreach ? ' · foreach ' + esc(e.foreach) : ''}`, ''); continue; }
    if (type === 'run_resumed') { html += ev(t(e), 'par', `ран возобновлён (resume)`, ''); continue; }
    if (type === 'run_end') { html += ev(t(e), (e.aborted || 0) ? 'err' : 'ok', `■ ран завершён: ok=${esc(String(e.ok || 0))} aborted=${esc(String(e.aborted || 0))}`, ''); continue; }
    if (type === 'run_failed') { html += ev(t(e), 'err', `■ ран упал${e.code ? ' [' + esc(e.code) + ']' : ''}: ${esc(errText(e.error))}`, ''); continue; }
    if (type === 'run_cancelled') { html += ev(t(e), 'skip', '■ ран отменён (resume — продолжить с места остановки)', ''); continue; }
    if (type === 'post_phase_start') { html += ev(t(e), 'dim', 'post-фаза (после foreach)…', ''); continue; }
    if (type === 'post_phase_end') { html += ev(t(e), 'dim', 'post-фаза завершена', ''); continue; }
    if (type === 'foreach_item_failed') { html += ev(t(e), 'err', `⤷ ${esc(e.step)} · элемент ${esc(String(e.item_index))}: ${esc(errText(e.error) || 'ошибка')}`, ''); continue; }
    if (type === 'file_ref_warning' || type === 'contract_warning') { html += ev(t(e), 'dim', '· ' + esc(e.message || e.warning || JSON.stringify(e)), ''); continue; }
  }
  if (inItem >= 0) html += '</div>';
  if (inPar) html += '</div>';
  return html || '<div class="empty">пусто</div>';
}

function ev(time, cls, msg, extra) {
  return `<div class="ev ${cls}"><span class="t">${esc(time)}</span><span class="m">${msg}</span>${extra ? `<span class="x">${extra}</span>` : ''}</div>`;
}

// refreshRunHead — обновить бейдж статуса и счётчики контекста в шапке деталки
// после завершения рана. Без этого деталка, открытая во время выполнения,
// навсегда остаётся с бейджем «идёт…» и «input 0 полей».
function refreshRunHead(d) {
  const head = document.querySelector('#detail .dhead');
  if (!head) return;
  const st = d.status === 'ok' ? 'ok' : d.status === 'running' ? 'run' : d.status === 'cancelled' ? 'skip' : 'err';
  const label = d.status === 'running' ? 'идёт…' : d.status === 'cancelled' ? 'отменён' : d.status;
  const badge = head.querySelector('.badge');
  if (badge) { badge.className = 'badge ' + st; badge.textContent = label; }
  const ctx = d.context || {};
  const steps = (ctx.steps && Object.keys(ctx.steps).length) || 0;
  const inp = ctx.input ? Object.keys(ctx.input).length : 0;
  const sub = head.querySelector('.sub');
  if (sub) {
    const id = (sub.textContent || '').split(' · ')[0];
    sub.textContent = `${id} · контекст: input ${inp} полей, steps ${steps}`;
  }
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
      <span class="sub">${esc(p.steps)} ш.${p.foreach ? ' · foreach ' + esc(p.foreach) : ''}</span>
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
    `<defs><marker id="arr" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L8,4 L0,8 z" style="fill:rgba(233,229,221,.45)"/></marker></defs>`;
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
  const actions = Array.isArray(gw.actions) ? gw.actions : ['accept', 'reject'];
  const btns = actions.filter(a => a === 'accept' || a === 'reject').map(a => {
    const cls = a === 'accept' ? 'ok' : 'err';
    const label = a === 'accept' ? '✓ принять' : '✗ отклонить';
    return `<button class="${cls}" data-gate-action="${esc(a)}">${label}</button>`;
  }).join('');
  card.innerHTML = `
    <h3>👤 human_gate · ${esc(gw.step)} — ран ждёт твоего решения</h3>
    ${fields || '<div class="gstatus">форма пуста</div>'}
    <div class="gactions">${btns}</div>
    <div class="gstatus" id="gate-status"></div>`;
  card.querySelectorAll('[data-gate-action]').forEach(button => button.addEventListener('click', () => submitGate(id, button.dataset.gateAction)));
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
