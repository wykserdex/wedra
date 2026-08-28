// v0.11 GUI — минимальная реализация M6: drag-and-drop, live YAML, validate, JSON на линиях
const $ = s => document.querySelector(s);
let plugins = [];
let pipelines = [];
let nodes = []; // {id, plugin, x,y, bind:{}, on_error, form}
let selected = null;
let counter = 1;

async function api(path, opts={}) {
  const r = await fetch(path, opts);
  if (!r.ok) throw new Error(await r.text());
  const ct = r.headers.get('content-type')||'';
  if (ct.includes('json')) return r.json();
  return r.text();
}

function uid() { return 'step_'+(counter++); }

function renderPlugins() {
  const el = $('#plugin-list');
  el.innerHTML = '';
  plugins.forEach(p=>{
    const d = document.createElement('div');
    d.className='plugin';
    d.draggable=true;
    d.innerHTML = `<b>${p.id}</b><br/><small>${p.description||''} — ${p.runtime||''}</small><br/><small>IN: ${Object.keys(p.input||{}).join(', ')}</small>`;
    d.addEventListener('dragstart', e=>{ e.dataTransfer.setData('text/plugin', p.id); });
    d.addEventListener('click', ()=>{
      // quick add
      addNode(p.id, 100+Math.random()*200, 100+Math.random()*200);
    });
    el.appendChild(d);
  });
}

function renderPipelines() {
  const el = $('#pipeline-list');
  el.innerHTML='';
  pipelines.forEach(p=>{
    const d=document.createElement('div');
    d.className='plugin';
    d.textContent = p.file + ' — ' + (p.name||'');
    d.onclick=async ()=>{
      const yaml = await api('/api/pipelines/'+p.file);
      $('#yaml-preview').textContent = yaml;
      // naive parse: try to load into nodes
      try {
        // for MVP just show, not parse
        alert('Загрузка YAML в канвас — TODO M6, пока только просмотр. Скопируй YAML в редактор.');
      } catch(e){}
    };
    el.appendChild(d);
  });
}

function renderRuns() {
  api('/api/runs').then(runs=>{
    const el=$('#run-list');
    el.innerHTML='';
    runs.slice(-10).reverse().forEach(r=>{
      const d=document.createElement('div');
      d.className='plugin';
      d.innerHTML=`<b>${r.id}</b><br/><small>${r.pipeline||''} — ${r.events} events</small>`;
      d.onclick=async ()=>{
        const detail = await api('/api/runs/'+r.id);
        $('#context-out').textContent = JSON.stringify(detail, null, 2);
      };
      el.appendChild(d);
    });
  }).catch(()=>{});
}

function addNode(pluginId, x,y) {
  const n = {id: uid(), plugin: pluginId, x, y, bind:{}, on_error:'stop', form:[], _meta: plugins.find(p=>p.id===pluginId)};
  nodes.push(n);
  renderCanvas();
  selectNode(n.id);
  updateYAML();
}

function renderCanvas() {
  const c = $('#canvas');
  c.innerHTML='';
  nodes.forEach(n=>{
    const el=document.createElement('div');
    el.className='node'+(selected===n.id?' selected':'');
    el.style.left=n.x+'px';
    el.style.top=n.y+'px';
    const inputs = n._meta ? Object.keys(n._meta.input||{}) : [];
    const outputs = n._meta ? Object.keys(n._meta.output||{}) : [];
    el.innerHTML = `<h4>${n.id} <small style="color:#888">${n.plugin}</small></h4>
      <div class="port">IN: ${inputs.join(', ')||'—'}</div>
      <div class="port">OUT: ${outputs.join(', ')||'—'}</div>
      <div class="port">on_error: ${n.on_error}</div>
      <div class="row"><button data-act="edit">Edit</button><button data-act="del">X</button></div>`;
    el.onmousedown = (e)=>{
      if (e.target.tagName==='BUTTON') return;
      selected=n.id;
      renderCanvas();
      renderProps();
      let sx=e.clientX, sy=e.clientY, ox=n.x, oy=n.y;
      function mm(ev){ n.x=ox+(ev.clientX-sx); n.y=oy+(ev.clientY-sy); el.style.left=n.x+'px'; el.style.top=n.y+'px'; }
      function mu(){ document.removeEventListener('mousemove',mm); document.removeEventListener('mouseup',mu); updateYAML(); }
      document.addEventListener('mousemove',mm);
      document.addEventListener('mouseup',mu);
    };
    el.querySelector('[data-act="edit"]').onclick=()=>{ selected=n.id; renderProps(); };
    el.querySelector('[data-act="del"]').onclick=()=>{ nodes=nodes.filter(x=>x.id!==n.id); selected=null; renderCanvas(); renderProps(); updateYAML(); };
    c.appendChild(el);
  });
  // connections as simple lines (by bind)
  // for MVP just draw text, real SVG in v0.12
}

function selectNode(id){ selected=id; renderCanvas(); renderProps(); }

function renderProps() {
  const el=$('#prop-content');
  const n=nodes.find(x=>x.id===selected);
  if (!n){ el.textContent='выбери ноду'; return; }
  const meta=n._meta||{};
  const inPorts = Object.keys(meta.input||{});
  el.innerHTML = `
    <div>ID: <input id="p-id" value="${n.id}"/></div>
    <div>Plugin: <b>${n.plugin}</b></div>
    <div>on_error: <select id="p-onerr"><option>stop</option><option>skip</option><option>retry</option></select></div>
    <div><b>Bind (порт → путь)</b></div>
    <div id="bind-list"></div>
    <button id="btn-add-bind">+ bind</button>
    <div style="margin-top:8px"><b>Form (human_gate)</b><br/><small>field: steps.*.*</small></div>
    <textarea id="p-form" rows="4" placeholder='[{"field":"steps.prev.value"}]'>${JSON.stringify(n.form||[], null, 2)}</textarea>
    <div class="row"><button id="btn-save" class="primary">Save</button></div>
    <pre>${JSON.stringify(meta, null, 2)}</pre>
  `;
  $('#p-onerr').value=n.on_error;
  const bl=$('#bind-list');
  function renderBind(){
    bl.innerHTML='';
    Object.entries(n.bind).forEach(([k,v])=>{
      const row=document.createElement('div');
      row.className='row';
      row.innerHTML=`<input value="${k}" data-k="${k}" placeholder="port"/><input value="${v}" data-v="${k}" placeholder="input.xxx / steps.xxx"/><button data-del="${k}">x</button>`;
      bl.appendChild(row);
    });
    // also show unbound ports
    inPorts.forEach(port=>{
      if (!(port in n.bind)){
        const row=document.createElement('div');
        row.className='row';
        row.innerHTML=`<input value="${port}" disabled/><input placeholder="input.xxx" data-new="${port}"/><button data-add="${port}">+</button>`;
        bl.appendChild(row);
      }
    });
  }
  renderBind();
  bl.addEventListener('click', e=>{
    if (e.target.dataset.del){ delete n.bind[e.target.dataset.del]; renderBind(); updateYAML(); }
    if (e.target.dataset.add){ const port=e.target.dataset.add; const inp=bl.querySelector(`[data-new="${port}"]`).value; if(inp){ n.bind[port]=inp; renderBind(); updateYAML(); } }
  });
  bl.addEventListener('change', e=>{
    if (e.target.dataset.k){ const oldk=e.target.dataset.k; const newk=e.target.value; if(newk!==oldk){ n.bind[newk]=n.bind[oldk]; delete n.bind[oldk]; renderBind(); } }
    if (e.target.dataset.v){ n.bind[e.target.dataset.v]=e.target.value; updateYAML(); }
  });
  $('#p-id').onchange=e=>{ n.id=e.target.value; renderCanvas(); updateYAML(); };
  $('#p-onerr').onchange=e=>{ n.on_error=e.target.value; updateYAML(); };
  $('#p-form').onchange=e=>{ try{ n.form=JSON.parse(e.target.value); }catch{} updateYAML(); };
  $('#btn-save').onclick=()=>{ updateYAML(); renderCanvas(); };
  $('#btn-add-bind').onclick=()=>{
    const k=prompt('port name?'); const v=prompt('path input.xxx / steps.xxx?');
    if(k&&v){ n.bind[k]=v; renderBind(); updateYAML(); }
  };
}

function buildPipelineObject() {
  const name = $('#pipeline-name').value || 'my_chain';
  return {
    format_version: '0.2',
    pipeline: {
      name,
      input: { emails: ['test@yandex.ru'] },
      steps: nodes.map(n=>({
        id: n.id,
        plugin: n._meta ? (n._meta.dir||'plugins/'+n.plugin) : n.plugin,
        on_error: n.on_error,
        bind: n.bind,
        form: n.form,
      }))
    }
  };
}

function toYAML(obj){
  // very naive YAML for MVP — real yaml library in v0.12, now JSON-ish
  let s = `format_version: "${obj.format_version}"\n`;
  s += `pipeline:\n  name: ${obj.pipeline.name}\n  input:\n    emails: ["test@yandex.ru"]\n  steps:\n`;
  obj.pipeline.steps.forEach(st=>{
    s+=`    - id: ${st.id}\n      plugin: ${st.plugin}\n      on_error: ${st.on_error}\n`;
    if (Object.keys(st.bind||{}).length){
      s+=`      bind:\n`;
      for (let [k,v] of Object.entries(st.bind)) s+=`        ${k}: ${v}\n`;
    }
    if (st.form && st.form.length){
      s+=`      form:\n`;
      st.form.forEach(f=>{ s+=`        - field: ${f.field}\n`; if(f.editable) s+=`          editable: true\n`; });
    }
  });
  return s;
}

function updateYAML(){
  const obj=buildPipelineObject();
  const yaml=toYAML(obj);
  $('#yaml-preview').textContent=yaml;
  // also update window.pipeline for export
  window._lastYAML=yaml;
  window._lastObj=obj;
}

async function doValidate(){
  const yaml = window._lastYAML;
  if (!yaml){ $('#validate-out').textContent='сначала добавь ноды'; return; }
  try{
    const res = await api('/api/validate/pipeline', {method:'POST', body: yaml, headers:{'Content-Type':'text/yaml'}});
    $('#validate-out').textContent = JSON.stringify(res, null, 2);
    const badge=$('#status');
    if (res.ok){ badge.textContent='OK'; badge.className='badge ok'; }
    else { badge.textContent='ERR'; badge.className='badge err'; }
  }catch(e){ $('#validate-out').textContent=''+e; }
}

async function doRun(){
  alert('Run --yes в GUI — план v0.12: сейчас запусти через CLI `orchestrator pipeline run` или API. В v0.11 GUI только собирает и валидирует.');
}

$('#btn-validate').onclick=doValidate;
$('#btn-plan').onclick=doValidate;
$('#btn-run').onclick=doRun;
$('#btn-export').onclick=()=>{
  const blob=new Blob([window._lastYAML||''], {type:'text/yaml'});
  const a=document.createElement('a'); a.href=URL.createObjectURL(blob); a.download=($('#pipeline-name').value||'pipeline')+'.yaml'; a.click();
};

$('#canvas').addEventListener('dragover', e=>e.preventDefault());
$('#canvas').addEventListener('drop', e=>{
  e.preventDefault();
  const pid=e.dataTransfer.getData('text/plugin');
  if(pid){ addNode(pid, e.offsetX, e.offsetY); }
});

// init
(async ()=>{
  try{
    const h=await api('/api/health');
    $('#status').textContent='v'+h.version+' ok';
    $('#status').className='badge ok';
  }catch(e){ $('#status').textContent='offline'; $('#status').className='badge err'; }
  plugins=await api('/api/plugins');
  renderPlugins();
  pipelines=await api('/api/pipelines');
  renderPipelines();
  renderRuns();
  updateYAML();
})();
