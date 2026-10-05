// Стенд режима артефактов (отдельная страница web/static/artifacts/app.js).
//
// Проверяем логику против НАСТОЯЩЕГО ядра: сгенерированный YAML уходит на
// /api/validate/pipeline живого сервера. Сравнение строк с моим же ожиданием
// ничего бы не доказало про формат — пайплайн либо валиден, либо нет.
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const REPO = process.argv[2] || process.cwd();
const BASE = process.argv[3] || 'http://127.0.0.1:8765';
const SRC = path.join(REPO, 'web/static/artifacts/app.js');

// ── минимальный DOM: странице нужен только id, чтобы render() не упал ──────
const byId = {};
function mk(id, tag) {
  const e = {
    id, tagName: (tag || 'div').toUpperCase(), _h: {}, children: [], parent: null,
    dataset: {}, style: {}, value: '', textContent: '', hidden: false, disabled: false,
    innerHTML: '',
    classList: { add() {}, remove() {}, toggle() {}, contains() { return false; } },
    appendChild(c) { this.children.push(c); return c; },
    addEventListener(t, f) { (this._h[t] = this._h[t] || []).push(f); },
    removeEventListener() {},
    querySelector() { return null; },
    querySelectorAll() { return []; },
    setAttribute() {}, removeAttribute() {},
    getBoundingClientRect() { return { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 }; },
    closest() { return null; }, focus() {},
  };
  byId[id] = e;
  return e;
}
for (const id of ['drop', 'file', 'alist', 'chain', 'pluglist', 'q', 'status',
  'build', 'to-editor', 'clear', 'out']) mk(id, id === 'file' ? 'input' : 'div');

const doc = {
  querySelector: s => {
    const m = /^#([\w-]+)$/.exec(s);
    return m ? (byId[m[1]] || null) : null;
  },
  querySelectorAll: () => [],
  getElementById: id => byId[id] || null,
  createElement: t => mk('tmp-' + Math.random().toString(36).slice(2), t),
  addEventListener() {}, removeEventListener() {},
};
const win = { location: { href: '' } };

const ctx = vm.createContext({
  window: win, document: doc, console,
  setTimeout: () => 0, clearTimeout: () => {},
  // init() страницы сам дёргает /api/plugins и /api/assets: отдаём пустые
  // списки, иначе st.plugins станет объектом и render() упадёт.
  fetch: async u => ({
    ok: true, status: 200, headers: { get: () => 'application/json' },
    json: async () => (String(u).indexOf('plugins') >= 0 || String(u).indexOf('assets') >= 0 ? [] : {}),
    text: async () => '',
  }),
  location: win.location, FormData,
  sessionStorage: { getItem: () => null, setItem() {}, removeItem() {} },
  Date, Math, JSON, Object, Array, String, Number, Boolean, RegExp, Error, Promise,
});
vm.runInContext(fs.readFileSync(SRC, 'utf8'), ctx, { filename: 'artifacts/app.js' });
const A = vm.runInContext(
  '({st, assetKey, fileInputs, fileInputsLoose, suggest, buildYAML, yamlStr, infoOf, render, planStep, prevOutputs, outputsOf, isDataPort, OPS_PORTS})', ctx);

// ── плагины: берём НАСТОЯЩИЕ у живого сервера ───────────────────────────────
//
// Рукописные фикстуры дважды соврали: я объявил csv_loader.has_header и
// report_formatter.results обязательными, а в манифестах они optional — и
// стенд «находил» несуществующую проблему. Данные берём оттуда, где они
// настоящие: /api/plugins. Тогда проверка не может разойтись с реальностью.
// HTTP без fetch — сознательно.
//
// fetch тянет undici, и при разборе его асинхронного handle на Windows Node
// падает с «Assertion failed: !(handle->flags & UV_HANDLE_CLOSING)»
// (win/async.c:94). Проверки к тому моменту напечатаны и все зелёные, но
// процесс завершается с кодом 127 — стенд выглядит упавшим. process.reallyExit
// не помогает: авария приходит из teardown'а, а не из кода выхода. Поэтому
// здесь node:http с Connection: close — undici в процессе нет вовсе.
const http = require('http');

function request(method, path, body, contentType) {
  return new Promise((resolve, reject) => {
    const u = new URL(path, BASE);
    const req = http.request({
      hostname: u.hostname, port: u.port, path: u.pathname + u.search,
      method, agent: false,
      headers: Object.assign({ Connection: 'close' },
        contentType ? { 'Content-Type': contentType } : null,
        body != null ? { 'Content-Length': Buffer.byteLength(body) } : null),
    }, res => {
      let data = '';
      res.setEncoding('utf8');
      res.on('data', c => { data += c; });
      res.on('end', () => resolve({ status: res.statusCode, body: data }));
    });
    req.on('error', reject);
    if (body != null) req.write(body);
    req.end();
  });
}

async function loadPlugins() {
  const r = await request('GET', '/api/plugins');
  const list = JSON.parse(r.body);
  const byId = {};
  for (const p of list) byId[p.id] = p;
  return { list, byId };
}

let pass = 0, fail = 0;
function ok(name, cond, extra) {
  if (cond) { pass++; console.log('  OK   ' + name); }
  else { fail++; console.log('  FAIL ' + name + (extra !== undefined ? '  -> ' + JSON.stringify(extra) : '')); }
}

async function coreOK(yaml) {
  const r = await request('POST', '/api/validate/pipeline', yaml, 'application/yaml');
  return JSON.parse(r.body);
}

async function main() {
  const real = await loadPlugins();
  const PLUGINS = real.list;
  const need = ['csv_loader', 'batch_email_triage', 'report_formatter', 'exiftool', 'json_flatten'];
  const missing = need.filter(x => !real.byId[x]);
  if (missing.length) { console.error('нет плагинов в /api/plugins:', missing); process.exit(1); }
  const CSV = real.byId.csv_loader, TRIAGE = real.byId.batch_email_triage,
        REPORT = real.byId.report_formatter, EXIF = real.byId.exiftool, FLATTEN = real.byId.json_flatten;
  A.st.plugins = PLUGINS;
  A.st.assets = [{ name: 'photo.jpg', path: 'C:/work/assets/photo.jpg', size: 4096, ext: 'jpg' }];
  A.st.file = A.st.assets[0];

  // --- имя входа ---
  ok('photo.jpg → photo', A.assetKey('photo.jpg') === 'photo', A.assetKey('photo.jpg'));
  ok('2024.jpg не с цифры', A.assetKey('2024.jpg') === 'a2024', A.assetKey('2024.jpg'));
  ok('пустое имя не склеивается', /^artifact[0-9a-f]{6}$/.test(A.assetKey('')), A.assetKey(''));
  ok('разные пустые имена → разные входы', A.assetKey('.jpg') !== A.assetKey('.png'));

  // --- «файловый» вход: только format: file_ref, без догадок ---
  ok('csv_loader объявил файл', A.fileInputsLoose(CSV).join() === 'path', A.fileInputsLoose(CSV));
  // Раньше здесь стояло «exiftool НЕ файловый» — это было ОПИСАНИЕ бага:
  // манифест говорил «путь к файлу на диске», а объявлял format: text, из-за
  // чего подсказка по фотографии молчала. Теперь объявление исправлено.
  ok('exiftool объявляет вход файлом', A.fileInputsLoose(EXIF).join() === 'file', A.fileInputsLoose(EXIF));
  ok('но его wall_timeout — не данные', A.isDataPort('wall_timeout') === false);
  ok('batch_email_triage не файловый', A.fileInputsLoose(TRIAGE).length === 0);

  // --- подсказки по типам: настоящая цепочка ---
  const n1 = A.suggest(A.infoOf(CSV)).map(x => x.p.id + '.' + x.field);
  ok('array-выход нашёл array-вход', n1.indexOf('batch_email_triage.items') >= 0, n1);
  // csv_loader на выходе даёт array/array/number — объектного выхода у него
  // НЕТ, поэтому json_flatten.data (object) и не должен подходить.
  ok('object-вход не подошёл на array-выходы', n1.indexOf('json_flatten.data') < 0, n1);
  ok('csv_loader себя не предлагает', n1.every(x => x.indexOf('csv_loader') !== 0), n1);

  const n2 = A.suggest(A.infoOf(TRIAGE)).map(x => x.p.id + '.' + x.field);
  ok('object → object: report_formatter.by_verdict', n2.indexOf('report_formatter.by_verdict') >= 0, n2);
  ok('object-выход не подошёл на array-вход', n2.indexOf('batch_email_triage.items') < 0, n2);

  // exiftool объявляет только string-выходы object/number — объект найдёт flatten
  const n3 = A.suggest(A.infoOf(EXIF)).map(x => x.p.id + '.' + x.field);
  ok('после exiftool объект нашёлся', n3.indexOf('json_flatten.data') >= 0, n3);
  // Раньше здесь стояла проверка «csv_loader не должен появиться среди
  // подсказок после exiftool» — она бессмысленна: exiftool.file объявлен
  // строкой, значит строковые входы (включая csv_loader.path) подходят по
  // типу, и это правильно. Признак «файловый вход» проверяется отдельно,
  // через fileInputsLoose, и он там зелёный.
  ok('и в этих подсказках нет рабочих портов', n3.every(x => x.indexOf('wall_timeout') < 0),
    n3.filter(x => x.indexOf('wall_timeout') >= 0).slice(0, 3));

  // --- файл получают ЛЮБЫЕ шаги, а не только первый ---
  // Именно это ломало сценарий человека: фото → csv_loader → exiftool, и
  // «не хватает: file», хотя файл лежит на диске. Проверяем два шага, оба
  // читающие один файл, — без двусмысленного items внутри.
  A.st.chain = [];
  for (const p of [CSV, EXIF]) {
    const plan = A.planStep(p, A.st.chain.length === 0);
    A.st.chain.push({ plugin: p.id, dir: p.dir, binds: plan.binds, missing: plan.missing, ambiguous: plan.ambiguous });
  }
  ok('exiftool объявил вход файлом', A.fileInputsLoose(EXIF).join() === 'file', A.fileInputsLoose(EXIF));
  ok('Шаг 1 получил файл', A.st.chain[0].binds.some(x => x.field === 'path' && x.from === null), A.st.chain[0].binds);
  ok('Шаг 2 тоже получил файл, хотя он не первый', A.st.chain[1].binds.some(x => x.field === 'file' && x.from === null), A.st.chain[1].binds);
  ok('Шаг 2 ни на что не жалуется', A.st.chain[1].missing.length === 0, A.st.chain[1].missing);
  const yx = A.buildYAML();
  ok('в YAML два шага читают один input', (yx.match(/input\.photo/g) || []).length === 2, (yx.match(/input\.photo/g) || []).length);
  const jx = await coreOK(yx);
  ok('ЯДРО ПРИНЯЛО два шага на одном файле', jx.ok === true, jx.errors || jx.issues);
  A.st.chain = [];

  // --- YAML: пусто ---
  ok('без шагов yaml пустой', A.buildYAML() === '');

  // --- YAML: настоящая цепочка из трёх шагов против живого ядра ---
  // Шаги планируются как в интерфейсе: planStep связывает ВСЕ обязательные
  // порты, а не один. Прежняя версия оставляла results unbound, и ядро
  // справедливо отвергало пайплайн.
  // Строим цепочку по очереди, как в интерфейсе: planStep смотрит на уже
  // добавленные шаги, поэтому и здесь шаг за шагом.
  A.st.chain = [];
  for (const p of [CSV, TRIAGE, REPORT]) {
    const plan = A.planStep(p, A.st.chain.length === 0);
    A.st.chain.push({ plugin: p.id, dir: p.dir, binds: plan.binds, missing: plan.missing, ambiguous: plan.ambiguous });
  }
  ok('csv_loader получил файл в свой file_ref-порт',
    A.st.chain[0].binds.map(x => x.field).join() === 'path', A.st.chain[0].binds);
  ok('у csv_loader нет недостающих портов', A.st.chain[0].missing.length === 0, A.st.chain[0].missing);
  // csv_loader отдаёт и rows, и headers — оба array. Молча выбрать headers
  // значило бы построить ВАЛИДНЫЙ, но бессмысленный пайплайн (в triage уедут
  // заголовки). Поэтому неоднозначность обязана быть видна и выбрана руками.
  const amb1 = A.st.chain[1].ambiguous || [];
  ok('triage: неоднозначность замечена, а не угадана', amb1.length === 1 && amb1[0].field === 'items', amb1);
  ok('triage: варианты — rows и headers',
    amb1[0] && amb1[0].choices.slice().sort().join() === 'headers,rows', amb1[0] && amb1[0].choices);
  ok('items НЕ связан молча', !A.st.chain[1].binds.some(x => x.field === 'items'), A.st.chain[1].binds);
  // выбираем rows вручную, как это делает человек в интерфейсе
  {
    const c = A.st.chain[1];
    const a = (c.ambiguous || []).find(x => x.field === 'items');
    c.binds = c.binds.filter(x => x.field !== a.field);
    c.binds.push({ field: a.field, from: a.choices.filter(x => x === 'rows')[0] });
    c.ambiguous = c.ambiguous.filter(x => x.field !== a.field);
  }
  ok('после выбора items ← rows', A.st.chain[1].binds.some(x => x.field === 'items' && x.from === 'rows'), A.st.chain[1].binds);
  ok('report_formator получил by_verdict', A.st.chain[2].binds.some(x => x.field === 'by_verdict' && x.from === 'by_verdict'),
    A.st.chain[2].binds);
  ok('report_formator получил и by_verdict, и results',
    A.st.chain[2].binds.map(x => x.field).sort().join() === 'by_verdict,results', A.st.chain[2].binds);
  ok('report_formator ни на что не жалуется', A.st.chain[2].missing.length === 0, A.st.chain[2].missing);
  const y = A.buildYAML();
  ok('вход пайплайна с путём', y.indexOf('photo: "C:/work/assets/photo.jpg"') >= 0, y);
  ok('шаг 1 связан с входом файла', y.indexOf('bind: { path: input.photo }') >= 0, y);
  // pos обязателен: без него редактор расставляет шаги случайно и они
  // накладываются (проверено скриншотом).
  ok('у шагов есть pos', y.indexOf('pos: [40, 60]') >= 0 && y.indexOf('pos: [380, 60]') >= 0, y);
  ok('шаг 2 связан с выходом шага 1 (выбрано вручную)', y.indexOf('items: steps.s1.rows') >= 0, y);
  ok('шаг 3 связан с выходом шага 2', y.indexOf('by_verdict: steps.s2.by_verdict') >= 0, y);
  const j = await coreOK(y);
  ok('ЯДРО ПРИНЯЛО трёхшаговую цепочку', j.ok === true, j.errors || j.issues);

  // --- ядро обязано и отвергать, иначе проверка выше ничего не значит ---
  A.st.chain = [{ plugin: 'нет_такого', dir: 'plugins/community/нет_такого', binds: [], missing: [] }];
  const jBad = await coreOK(A.buildYAML());
  ok('ядро отвергает несуществующий плагин', jBad.ok === false, jBad.ok);

  // --- YAML не должен ломаться на пути с кавычкой ---
  A.st.assets = [{ name: 'x.jpg', path: 'C:/wor"k/assets/x.jpg', size: 10, ext: 'jpg' }];
  A.st.file = A.st.assets[0];
  A.st.chain = [{ plugin: 'csv_loader', dir: CSV.dir, binds: [{ field: 'path', from: null }], missing: [] }];
  const jQ = await coreOK(A.buildYAML()).catch(() => ({ ok: false, errors: ['ответ не JSON'] }));
  ok('путь с кавычкой не ломает yaml', jQ.ok === true, jQ.errors || jQ.issues);

  // --- имя файла приходит с сервера уже ASCII (транслитерация в assets.go) ---
  A.st.assets = [{ name: 'moe_foto.jpg', path: 'C:/work/assets/moe_foto.jpg', size: 10, ext: 'jpg' }];
  A.st.file = A.st.assets[0];
  A.st.chain = [{ plugin: 'csv_loader', dir: CSV.dir, binds: [{ field: 'path', from: null }], missing: [] }];
  const yt = A.buildYAML();
  ok('сервер уже транслитерировал — ключ input.moe_foto', yt.indexOf('moe_foto: "') >= 0,
    yt.split('\n').slice(4, 8).join(' | '));
  const jT = await coreOK(yt);
  ok('ядро приняло пайплайн с таким входом', jT.ok === true, jT.errors || jT.issues);

  // --- yamlStr напрямую ---
  const BS = String.fromCharCode(92), Q = String.fromCharCode(34);
  // рабочие порты не данные
  ok('wall_timeout не считается данными', A.isDataPort('wall_timeout') === false);
  ok('timeout не считается данными', A.isDataPort('timeout') === false);
  ok('обычное поле — данные', A.isDataPort('rows') === true);
  const sugg = A.suggest(A.infoOf(CSV)).map(x => x.p.id + '.' + x.field);
  ok('в подсказках нет wall_timeout', sugg.every(x => x.indexOf('wall_timeout') < 0),
    sugg.filter(x => x.indexOf('wall_timeout') >= 0).slice(0, 5));

  ok('обратный слэш удвоен', A.yamlStr('C:' + BS + 'w' + BS + 'x.jpg') === '"C:\\\\w\\\\x.jpg"',
    A.yamlStr('C:' + BS + 'w' + BS + 'x.jpg'));
  ok('кавычка экранирована', A.yamlStr('a"b') === '"a\\"b"', A.yamlStr('a"b'));

  console.log('\n' + (fail ? `провалено ${fail}, ` : '') + `пройдено ${pass}`);
  console.log(fail ? '\nСТЕНД НЕ ЗЕЛЕНЫЙ' : '\nвсе проверки пройдены');
  reallyExit(fail ? 1 : 0);
}
main().catch(e => { console.error('ИСКЛЮЧЕНИЕ:', e && e.stack || e); reallyExit(1); });

// Выход через reallyExit, а не process.exit.
//
// Стенд ходит в живой API настоящим fetch, и при разборе его асинхронного
// handle на Windows Node падает с «Assertion failed: !(handle->flags &
// UV_HANDLE_CLOSING)» (win/async.c:94). Проверки к этому моменту уже
// напечатаны и все зелёные, но процесс завершается с кодом 127 — то есть
// стенд выглядит упавшим. reallyExit выходит мимо teardown.
function reallyExit(code) {
  if (process.reallyExit) process.reallyExit(code);
  else process.exit(code);
}