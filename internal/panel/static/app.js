// API WhatsApp BOB - Painel web. Desenvolvido por Edton Braz.
(() => {
  "use strict";
  const $ = (s, el = document) => el.querySelector(s);
  const $$ = (s, el = document) => [...el.querySelectorAll(s)];
  const esc = (v) => String(v ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const store = {
    get() { try { return sessionStorage.getItem("bob.key") || localStorage.getItem("bob.key") || ""; } catch { return ""; } },
    set(k, remember) { try { sessionStorage.setItem("bob.key", k); if (remember) localStorage.setItem("bob.key", k); } catch {} },
    clear() { try { sessionStorage.removeItem("bob.key"); localStorage.removeItem("bob.key"); } catch {} },
  };
  let KEY = store.get();
  let instances = [];
  let refreshTimer = null;

  // ------------------------------------------------------------ API
  async function api(method, path, body, instance) {
    const headers = { apikey: KEY };
    if (instance) headers.instance = instance;
    if (body !== undefined) headers["Content-Type"] = "application/json";
    const res = await fetch(path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined });
    let data = null;
    try { data = await res.json(); } catch {}
    if (!res.ok) {
      if (res.status === 401 && !instance) { logout(); }
      throw new Error((data && (data.error || data.message)) || "Erro " + res.status);
    }
    return data && "data" in data ? data.data : data;
  }

  function toast(msg, bad) {
    const t = $("#toast");
    t.textContent = msg;
    t.className = "toast" + (bad ? " bad" : "");
    clearTimeout(t._h);
    t._h = setTimeout(() => t.classList.add("hidden"), 4000);
  }

  async function run(btn, fn) {
    if (btn) btn.disabled = true;
    try { return await fn(); } catch (e) { toast(e.message, true); } finally { if (btn) btn.disabled = false; }
  }

  // ------------------------------------------------------------ entrada
  function showLogin() { $("#login").classList.remove("hidden"); $("#app").classList.add("hidden"); $("#login-key").focus(); }
  function logout() { store.clear(); KEY = ""; clearInterval(refreshTimer); closeSocket(); showLogin(); }
  $("#login-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    KEY = $("#login-key").value.trim();
    $("#login-error").textContent = "";
    try {
      await api("GET", "/server/info");
      store.set(KEY, $("#login-remember").checked);
      start();
    } catch (err) {
      $("#login-error").textContent = "Chave recusada. Confira a GLOBAL_API_KEY do servidor.";
      KEY = "";
    }
  });
  $("#logout").addEventListener("click", logout);

  // ------------------------------------------------------------ navegação
  const titles = { instancias: "Instâncias", enviar: "Enviar mensagem", chamadas: "Chamadas", eventos: "Eventos ao vivo", servidor: "Servidor", apoie: "Apoie o projeto" };
  function go() {
    const page = (location.hash || "#instancias").slice(1);
    const name = titles[page] ? page : "instancias";
    $$(".page").forEach((p) => p.classList.toggle("hidden", p.id !== "page-" + name));
    $$("nav a[data-page]").forEach((a) => a.classList.toggle("active", a.dataset.page === name));
    $("#page-title").textContent = titles[name];
    $("#sidebar").classList.remove("open");
    if (name === "servidor") loadServer();
    if (name === "eventos") openSocket();
    if (name === "apoie") loadPix();
    if (name === "enviar") fillSelect($("#send-instance"), false);
    if (name === "chamadas") openCalls(); else stopCalls();
  }
  window.addEventListener("hashchange", go);
  $("#menu").addEventListener("click", () => $("#sidebar").classList.toggle("open"));

  async function start() {
    $("#login").classList.add("hidden");
    $("#app").classList.remove("hidden");
    go();
    await refresh();
    clearInterval(refreshTimer);
    refreshTimer = setInterval(refresh, 8000);
  }

  // ------------------------------------------------------------ instâncias
  const STATE = { open: ["Conectada", "ok"], connecting: ["Conectando", "warn"], close: ["Desconectada", "bad"] };

  async function refresh() {
    try {
      const [list, info] = await Promise.all([api("GET", "/instance/all"), api("GET", "/server/info")]);
      instances = list || [];
      $("#version").textContent = "v" + info.version;
      $("#server-status").textContent = "API no ar · " + info.uptime;
      $("#server-status").className = "pill ok";
      renderStats(info);
      renderInstances();
    } catch (e) {
      $("#server-status").textContent = "API sem resposta";
      $("#server-status").className = "pill bad";
    }
  }

  function renderStats(info) {
    const off = instances.length - instances.filter((i) => i.connected).length;
    $("#stats").innerHTML = `
      <div class="stat"><span class="muted">Instâncias</span><b>${instances.length}</b></div>
      <div class="stat ok"><span class="muted">Conectadas</span><b>${instances.filter((i) => i.connected).length}</b></div>
      <div class="stat bad"><span class="muted">Desconectadas</span><b>${off}</b></div>
      <div class="stat"><span class="muted">Versão</span><b>${esc(info.version)}</b></div>`;
  }

  function renderInstances() {
    const q = $("#filter").value.trim().toLowerCase();
    const list = instances.filter((i) => !q || (i.name + " " + (i.phone || "") + " " + (i.profileName || "")).toLowerCase().includes(q));
    if (!list.length) {
      $("#instances").innerHTML = `<div class="card empty"><p>${instances.length ? "Nenhuma instância encontrada." : "Nenhuma instância ainda."}</p>${instances.length ? "" : '<button class="btn primary" data-act="new">+ Criar a primeira instância</button>'}</div>`;
      return;
    }
    $("#instances").innerHTML = list.map((i) => {
      const [label, cls] = STATE[i.state] || STATE.close;
      return `<article class="card inst" data-id="${esc(i.id)}">
        <div class="inst-head">
          <div class="avatar ${i.connected ? "on" : ""}">${i.connected ? "🟢" : "📱"}</div>
          <div style="min-width:0;flex:1"><h3>${esc(i.name)}</h3><div class="muted">${esc(i.profileName || (i.phone ? "+" + i.phone : "sem número"))}</div></div>
          <span class="pill ${cls}">${label}</span>
        </div>
        <div class="inst-meta">
          <span>Número</span><span>${i.phone ? "+" + esc(i.phone) : "—"}</span>
          <span>Webhook</span><span title="${esc(i.webhook)}">${esc(i.webhook || "não definido")}</span>
          <span>Eventos</span><span>${esc(i.events || "todos")}</span>
          <span>Criada em</span><span>${new Date(i.createdAt).toLocaleString("pt-BR")}</span>
        </div>
        <div class="inst-actions">
          ${i.connected ? "" : '<button class="btn primary small" data-act="connect">Conectar</button>'}
          <button class="btn small" data-act="settings">Configurar</button>
          <button class="btn small" data-act="logs">Registro</button>
          ${i.connected ? '<button class="btn small" data-act="reconnect">Reconectar</button><button class="btn small" data-act="disconnect">Desconectar</button><button class="btn small danger" data-act="logout">Sair do WhatsApp</button>' : ""}
          <button class="btn small danger" data-act="delete">Excluir</button>
        </div>
      </article>`;
    }).join("");
  }

  $("#filter").addEventListener("input", renderInstances);
  $("#new-instance").addEventListener("click", newInstance);
  $("#instances").addEventListener("click", (e) => {
    const btn = e.target.closest("button[data-act]");
    if (!btn) return;
    if (btn.dataset.act === "new") return newInstance();
    const inst = instances.find((i) => i.id === btn.closest("[data-id]").dataset.id);
    const acts = {
      connect: () => connectDialog(inst),
      settings: () => settingsDialog(inst),
      logs: () => logsDialog(inst),
      reconnect: () => run(btn, async () => { await api("POST", "/instance/reconnect", {}, inst.id); toast("Reconectando " + inst.name); refresh(); }),
      disconnect: () => run(btn, async () => { await api("POST", "/instance/disconnect", {}, inst.id); toast(inst.name + " desconectada. O login continua salvo."); refresh(); }),
      logout: () => confirm(`Encerrar a sessão de "${inst.name}" no WhatsApp? Será preciso ler o QR Code de novo.`) && run(btn, async () => { await api("DELETE", "/instance/logout", undefined, inst.id); toast("Sessão encerrada"); refresh(); }),
      delete: () => confirm(`Excluir a instância "${inst.name}"? A sessão do WhatsApp também será encerrada.`) && run(btn, async () => { await api("DELETE", "/instance/delete/" + inst.id); toast("Instância excluída"); refresh(); }),
    };
    acts[btn.dataset.act] && acts[btn.dataset.act]();
  });

  // ------------------------------------------------------------ janelas
  const modal = $("#modal");
  let modalCleanup = null;
  function openModal(title, html) {
    $("#modal-title").textContent = title;
    $("#modal-body").innerHTML = html;
    if (!modal.open) modal.showModal();
    return $("#modal-body");
  }
  function closeModal() { if (modalCleanup) { modalCleanup(); modalCleanup = null; } modal.close(); }
  $("#modal-close").addEventListener("click", closeModal);
  modal.addEventListener("close", () => { if (modalCleanup) { modalCleanup(); modalCleanup = null; } });

  function newInstance() {
    const body = openModal("Nova instância", `
      <form id="f-new">
        <label>Nome (sem espaços, ex.: vendas) <input name="name" required pattern="[A-Za-z0-9_.\\-]+" placeholder="vendas"></label>
        <label>Token (opcional, gerado se ficar em branco) <input name="token" placeholder="deixe em branco"></label>
        <label>Webhook (opcional) <input name="webhook" type="url" placeholder="https://seu-sistema.com/webhook"></label>
        <div class="actions"><button class="btn primary" type="submit">Criar e conectar</button></div>
      </form>`);
    $("#f-new", body).addEventListener("submit", (e) => {
      e.preventDefault();
      const f = new FormData(e.target);
      run(e.submitter, async () => {
        const inst = await api("POST", "/instance/create", { name: f.get("name").trim(), token: f.get("token").trim(), webhook: f.get("webhook").trim() });
        toast("Instância criada");
        await refresh();
        connectDialog(instances.find((i) => i.id === inst.id) || inst);
      });
    });
  }

  function connectDialog(inst) {
    const body = openModal("Conectar " + inst.name, `
      <div class="tabs"><button class="active" data-tab="qr">QR Code</button><button data-tab="pair">Código pelo número</button></div>
      <div data-pane="qr" class="qr">
        <p class="muted">No celular: WhatsApp › Aparelhos conectados › Conectar aparelho, e aponte para o código.</p>
        <div id="qr-box"><p class="muted">Gerando QR Code…</p></div>
        <span id="qr-state" class="pill warn">aguardando leitura</span>
      </div>
      <div data-pane="pair" class="hidden">
        <p class="muted">Sem câmera? Gere um código e digite no celular em Aparelhos conectados › Conectar com número de telefone.</p>
        <form id="f-pair" class="row"><label>Número com DDI e DDD <input name="phone" placeholder="5511999998888" required></label><div class="actions" style="align-items:end"><button class="btn primary" type="submit">Gerar código</button></div></form>
        <div id="pair-out" class="qr"></div>
      </div>`);
    $$(".tabs button", body).forEach((b) => b.addEventListener("click", () => {
      $$(".tabs button", body).forEach((x) => x.classList.toggle("active", x === b));
      $$("[data-pane]", body).forEach((p) => p.classList.toggle("hidden", p.dataset.pane !== b.dataset.tab));
    }));
    $("#f-pair", body).addEventListener("submit", (e) => {
      e.preventDefault();
      run(e.submitter, async () => {
        const r = await api("POST", "/instance/pair", { phone: new FormData(e.target).get("phone") }, inst.id);
        $("#pair-out", body).innerHTML = `<div class="pair-code">${esc(r.PairingCode || r.pairingCode)}</div><p class="muted">Digite este código no celular. Ele vale por poucos minutos.</p>`;
      });
    });
    let stop = false;
    modalCleanup = () => { stop = true; };
    (async () => {
      try { await api("POST", "/instance/connect", {}, inst.id); } catch (e) { $("#qr-box", body).innerHTML = `<p class="error">${esc(e.message)}</p>`; }
      let last = "";
      while (!stop) {
        try {
          const st = await api("GET", "/instance/status", undefined, inst.id);
          if (st.LoggedIn && st.Connected) {
            $("#qr-box", body).innerHTML = `<div class="pair-code">✓</div><p>Conectada como <strong>${esc(st.Name)}</strong></p>`;
            $("#qr-state", body).textContent = "conectada"; $("#qr-state", body).className = "pill ok";
            toast(inst.name + " conectada"); refresh();
            return;
          }
          const qr = await api("GET", "/instance/qr", undefined, inst.id).catch(() => null);
          if (qr && qr.qrcode && qr.qrcode !== last) {
            last = qr.qrcode;
            $("#qr-box", body).innerHTML = `<img src="${qr.qrcode}" alt="QR Code">`;
          }
        } catch {}
        await new Promise((r) => setTimeout(r, 2500));
      }
    })();
  }

  const EVENTS = ["MESSAGE", "SEND_MESSAGE", "READ_RECEIPT", "CONNECTION", "QRCODE", "PRESENCE", "CHAT_PRESENCE", "CALL", "CONTACT", "GROUP", "LABEL", "CHAT", "HISTORY_SYNC", "NEWSLETTER", "BLOCKLIST"];
  const INTEGRATIONS = {
    chatwoot: { title: "Chatwoot", fields: [["url", "URL do Chatwoot", "https://chatwoot.seudominio.com"], ["accountId", "ID da conta", "1"], ["token", "Token de acesso (perfil › token)", "", "password"], ["inboxId", "ID da caixa de entrada (API)", "3", "number"]], checks: [["signMsg", "Assinar mensagens com o nome do agente"], ["importFromMe", "Enviar ao Chatwoot as mensagens enviadas pelo celular"]] },
    typebot: { title: "Typebot", fields: [["url", "URL do Typebot (viewer)", "https://typebot.seudominio.com"], ["typebot", "ID público do fluxo", "meu-fluxo"], ["apiToken", "Token da API (opcional)", "", "password"], ["triggerType", "Quando responder (all, keyword, none)", "all"], ["triggerValue", "Palavra-chave (se keyword)", ""], ["keywordFinish", "Palavra para encerrar", "#sair"], ["expire", "Encerrar sessão sem resposta (minutos)", "0", "number"]] },
    n8n: { title: "N8N", fields: [["webhookUrl", "URL do webhook do fluxo", "https://n8n.seudominio.com/webhook/bob"], ["basicAuthUser", "Usuário (opcional)", ""], ["basicAuthPass", "Senha (opcional)", "", "password"], ["triggerType", "Quando chamar (all, keyword, none)", "all"], ["triggerValue", "Palavra-chave (se keyword)", ""]] },
  };

  async function settingsDialog(inst) {
    const fresh = await api("GET", "/instance/info/" + inst.id).catch(() => inst);
    const s = fresh.advancedSettings || {};
    const chosen = (fresh.events || "").split(",").filter(Boolean);
    const body = openModal("Configurar " + inst.name, `
      <div class="tabs">
        <button class="active" data-tab="webhook">Webhook</button><button data-tab="comportamento">Comportamento</button>
        <button data-tab="integracoes">Integrações</button><button data-tab="proxy">Proxy</button><button data-tab="acesso">Acesso</button>
      </div>
      <form data-pane="webhook" id="f-webhook">
        <label>URL do webhook <input name="webhook" type="url" value="${esc(fresh.webhook)}" placeholder="https://seu-sistema.com/webhook"></label>
        <p class="muted small">Eventos enviados (nenhum marcado = todos):</p>
        <div class="checks">${EVENTS.map((e) => `<label class="check"><input type="checkbox" name="ev" value="${e}" ${chosen.includes(e) || chosen.includes("ALL") ? "checked" : ""}> ${e}</label>`).join("")}</div>
        <label class="check"><input type="checkbox" name="byEvents" ${fresh.webhookByEvents ? "checked" : ""}> Um endereço por evento (URL/messages-upsert, URL/connection-update...)</label>
        <label class="check"><input type="checkbox" name="base64" ${fresh.webhookBase64 === "off" ? "" : fresh.webhookBase64 === "on" || fresh.webhookFilesDefault ? "checked" : ""}> Mídia recebida em base64 no webhook</label>
        <div class="actions"><button class="btn primary" type="submit">Salvar webhook</button></div>
      </form>
      <form data-pane="comportamento" id="f-adv" class="hidden">
        <label class="check"><input type="checkbox" name="rejectCall" ${s.rejectCall ? "checked" : ""}> Recusar ligações</label>
        <label>Mensagem ao recusar ligação <input name="msgRejectCall" value="${esc(s.msgRejectCall)}" placeholder="No momento não atendemos ligações."></label>
        <label class="check"><input type="checkbox" name="readMessages" ${s.readMessages ? "checked" : ""}> Marcar mensagens recebidas como lidas</label>
        <label class="check"><input type="checkbox" name="alwaysOnline" ${s.alwaysOnline ? "checked" : ""}> Aparecer sempre online</label>
        <label class="check"><input type="checkbox" name="ignoreGroups" ${s.ignoreGroups ? "checked" : ""}> Ignorar mensagens de grupos</label>
        <label class="check"><input type="checkbox" name="ignoreStatus" ${s.ignoreStatus ? "checked" : ""}> Ignorar status (stories)</label>
        <div class="actions"><button class="btn primary" type="submit">Salvar</button></div>
      </form>
      <div data-pane="integracoes" class="hidden">
        <div class="tabs" id="int-tabs">${Object.entries(INTEGRATIONS).map(([k, v], n) => `<button type="button" data-int="${k}" class="${n ? "" : "active"}">${v.title}</button>`).join("")}</div>
        <div id="int-form"></div>
      </div>
      <form data-pane="proxy" id="f-proxy" class="hidden">
        <p class="muted small">Proxy de saída só desta instância. Em branco, usa o proxy do servidor (se houver).</p>
        <div class="row">
          <label>Protocolo <select name="protocol"><option value="http">http</option><option value="https">https</option><option value="socks5">socks5</option></select></label>
          <label>Host <input name="host" placeholder="proxy.exemplo.com"></label>
          <label>Porta <input name="port" placeholder="8080"></label>
        </div>
        <div class="row"><label>Usuário <input name="username"></label><label>Senha <input name="password" type="password"></label></div>
        <div class="actions"><button class="btn ghost" type="button" id="proxy-del">Remover proxy</button><button class="btn primary" type="submit">Salvar proxy</button></div>
      </form>
      <div data-pane="acesso" class="hidden">
        <label>ID da instância <input readonly value="${esc(fresh.id)}"></label>
        <label>Token da instância (cabeçalho apikey) <input readonly value="${esc(fresh.token)}" id="tok"></label>
        <div class="actions"><button class="btn" id="copy-tok" type="button">Copiar token</button></div>
        <p class="muted small">Use o token para enviar mensagens pela API. Exemplo:</p>
        <pre class="result">curl -X POST ${esc(location.origin)}/send/text \\
  -H "apikey: ${esc(fresh.token)}" -H "Content-Type: application/json" \\
  -d '{"number":"5511999998888","text":"Olá!"}'</pre>
      </div>`);
    $$(".tabs:not(#int-tabs) button", body).forEach((b) => b.addEventListener("click", () => {
      $$(".tabs:not(#int-tabs) button", body).forEach((x) => x.classList.toggle("active", x === b));
      $$("[data-pane]", body).forEach((p) => p.classList.toggle("hidden", p.dataset.pane !== b.dataset.tab));
    }));
    $("#f-webhook", body).addEventListener("submit", (e) => {
      e.preventDefault();
      const f = new FormData(e.target);
      run(e.submitter, async () => { await api("POST", `/instance/${inst.id}/webhook`, { webhook: f.get("webhook"), subscribe: f.getAll("ev"), byEvents: f.get("byEvents") === "on", base64: f.get("base64") === "on" }); toast("Webhook salvo"); refresh(); });
    });
    $("#f-adv", body).addEventListener("submit", (e) => {
      e.preventDefault();
      const f = e.target;
      const v = { msgRejectCall: f.msgRejectCall.value };
      ["rejectCall", "readMessages", "alwaysOnline", "ignoreGroups", "ignoreStatus"].forEach((k) => (v[k] = f[k].checked));
      run(e.submitter, async () => { await api("PUT", `/instance/${inst.id}/advanced-settings`, v); toast("Configurações salvas"); });
    });
    try {
      if (fresh.proxy) { const p = JSON.parse(fresh.proxy); const f = $("#f-proxy", body); ["protocol", "host", "port", "username"].forEach((k) => p[k] && (f[k].value = p[k])); }
    } catch {}
    $("#f-proxy", body).addEventListener("submit", (e) => {
      e.preventDefault();
      const f = Object.fromEntries(new FormData(e.target));
      run(e.submitter, async () => { await api("POST", "/instance/proxy/" + inst.id, f); toast("Proxy salvo"); });
    });
    $("#proxy-del", body).addEventListener("click", (e) => run(e.target, async () => { await api("DELETE", "/instance/proxy/" + inst.id); e.target.form.reset(); toast("Proxy removido"); }));
    $("#copy-tok", body).addEventListener("click", () => navigator.clipboard.writeText(fresh.token).then(() => toast("Token copiado")));
    const showInt = async (kind) => {
      $$("#int-tabs button", body).forEach((x) => x.classList.toggle("active", x.dataset.int === kind));
      const def = INTEGRATIONS[kind];
      const cur = await api("GET", "/" + kind + "/find", undefined, inst.id).catch(() => ({}));
      const cfg = cur.config || {};
      $("#int-form", body).innerHTML = `<form id="f-int">
        <label class="check"><input type="checkbox" name="enabled" ${cur.configured ? (cur.enabled ? "checked" : "") : "checked"}> Ativado</label>
        ${def.fields.map(([k, l, ph, t]) => `<label>${l} <input name="${k}" type="${t || "text"}" placeholder="${esc(t === "password" && cur.configured ? "manter o atual" : ph)}" value="${t === "password" ? "" : esc(cfg[k] ?? "")}"></label>`).join("")}
        ${(def.checks || []).map(([k, l]) => `<label class="check"><input type="checkbox" name="${k}" ${cfg[k] ? "checked" : ""}> ${l}</label>`).join("")}
        ${cur.webhookUrl ? `<label>Webhook para colar na caixa de entrada do Chatwoot <input readonly value="${esc(cur.webhookUrl)}"></label>` : ""}
        <div class="actions">${cur.configured ? '<button class="btn ghost danger" type="button" id="int-del">Remover</button>' : ""}<button class="btn primary" type="submit">Salvar ${def.title}</button></div>
      </form>`;
      $("#f-int", body).addEventListener("submit", (e) => {
        e.preventDefault();
        const f = e.target, v = { enabled: f.enabled.checked };
        def.fields.forEach(([k, , , t]) => (v[k] = t === "number" ? Number(f[k].value || 0) : f[k].value.trim()));
        (def.checks || []).forEach(([k]) => (v[k] = f[k].checked));
        run(e.submitter, async () => { await api("POST", "/" + kind + "/set", v, inst.id); toast(def.title + " salvo"); showInt(kind); });
      });
      const del = $("#int-del", body);
      del && del.addEventListener("click", () => run(del, async () => { await api("DELETE", "/" + kind, undefined, inst.id); toast(def.title + " removido"); showInt(kind); }));
    };
    $$("#int-tabs button", body).forEach((b) => b.addEventListener("click", () => showInt(b.dataset.int)));
    showInt("chatwoot");
  }

  async function logsDialog(inst) {
    const body = openModal("Registro de " + inst.name, `<div class="logs" id="logs">Carregando…</div>`);
    const load = async () => {
      const list = await api("GET", "/instance/logs/" + inst.id).catch(() => []);
      $("#logs", body).innerHTML = list.length ? list.slice().reverse().map((l) => `<div class="lv-${esc(l.level)}">${new Date(l.time).toLocaleString("pt-BR")} · ${esc(l.level)} · ${esc(l.message)}</div>`).join("") : '<p class="muted">Sem registros desde o último início da API.</p>';
    };
    await load();
    const t = setInterval(load, 5000);
    modalCleanup = () => clearInterval(t);
  }

  // ------------------------------------------------------------ enviar
  function fillSelect(sel, withAll) {
    const cur = sel.value;
    sel.innerHTML = (withAll ? '<option value="">Todas as instâncias</option>' : "") + instances.map((i) => `<option value="${esc(i.id)}">${esc(i.name)}${i.connected ? "" : " (desconectada)"}</option>`).join("");
    if (cur) sel.value = cur;
  }
  $("#send-type").addEventListener("change", (e) => $$("#send-form [data-type]").forEach((d) => d.classList.toggle("hidden", d.dataset.type !== e.target.value)));
  $("#send-form").addEventListener("submit", (e) => {
    e.preventDefault();
    const inst = $("#send-instance").value, number = $("#send-number").value.trim(), type = $("#send-type").value;
    const routes = {
      text: ["/send/text", { number, text: $("#send-text").value }],
      media: ["/send/media", { number, url: $("#send-url").value.trim(), type: $("#send-media-type").value, caption: $("#send-caption").value }],
      sticker: ["/send/sticker", { number, sticker: $("#send-sticker").value.trim() }],
      location: ["/send/location", { number, latitude: parseFloat($("#send-lat").value), longitude: parseFloat($("#send-lng").value), name: $("#send-place").value }],
      poll: ["/send/poll", { number, question: $("#send-question").value, options: $("#send-options").value.split("\n").map((s) => s.trim()).filter(Boolean), maxAnswer: 1 }],
    };
    const [path, body] = routes[type];
    run(e.submitter, async () => {
      const r = await api("POST", path, body, inst);
      $("#send-result").classList.remove("hidden");
      $("#send-result").textContent = JSON.stringify(r, null, 2);
      toast("Mensagem enviada");
    });
  });


  // ------------------------------------------------------------ chamadas (softphone)
  // O navegador manda a oferta WebRTC já com todos os candidatos (sem "trickle") e recebe a
  // resposta da BOB na mesma requisição. A oferta sempre leva uma linha de vídeo: a câmera
  // liga e desliga no meio da chamada sem renegociar.
  let callTimer = null, phone = null;
  const CALL_STATUS = { ringing: ["Tocando", "warn"], connected: ["Em andamento", "ok"], ended: ["Encerrada", ""], missed: ["Perdida", "bad"], rejected: ["Recusada", "bad"] };
  const callInst = () => $("#call-instance").value;
  const fmtNum = (c) => (c.number || c.remoteJid || "").replace(/@.*/, "");
  const fmtDur = (s) => (s >= 3600 ? Math.floor(s / 3600) + ":" : "") + String(Math.floor((s % 3600) / 60)).padStart(2, "0") + ":" + String(s % 60).padStart(2, "0");

  function stopCalls() { clearInterval(callTimer); callTimer = null; }
  async function openCalls() {
    fillSelect($("#call-instance"), false);
    await loadCallSettings();
    stopCalls();
    callTimer = setInterval(pollCalls, 2000);
    pollCalls();
    loadHistory();
  }
  $("#call-instance").addEventListener("change", () => { loadCallSettings(); pollCalls(); loadHistory(); });
  $("#call-filter").addEventListener("change", loadHistory);

  async function loadCallSettings() {
    const inst = callInst();
    if (!inst) { $("#call-state").textContent = "crie uma instância"; return; }
    const st = await api("GET", "/call/settings", undefined, inst).catch((e) => ({ error: e.message }));
    const pill = $("#call-state");
    if (st.error || !st.available) { pill.textContent = st.error || "indisponível no servidor"; pill.className = "pill bad"; return; }
    $("#call-enabled").checked = !!st.enabled;
    pill.textContent = st.active ? "recebendo chamadas" : st.enabled ? "ligadas (aguardando conexão)" : "desligadas";
    pill.className = "pill " + (st.active ? "ok" : st.enabled ? "warn" : "");
    if (st.rejectCall && st.enabled) { pill.textContent += " · recusa automática ativa"; pill.className = "pill warn"; }
  }
  $("#call-enabled").addEventListener("change", (e) => run(null, async () => {
    const r = await api("POST", "/call/settings", { enabled: e.target.checked }, callInst());
    toast(r.enabled ? "Chamadas ligadas" + (r.reconnected ? " (instância reconectada)" : "") : "Chamadas desligadas");
    setTimeout(loadCallSettings, 1500);
  }).finally(loadCallSettings));

  function callRow(c, live) {
    const [label, cls] = CALL_STATUS[c.status] || [c.status, ""];
    const kind = c.isVideo ? "📹" : "📞";
    const dir = c.fromMe ? "↗" : "↙";
    const when = new Date(c.startedAt * 1000).toLocaleString("pt-BR");
    let actions = "";
    if (live && c.status === "ringing" && !c.fromMe && !c.browser) {
      actions = `<button class="btn primary small" data-call="answer" data-id="${esc(c.callId)}">Atender</button>` +
        `<button class="btn small" data-call="answer-video" data-id="${esc(c.callId)}">Atender com vídeo</button>` +
        `<button class="btn danger small" data-call="reject" data-id="${esc(c.callId)}">Recusar</button>`;
    } else if (live) {
      actions = (phone && phone.id === c.callId ? "" : `<button class="btn small" data-call="transfer" data-id="${esc(c.callId)}">Puxar para este navegador</button>`) +
        `<button class="btn danger small" data-call="hangup" data-id="${esc(c.callId)}">Desligar</button>`;
    }
    const agent = c.agent ? ` · ${esc(c.agent)}` : "";
    return `<div class="call-row ${live && c.status === "ringing" ? "ringing" : ""}"><span>${kind} ${dir}</span><div class="who"><strong>${esc(fmtNum(c))}</strong><div class="muted small">${when}${agent}${c.durationSec ? " · " + fmtDur(c.durationSec) : ""}${c.reason ? " · " + esc(c.reason) : ""}</div></div><span class="pill ${cls}">${esc(label)}</span>${actions}</div>`;
  }

  async function pollCalls() {
    const inst = callInst();
    if (!inst) return;
    const r = await api("GET", "/call/live", undefined, inst).catch(() => null);
    if (!r) return;
    const list = r.calls || [];
    $("#call-live").innerHTML = list.length ? list.map((c) => callRow(c, true)).join("") : '<p class="muted">Nenhuma chamada em andamento.</p>';
    if (phone) {
      const mine = list.find((c) => c.callId === phone.id);
      if (mine) phoneStatus(mine);
      else if (phone.id) { endPhone("Chamada encerrada"); loadHistory(); }
    }
  }

  async function loadHistory() {
    const inst = callInst();
    if (!inst) return;
    const st = $("#call-filter").value;
    const r = await api("GET", "/call/list?limit=30" + (st ? "&status=" + st : ""), undefined, inst).catch((e) => ({ error: e.message }));
    if (r.error) { $("#call-history").innerHTML = `<p class="error">${esc(r.error)}</p>`; return; }
    $("#call-history").innerHTML = r.calls.length ? r.calls.map((c) => callRow(c, false)).join("") : '<p class="muted">Nenhuma chamada registrada ainda.</p>';
  }

  // Monta o RTCPeerConnection, espera os candidatos e devolve a oferta pronta.
  async function preparePhone(video) {
    if (!window.RTCPeerConnection || !navigator.mediaDevices) throw new Error("Este navegador não faz chamadas WebRTC (use HTTPS ou localhost).");
    if (phone) throw new Error("Já existe uma chamada neste navegador.");
    const ice = await api("GET", "/call/ice-servers?agent=painel", undefined, callInst());
    const stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true }, video: video ? { width: 640, height: 480 } : false });
    const pc = new RTCPeerConnection({ iceServers: ice.iceServers || [] });
    const p = { pc, stream, id: "", muted: false, camera: !!video, since: 0 };
    phone = p;
    pc.addTrack(stream.getAudioTracks()[0], stream);
    p.videoSender = pc.addTransceiver("video", { direction: "sendrecv" }).sender;
    if (video) await p.videoSender.replaceTrack(stream.getVideoTracks()[0]);
    pc.ontrack = (e) => {
      if (e.track.kind === "audio") $("#phone-audio").srcObject = new MediaStream([e.track]);
      else $("#phone-remote").srcObject = new MediaStream([e.track]);
    };
    pc.onconnectionstatechange = () => { if (pc.connectionState === "failed") toast("A conexão de áudio falhou (confira o TURN no servidor)", true); };
    $("#phone-local").srcObject = video ? stream : null;
    await pc.setLocalDescription(await pc.createOffer());
    await new Promise((ok) => {
      if (pc.iceGatheringState === "complete") return ok();
      const t = setTimeout(ok, 4000);
      pc.addEventListener("icegatheringstatechange", () => { if (pc.iceGatheringState === "complete") { clearTimeout(t); ok(); } });
    });
    return pc.localDescription.sdp;
  }

  async function connectPhone(path, body) {
    try {
      const r = await api("POST", path, body, callInst());
      phone.id = r.call.callId;
      await phone.pc.setRemoteDescription({ type: "answer", sdp: r.sdp });
      $("#phone").classList.remove("hidden");
      phoneStatus(r.call);
      return r;
    } catch (e) {
      endPhone();
      throw e;
    }
  }

  function phoneStatus(c) {
    $("#phone-peer").textContent = fmtNum(c);
    const [label, cls] = CALL_STATUS[c.status] || [c.status, ""];
    $("#phone-status").textContent = label;
    $("#phone-status").className = "pill " + cls;
    if (c.status === "connected" && !phone.since) phone.since = c.answeredAt ? c.answeredAt * 1000 : Date.now();
    $("#phone-timer").textContent = phone.since ? fmtDur(Math.max(0, Math.round((Date.now() - phone.since) / 1000))) : "";
    $("#phone-mute").className = "btn " + (phone.muted ? "off" : "on");
    $("#phone-camera").className = "btn " + (phone.camera ? "on" : "");
  }

  function endPhone(msg) {
    if (!phone) return;
    phone.stream.getTracks().forEach((t) => t.stop());
    phone.pc.close();
    phone = null;
    $("#phone").classList.add("hidden");
    $("#phone-remote").srcObject = null;
    $("#phone-local").srcObject = null;
    if (msg) toast(msg);
  }

  $("#call-form").addEventListener("submit", (e) => {
    e.preventDefault();
    const video = $("#call-video").checked;
    run(e.submitter, async () => {
      const sdp = await preparePhone(video);
      await connectPhone("/call/start", { number: $("#call-number").value.trim(), video, sdp, agent: "painel" });
      toast("Chamando…");
    });
  });

  $("#page-chamadas").addEventListener("click", (e) => {
    const b = e.target.closest("[data-call]");
    if (!b) return;
    const id = b.dataset.id, act = b.dataset.call;
    run(b, async () => {
      if (act === "answer" || act === "answer-video") {
        const sdp = await preparePhone(act === "answer-video");
        await connectPhone("/call/answer", { callId: id, sdp, agent: "painel" });
        if (act === "answer-video") await api("POST", "/call/camera", { callId: id, on: true }, callInst()).catch(() => {});
      } else if (act === "transfer") {
        const sdp = await preparePhone(false);
        await connectPhone("/call/transfer", { callId: id, sdp, agent: "painel" });
        toast("Chamada neste navegador");
      } else if (act === "reject") {
        await api("POST", "/call/reject", { callId: id }, callInst());
      } else if (act === "hangup") {
        await api("POST", "/call/hangup", { callId: id }, callInst());
      }
      pollCalls();
    });
  });

  $("#phone-hangup").addEventListener("click", (e) => run(e.target, async () => {
    if (phone && phone.id) await api("POST", "/call/hangup", { callId: phone.id }, callInst()).catch(() => {});
    endPhone("Chamada encerrada");
    loadHistory();
  }));
  $("#phone-mute").addEventListener("click", (e) => run(e.target, async () => {
    if (!phone) return;
    phone.muted = !phone.muted;
    phone.stream.getAudioTracks().forEach((t) => (t.enabled = !phone.muted));
    await api("POST", "/call/mute", { callId: phone.id, muted: phone.muted }, callInst());
    $("#phone-mute").className = "btn " + (phone.muted ? "off" : "on");
  }));
  $("#phone-camera").addEventListener("click", (e) => run(e.target, async () => {
    if (!phone) return;
    const on = !phone.camera;
    if (on) {
      let track = phone.stream.getVideoTracks()[0];
      if (!track) {
        const cam = await navigator.mediaDevices.getUserMedia({ video: { width: 640, height: 480 } });
        track = cam.getVideoTracks()[0];
        phone.stream.addTrack(track);
      }
      await phone.videoSender.replaceTrack(track);
      $("#phone-local").srcObject = phone.stream;
    } else {
      await phone.videoSender.replaceTrack(null);
      $("#phone-local").srcObject = null;
    }
    await api("POST", "/call/camera", { callId: phone.id, on }, callInst());
    phone.camera = on;
    $("#phone-camera").className = "btn " + (on ? "on" : "");
  }));

  // ------------------------------------------------------------ eventos ao vivo
  let ws = null, paused = false;
  function closeSocket() { if (ws) { ws.onclose = null; ws.close(); ws = null; } }
  function openSocket() {
    fillSelect($("#events-instance"), true);
    closeSocket();
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const inst = $("#events-instance").value;
    ws = new WebSocket(`${proto}://${location.host}/ws?apikey=${encodeURIComponent(KEY)}${inst ? "&instance=" + encodeURIComponent(inst) : ""}`);
    const st = $("#events-state");
    ws.onopen = () => { st.textContent = "ao vivo"; st.className = "pill ok"; };
    ws.onclose = () => { st.textContent = "desconectado"; st.className = "pill bad"; setTimeout(() => location.hash === "#eventos" && openSocket(), 3000); };
    ws.onmessage = (m) => {
      if (paused) return;
      let ev; try { ev = JSON.parse(m.data); } catch { return; }
      const box = $("#events");
      if (box.querySelector("p.muted")) box.innerHTML = "";
      const el = document.createElement("details");
      el.className = "event";
      el.innerHTML = `<summary><span class="tag">${esc(ev.event)}</span><strong>${esc(ev.instanceName)}</strong><span class="muted">${new Date(ev.date_time || Date.now()).toLocaleTimeString("pt-BR")}</span></summary><pre>${esc(JSON.stringify(ev.data, null, 2))}</pre>`;
      box.prepend(el);
      while (box.children.length > 200) box.lastChild.remove();
    };
  }
  $("#events-instance").addEventListener("change", openSocket);
  $("#events-toggle").addEventListener("click", (e) => { paused = !paused; e.target.textContent = paused ? "Continuar" : "Pausar"; });
  $("#events-clear").addEventListener("click", () => ($("#events").innerHTML = '<p class="muted">Aguardando eventos…</p>'));

  // ------------------------------------------------------------ servidor e Pix
  async function loadServer() {
    const i = await api("GET", "/server/info").catch((e) => ({ error: e.message }));
    if (i.error) { $("#server").innerHTML = `<p class="error">${esc(i.error)}</p>`; return; }
    const card = (t, rows) => `<div class="card"><h3>${t}</h3><div class="inst-meta">${rows.map(([k, v]) => `<span>${k}</span><span>${esc(v)}</span>`).join("")}</div></div>`;
    $("#server").innerHTML =
      card("API WhatsApp BOB", [["Versão", i.version], ["Commit", i.commit ? i.commit.slice(0, 12) : "—"], ["Motor", i.engine], ["Go", i.go]]) +
      card("Funcionamento", [["No ar há", i.uptime], ["Iniciada em", new Date(i.startedAt).toLocaleString("pt-BR")], ["Memória", i.memoryMB + " MB"], ["Banco", i.database]]) +
      card("Uso", [["Instâncias", i.instances], ["Conectadas", i.connected], ["WebSockets abertos", i.websockets], ["ffmpeg (áudio)", i.ffmpeg ? "instalado" : "não instalado"]]) +
      `<div class="card"><h3>Links</h3><p><a href="/swagger/index.html" target="_blank">Documentação interativa (Swagger)</a></p><p><a href="/openapi.yaml" target="_blank">Especificação OpenAPI</a></p><p><a href="https://github.com/Edton180/API-Whatsapp-BOB-" target="_blank" rel="noopener">Repositório no GitHub</a></p></div>`;
  }
  async function loadPix() {
    const p = await fetch("/donate/pix").then((r) => r.json()).catch(() => null);
    if (p && p.data) $("#pix-code").value = p.data.copia_e_cola;
  }
  $("#pix-copy").addEventListener("click", () => navigator.clipboard.writeText($("#pix-code").value).then(() => toast("Código Pix copiado")));

  // ------------------------------------------------------------ início
  if (KEY) {
    api("GET", "/server/info").then(start).catch(showLogin);
  } else {
    showLogin();
  }
})();
