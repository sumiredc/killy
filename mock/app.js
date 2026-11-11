// Killy mock. The agents, the web server and SSE are simulated here.
// One battle screen covers every registered server: the parents of all online servers
// form a single queue, and each one shows which server it lives on.
const HOLD_MS = 1000;
const MAX_ZOMBIES = 6;
const COMPACT_AT = 5;

const I18N = {
  ja: {
    lockDate: '11月10日 火曜日',
    now: 'たった今',
    pushText: 'zombie ×{n} / parent: {name} (PID {pid}) on {host}',
    tapToOpen: '通知をタップして開く',
    counter: '親プロセス {i} / {n}',
    zLabel: '体のゾンビプロセス',
    whoParent: '親 PID {pid}',
    whoKilled: 'PID {pid} を kill しました',
    whoReaped: 'ゾンビ {n} 体は init に回収されました',
    hold: '長押しで kill',
    killSub: 'SIGKILL → PID {pid} on {host}',
    clearTitle: 'All clear',
    clearText: 'すべてのサーバーのゾンビが成仏しました',
    statParents: 'kill した親',
    statZombies: '成仏したゾンビ',
    close: '閉じる',
    emptyNoneTitle: 'ゾンビプロセスはいません',
    emptyNoneText: 'ゾンビプロセスは見つかっていません。見つかったら通知します。',
    emptyNoServerTitle: 'サーバーがありません',
    emptyNoServerText: '設定からサーバーを追加すると、監視が始まります。',
    settings: '設定',
    tabBattle: '撃退',
    servers: 'サーバー',
    addServer: 'サーバーを追加',
    app: 'アプリ',
    language: '言語',
    notifications: '通知',
    notificationsHint: 'ゾンビプロセスが見つかったら知らせる',
    rowZombies: 'ゾンビプロセス {n} / 親 {p}',
    rowClean: 'ゾンビプロセスなし',
    rowOffline: '最新の状態を取得できていません',
    noServers: 'サーバーがまだありません。追加すると、ゾンビプロセスの監視が始まります。',
    addTitle: 'サーバーを追加',
    addStep1: '監視したいサーバーで、次のコマンドを実行します',
    addStep2: '表示された 6 文字のペアリングコードを入力します',
    codeLabel: 'ペアリングコード',
    scanQr: 'QR コードを読み取る',
    nameLabel: '表示名（省略可）',
    nameLabelEdit: '表示名',
    namePlaceholder: '省略するとサーバーのホスト名を使います',
    addSubmit: '追加する',
    cancel: 'キャンセル',
    pairing: '追加しています…',
    paired: '{name} を追加しました',
    pairError: 'このコードは見つからないか、有効期限（10 分）が切れています。サーバーで killy agent pair をもう一度実行してください。',
    detailTitle: 'サーバーの設定',
    hostLabel: 'アドレス',
    notifyServer: 'このサーバーの通知',
    connTitle: '接続',
    connCheck: '確認する',
    connRecheck: 'もう一度確認',
    connChecking: 'エージェントに問い合わせています…',
    connOk: '接続できました（応答 {ms} ms / agent {ver}）',
    connFail: 'エージェントから応答がありません。最後の受信は {min} 分前です。サーバーで killy agent が動いているか確認してください。',
    remove: 'このサーバーを削除',
    removeConfirm: 'もう一度押すと削除します',
    removeHint: '削除しても、サーバー上のエージェントは止まりません。',
    done: '完了',
    mockTitle: 'モック操作',
    mockNote: '本番アプリには含まれません',
    mockSpawn: '戦闘中の親にゾンビを 1 体増やす',
    mockTarget: '対象サーバー',
    mockAddParent: 'ゾンビを持つ親プロセスを追加',
    mockOffline: 'オンライン / オフライン切替',
    mockReset: 'すべてのサーバーを最初の状態に戻す',
    mockLock: '通知から始める',
    mockFailPair: '次のサーバー追加を失敗させる',
  },
  en: {
    lockDate: 'Tuesday, November 10',
    now: 'now',
    pushText: 'zombie ×{n} / parent: {name} (PID {pid}) on {host}',
    tapToOpen: 'Tap the notification to open',
    counter: 'parent process {i} of {n}',
    zLabel: 'zombie processes',
    whoParent: 'parent PID {pid}',
    whoKilled: 'PID {pid} killed',
    whoReaped: '{n} zombies reaped by init',
    hold: 'Hold to kill',
    killSub: 'SIGKILL → PID {pid} on {host}',
    clearTitle: 'All clear',
    clearText: 'Every zombie on every server is at rest',
    statParents: 'Parents killed',
    statZombies: 'Zombies released',
    close: 'Close',
    emptyNoneTitle: 'No zombie processes',
    emptyNoneText: 'No zombie processes found. You will be notified when one appears.',
    emptyNoServerTitle: 'No servers yet',
    emptyNoServerText: 'Add a server in Settings to start watching it.',
    settings: 'Settings',
    tabBattle: 'Battle',
    servers: 'Servers',
    addServer: 'Add server',
    app: 'App',
    language: 'Language',
    notifications: 'Notifications',
    notificationsHint: 'Alert me when zombie processes appear',
    rowZombies: '{n} zombie processes / {p} parents',
    rowClean: 'No zombie processes',
    rowOffline: 'Latest status unavailable',
    noServers: 'No servers yet. Add one to start watching for zombie processes.',
    addTitle: 'Add a server',
    addStep1: 'On the server you want to watch, run',
    addStep2: 'Enter the 6-character pairing code it shows',
    codeLabel: 'Pairing code',
    scanQr: 'Scan QR code',
    nameLabel: 'Display name (optional)',
    nameLabelEdit: 'Display name',
    namePlaceholder: "Uses the server's hostname if empty",
    addSubmit: 'Add',
    cancel: 'Cancel',
    pairing: 'Adding…',
    paired: 'Added {name}',
    pairError: 'This code was not found or has expired (10 min). Run killy agent pair on the server again.',
    detailTitle: 'Server settings',
    hostLabel: 'Address',
    notifyServer: 'Notifications for this server',
    connTitle: 'Connection',
    connCheck: 'Check',
    connRecheck: 'Check again',
    connChecking: 'Asking the agent…',
    connOk: 'Connected ({ms} ms / agent {ver})',
    connFail: 'No response from the agent. Last heard from {min} min ago. Check that killy agent is running on the server.',
    remove: 'Remove this server',
    removeConfirm: 'Tap again to remove',
    removeHint: 'Removing it does not stop the agent on the server.',
    done: 'Done',
    mockTitle: 'Mock controls',
    mockNote: 'Not part of the real app',
    mockSpawn: 'Add a zombie to the current parent',
    mockTarget: 'Target server',
    mockAddParent: 'Add a parent with zombies',
    mockOffline: 'Toggle online / offline',
    mockReset: 'Reset all servers',
    mockLock: 'Start from notification',
    mockFailPair: 'Make the next pairing fail',
  },
};

// ---------- Mock data ----------
const SEED = {
  s1: { name: 'demo-linux-01', host: '192.168.10.21', online: true, lastSeen: 0, notify: true,
    parents: [
      { pid: 3801, cmd: 'node evil_daemon.js', zombies: [4092, 4093, 4097] },
      { pid: 5120, cmd: 'python3 worker.py', zombies: [5133, 5140] },
    ] },
  s2: { name: 'prod-web-02', host: '10.0.3.14', online: true, lastSeen: 0, notify: true,
    parents: [{ pid: 2210, cmd: 'php-fpm: pool www', zombies: [2291] }] },
  s3: { name: 'build-runner', host: '10.0.5.7', online: false, lastSeen: 12, notify: false,
    parents: [{ pid: 9031, cmd: 'make -j8', zombies: [9044, 9045] }] },
};
const EXTRA_PARENTS = [
  { pid: 6204, cmd: 'ruby sidekiq.rb' },
  { pid: 7311, cmd: 'java -jar batch.jar' },
  { pid: 8150, cmd: 'bash cron-job.sh' },
];
const NEW_HOSTS = ['linux-vm-03', 'staging-api-01', 'batch-02', 'edge-proxy-04'];

const clone = (o) => JSON.parse(JSON.stringify(o));
const seedServers = () => Object.keys(SEED).map((id) => ({ id, ...clone(SEED[id]) }));

let servers = seedServers();
let lang = 'ja';
let appNotify = true;
let state = null;
let extraCount = 0;
let addedCount = 0;

const $ = (s) => document.querySelector(s);
const delay = (ms) => new Promise((r) => setTimeout(r, ms));
const randPid = (base) => base + Math.floor(Math.random() * 400);
const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

function t(key, params = {}) {
  return (I18N[lang][key] ?? key).replace(/\{(\w+)\}/g, (_, k) => params[k] ?? '');
}

const server = (id) => servers.find((s) => s.id === id);
const aliveParents = (s) => s.parents.filter((p) => !p.dead);
const zombieCount = (s) => aliveParents(s).reduce((n, p) => n + p.zombies.length, 0);
function current() { return state?.parents[state.index]; }

function applyI18n() {
  document.documentElement.lang = lang;
  document.querySelectorAll('[data-i18n]').forEach((el) => { el.textContent = t(el.dataset.i18n); });
  document.querySelectorAll('[data-i18n-label]').forEach((el) => el.setAttribute('aria-label', t(el.dataset.i18nLabel)));
  document.querySelectorAll('[data-i18n-placeholder]').forEach((el) => { el.placeholder = t(el.dataset.i18nPlaceholder); });
  document.querySelectorAll('#langSeg button').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.lang === lang)));
  renderPush();
  renderServerList();
  renderMockServers();
  if (state) renderBattle();
  if (!$('#serverSheet').hidden) renderRemoveBtn();
}

function renderPush() {
  const s = servers.find((x) => x.online && aliveParents(x).length);
  const p = s && aliveParents(s)[0];
  $('#pushText').textContent = p
    ? t('pushText', { n: p.zombies.length, name: p.cmd.split(' ')[0], pid: p.pid, host: s.name })
    : '';
}

// ---------- Killy: no words, only symbols and gestures ----------
let reactTimer;
function react(symbol, gesture, ms = 1400) {
  const killy = $('#killy');
  const bubble = $('#bubble');
  clearTimeout(reactTimer);
  killy.classList.remove('orooro', 'dash', 'hop', 'bow');
  void killy.offsetWidth;
  if (gesture) killy.classList.add(gesture);
  bubble.textContent = symbol || '';
  bubble.classList.toggle('show', Boolean(symbol));
  reactTimer = setTimeout(() => {
    bubble.classList.remove('show');
    if (gesture && gesture !== 'bow') killy.classList.remove(gesture);
  }, ms);
}

function flash() {
  const el = document.createElement('div');
  el.className = 'flash';
  $('#arena').append(el);
  setTimeout(() => el.remove(), 400);
}

function floatText(text) {
  const arena = $('#arena').getBoundingClientRect();
  const parent = $('#parent').getBoundingClientRect();
  const el = document.createElement('span');
  el.className = 'float';
  el.textContent = text;
  el.style.left = `${parent.left - arena.left - 10}px`;
  el.style.top = `${parent.top - arena.top - 10}px`;
  $('#arena').append(el);
  setTimeout(() => el.remove(), 1000);
}

// ---------- Battle rendering ----------
function emptyKind() {
  if (servers.length === 0) return 'noServer';
  if (!current() || (state.lastKill && state.index === state.parents.length - 1 && !state.busy)) return 'none';
  return null;
}

function renderBattle(entering = false) {
  const kind = emptyKind();
  $('#empty').hidden = !kind;
  $('#info').hidden = Boolean(kind);
  $('#party').hidden = Boolean(kind);
  if (kind) {
    $('#emptyTitle').textContent = t(kind === 'none' ? 'emptyNoneTitle' : 'emptyNoServerTitle');
    $('#emptyText').textContent = t(kind === 'none' ? 'emptyNoneText' : 'emptyNoServerText');
    $('#counter').innerHTML = '';
    $('#zombies').innerHTML = '';
    $('#killSub').textContent = '';
    $('#killBtn').disabled = true;
    return;
  }
  if (entering) $('#parent').className = 'parent enter';
  renderInfo(entering);
  if (!$('#zombies').children.length || entering) renderZombies();
  $('#killBtn').disabled = state.busy;
}

function renderInfo(entering = false) {
  const p = current();
  const srv = $('#srv');
  $('#srvName').textContent = p.server.name;
  if (entering && state.index > 0 && state.parents[state.index - 1].server !== p.server) {
    srv.classList.remove('changed');
    void srv.offsetWidth;
    srv.classList.add('changed');
  }
  $('#zCount').textContent = p.zombies.length;
  $('#zLabel').textContent = t('zLabel');

  // One hood per parent, like lives. From COMPACT_AT parents on, show one hood and "× remaining".
  const counter = $('#counter');
  const total = state.parents.length;
  if (total >= COMPACT_AT) {
    const remaining = total - state.index - (state.lastKill ? 1 : 0);
    counter.innerHTML = `<span class="hood current">${hoodIconSVG()}</span><span class="times">× ${remaining}</span>`;
  } else {
    counter.innerHTML = state.parents
      .map((_, i) => {
        const cls = i < state.index || (i === state.index && state.lastKill) ? 'done' : i === state.index ? 'current' : '';
        return `<span class="hood ${cls}">${hoodIconSVG()}</span>`;
      })
      .join('');
  }
  counter.setAttribute('aria-label', t('counter', { i: state.index + 1, n: total }));
  $('#killSub').textContent = t('killSub', { pid: p.pid, host: p.server.name });

  const who = $('#who');
  who.innerHTML = '';
  const b = document.createElement('b');
  const span = document.createElement('span');
  if (state.lastKill) {
    who.className = 'who done';
    b.textContent = t('whoKilled', { pid: state.lastKill.pid });
    span.textContent = t('whoReaped', { n: state.lastKill.n });
  } else {
    who.className = 'who';
    b.textContent = t('whoParent', { pid: p.pid });
    span.textContent = p.cmd;
  }
  who.append(b, span);
}

// Where each zombie stands on the ground: a front row, then a smaller back row.
// left is % of the arena width, bottom is px, s is the depth scale.
const SLOTS = [
  { left: 47, bottom: 40, s: 1 },
  { left: 63, bottom: 40, s: 1 },
  { left: 79, bottom: 40, s: 1 },
  { left: 55, bottom: 96, s: 0.84 },
  { left: 71, bottom: 96, s: 0.84 },
  { left: 87, bottom: 96, s: 0.84 },
];

function renderZombies(spawnedPid) {
  const box = $('#zombies');
  box.innerHTML = '';
  const p = current();
  if (!p) return;
  p.zombies.forEach((pid, i) => {
    const slot = SLOTS[i];
    const el = document.createElement('div');
    el.className = 'zombie' + (pid === spawnedPid ? ' spawn' : '');
    el.style.left = `${slot.left}%`;
    el.style.bottom = `${slot.bottom}px`;
    el.style.zIndex = i < 3 ? 9 : 5;
    el.style.setProperty('--s', slot.s);
    el.innerHTML = `<small>${pid}</small><div class="art body">${zombieSVG()}</div><div class="art soul">${soulSVG()}</div>`;
    box.append(el);
  });
}

// Marionette strings from the necromancer's control bar to each zombie's head and hand,
// redrawn every frame so they follow the motion.
function drawThreads() {
  const svg = $('#threads');
  if (!$('#battle').hidden && !$('#party').hidden) {
    const a = $('#arena').getBoundingClientRect();
    const center = (el) => {
      const r = el.getBoundingClientRect();
      return { x: r.left + r.width / 2 - a.left, y: r.top + r.height / 2 - a.top };
    };
    let d = '';
    if (!$('#parent').classList.contains('gone')) {
      const anchors = [...document.querySelectorAll('#parent .anchor')].map(center);
      const nearest = (pt, skip) => anchors
        .filter((x) => x !== skip)
        .reduce((best, x) => (Math.abs(x.x - pt.x) < Math.abs(best.x - pt.x) ? x : best));
      document.querySelectorAll('.zombie:not(.dead)').forEach((z) => {
        const head = center(z.querySelector('.head-pt'));
        const hand = center(z.querySelector('.hand'));
        const a1 = nearest(head);
        const a2 = nearest(hand, a1);
        d += `<line x1="${a1.x}" y1="${a1.y}" x2="${head.x}" y2="${head.y}"/>`;
        d += `<line x1="${a2.x}" y1="${a2.y}" x2="${hand.x}" y2="${hand.y}"/>`;
      });
    }
    svg.innerHTML = d;
  } else {
    svg.innerHTML = '';
  }
  requestAnimationFrame(drawThreads);
}

function renderClear() {
  $('#clearText').textContent = t('clearText');
  $('#statParents').textContent = state.killed;
  $('#statZombies').textContent = state.released;
}

function setBusy(busy) {
  state.busy = busy;
  $('#killBtn').disabled = busy;
  $('#tabSettings').disabled = busy;
}

// ---------- Battle flow ----------
// Builds the queue from every online server, in the order they are listed in Settings.
function openBattle() {
  const parents = servers
    .filter((s) => s.online)
    .flatMap((s) => aliveParents(s).map((p) => Object.assign(p, { server: s })));
  state = { parents, index: 0, busy: false, killed: 0, released: 0, lastKill: null };
  $('#clear').hidden = true;
  $('#killy').className = 'killy';
  $('#zombies').innerHTML = '';
  $('#threads').classList.remove('cut');
  setBusy(false);
  renderBattle(true);
  if (!emptyKind()) react('……！', 'orooro');
}

async function kill() {
  if (state.busy || emptyKind()) return;
  setBusy(true);
  const p = current();
  react('……！！', 'dash', 800);
  await delay(320);

  flash();
  floatText('SIGKILL');
  $('#threads').classList.add('cut');
  $('#parent').classList.add('hit');
  await delay(350);
  $('#parent').classList.remove('hit');

  // Strings cut: each puppet takes the hit, sags, and its soul rises.
  // The necromancer that held them fades only after they are gone.
  const zs = [...document.querySelectorAll('.zombie')];
  zs.forEach((el, i) => setTimeout(() => el.classList.add('dead', 'hit'), i * 70));
  await delay(380 + zs.length * 70);
  zs.forEach((el) => { el.classList.remove('hit'); el.classList.add('down'); });
  await delay(600);
  zs.forEach((el, i) => setTimeout(() => el.classList.add('release'), i * 130));
  await delay(1300 + zs.length * 130);
  $('#parent').classList.add('gone');
  await delay(900);

  const n = p.zombies.length;
  state.killed += 1;
  state.released += n;
  state.lastKill = { pid: p.pid, n };
  p.dead = true;
  p.zombies = [];
  $('#zombies').innerHTML = '';
  renderInfo();
  renderServerList();
  react('……！', 'hop', 1000);
  await delay(1300);

  if (state.index < state.parents.length - 1) {
    state.index += 1;
    state.lastKill = null;
    $('#threads').classList.remove('cut');
    setBusy(false);
    renderBattle(true);
    react('……！', 'orooro');
  } else {
    react('🤍', 'bow', 3000);
    await delay(600);
    renderClear();
    $('#clearKilly').innerHTML = killySVG();
    $('#clear').hidden = false;
    setBusy(false);
  }
}

// ---------- Hold to kill ----------
(function setupHold() {
  const btn = $('#killBtn');
  const fill = $('#killFill');
  const killy = $('#killy');
  let start = 0;
  let raf = 0;

  const setCharge = (p) => {
    fill.style.transform = `scaleX(${p})`;
    killy.style.setProperty('--charge', p);
    killy.classList.toggle('charging', p > 0);
  };
  const reset = () => {
    cancelAnimationFrame(raf);
    start = 0;
    setCharge(0);
  };
  const tick = (now) => {
    const p = Math.min(1, (now - start) / HOLD_MS);
    setCharge(p);
    if (p >= 1) {
      reset();
      kill();
    } else {
      raf = requestAnimationFrame(tick);
    }
  };
  btn.addEventListener('pointerdown', (e) => {
    if (btn.disabled) return;
    e.preventDefault();
    start = performance.now();
    raf = requestAnimationFrame(tick);
  });
  ['pointerup', 'pointerleave', 'pointercancel'].forEach((ev) => btn.addEventListener(ev, () => { if (start) reset(); }));
  btn.addEventListener('contextmenu', (e) => e.preventDefault());
})();

// ---------- Settings: server list ----------
function renderServerList() {
  renderTabBadge();
  const list = $('#serverList');
  list.innerHTML = '';
  $('#noServers').hidden = servers.length > 0;
  list.hidden = servers.length === 0;
  for (const s of servers) {
    const n = zombieCount(s);
    const status = !s.online
      ? t('rowOffline')
      : n > 0 ? t('rowZombies', { n, p: aliveParents(s).length }) : t('rowClean');
    const li = document.createElement('li');
    li.className = 'server' + (s.online ? '' : ' offline');
    li.innerHTML = `
      <button class="server-open" type="button" data-id="${s.id}">
        <span class="row-main">
          <span class="server-name">${esc(s.name)}</span>
          <span class="row-sub">${esc(status)}</span>
        </span>
        ${s.online && n > 0 ? `<span class="badge">${n}</span>` : ''}
        <span class="chev-r" aria-hidden="true">›</span>
      </button>`;
    list.append(li);
  }
}

// ---------- Footer tabs ----------
function showTab(name) {
  const settings = name === 'settings';
  $('#addSheet').hidden = true;
  $('#serverSheet').hidden = true;
  if (settings) renderServerList();
  $('#settings').hidden = !settings;
  $('#tabBattle').toggleAttribute('aria-current', !settings);
  $('#tabSettings').toggleAttribute('aria-current', settings);
  if (settings) {
    $('#tabSettings').setAttribute('aria-current', 'page');
  } else {
    $('#tabBattle').setAttribute('aria-current', 'page');
    if (state && !state.busy) renderBattle();
  }
  renderTabBadge();
}

// While on Settings, the Battle tab shows how many zombie processes are waiting.
function renderTabBadge() {
  const n = servers.filter((s) => s.online).reduce((sum, s) => sum + zombieCount(s), 0);
  const badge = $('#tabBadge');
  badge.textContent = n;
  badge.hidden = n === 0 || $('#settings').hidden;
}

$('#serverList').addEventListener('click', (e) => {
  const row = e.target.closest('.server-open');
  if (row) openServerSheet(row.dataset.id);
});

// ---------- Settings: add server ----------
function openAddSheet() {
  $('#addForm').hidden = false;
  $('#pairing').hidden = true;
  $('#paired').hidden = true;
  $('#pairError').hidden = true;
  $('#codeInput').value = '';
  $('#nameInput').value = '';
  $('#submitAddBtn').disabled = true;
  $('#addSheet').hidden = false;
}

$('#codeInput').addEventListener('input', (e) => {
  e.target.value = e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 6);
  $('#submitAddBtn').disabled = e.target.value.length !== 6;
  $('#pairError').hidden = true;
});

$('#scanBtn').addEventListener('click', () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  $('#codeInput').value = Array.from({ length: 6 }, () => chars[Math.floor(Math.random() * chars.length)]).join('');
  $('#codeInput').dispatchEvent(new Event('input'));
});

$('#submitAddBtn').addEventListener('click', async () => {
  $('#addForm').hidden = true;
  $('#pairing').hidden = false;
  await delay(1400);
  if ($('#mFailPair').checked) {
    $('#mFailPair').checked = false;
    $('#pairing').hidden = true;
    $('#addForm').hidden = false;
    $('#pairError').textContent = t('pairError');
    $('#pairError').hidden = false;
    return;
  }
  const name = $('#nameInput').value.trim() || NEW_HOSTS[addedCount % NEW_HOSTS.length];
  addedCount += 1;
  servers.push({
    id: `n${Date.now()}`,
    name,
    host: `10.0.8.${10 + Math.floor(Math.random() * 200)}`,
    online: true,
    lastSeen: 0,
    notify: true,
    parents: [],
  });
  $('#pairing').hidden = true;
  $('#pairedKilly').innerHTML = killySVG();
  $('#pairedText').textContent = t('paired', { name });
  $('#paired').hidden = false;
  renderServerList();
  renderMockServers();
  await delay(1300);
  $('#addSheet').hidden = true;
});

// ---------- Settings: one server ----------
let editingId = null;
let removeArmed = false;

function renderRemoveBtn() {
  const btn = $('#removeBtn');
  btn.textContent = removeArmed ? t('removeConfirm') : t('remove');
  btn.classList.toggle('confirm', removeArmed);
}

// Asks the server's agent for a reply, through the web server, and shows the result.
let checkSeq = 0;
function resetCheck() {
  checkSeq += 1;
  const r = $('#checkResult');
  r.className = 'row-sub';
  r.textContent = '';
  $('#checkBtn').disabled = false;
  $('#checkBtn').textContent = t('connCheck');
}

async function runCheck() {
  const seq = ++checkSeq;
  const s = server(editingId);
  const r = $('#checkResult');
  $('#checkBtn').disabled = true;
  r.className = 'row-sub checking';
  r.textContent = t('connChecking');
  await delay(1100);
  if (seq !== checkSeq || !s) return;
  if (s.online) {
    s.lastSeen = 0;
    r.className = 'row-sub ok';
    r.textContent = t('connOk', { ms: 20 + Math.floor(Math.random() * 60), ver: '0.1.0' });
  } else {
    r.className = 'row-sub fail';
    r.textContent = t('connFail', { min: s.lastSeen });
  }
  $('#checkBtn').disabled = false;
  $('#checkBtn').textContent = t('connRecheck');
}

$('#checkBtn').addEventListener('click', runCheck);

function openServerSheet(id) {
  const s = server(id);
  editingId = id;
  removeArmed = false;
  resetCheck();
  $('#editName').value = s.name;
  $('#editHost').textContent = s.host;
  $('#serverNotify').setAttribute('aria-checked', String(s.notify));
  renderRemoveBtn();
  $('#serverSheet').hidden = false;
}

function closeServerSheet() {
  checkSeq += 1;
  const s = server(editingId);
  if (s) s.name = $('#editName').value.trim() || s.name;
  $('#serverSheet').hidden = true;
  renderServerList();
  renderMockServers();
}

$('#serverNotify').addEventListener('click', (e) => {
  const s = server(editingId);
  s.notify = !s.notify;
  e.currentTarget.setAttribute('aria-checked', String(s.notify));
});

$('#removeBtn').addEventListener('click', () => {
  if (!removeArmed) {
    removeArmed = true;
    renderRemoveBtn();
    return;
  }
  servers = servers.filter((s) => s.id !== editingId);
  $('#serverSheet').hidden = true;
  renderServerList();
  renderMockServers();
  openBattle();
});

$('#doneBtn').addEventListener('click', closeServerSheet);
$('#serverSheet').addEventListener('click', (e) => { if (e.target === e.currentTarget) closeServerSheet(); });
$('#addSheet').addEventListener('click', (e) => { if (e.target === e.currentTarget) $('#addSheet').hidden = true; });

// ---------- Settings: app ----------
$('#langSeg').addEventListener('click', (e) => {
  const b = e.target.closest('button');
  if (!b) return;
  lang = b.dataset.lang;
  applyI18n();
});

$('#appNotify').addEventListener('click', (e) => {
  appNotify = !appNotify;
  e.currentTarget.setAttribute('aria-checked', String(appNotify));
});

// ---------- Mock-only controls ----------
function renderMockServers() {
  const sel = $('#mServer');
  const keep = sel.value;
  sel.innerHTML = servers.map((s) => `<option value="${s.id}">${esc(s.name)}${s.online ? '' : ' (offline)'}</option>`).join('');
  if (server(keep)) sel.value = keep;
}

function mockSpawn() {
  if (!state || state.busy || !$('#clear').hidden || emptyKind()) return;
  const p = current();
  if (p.zombies.length >= MAX_ZOMBIES) return;
  const child = randPid(p.pid + 300);
  p.zombies.push(child);
  renderZombies(child);
  renderInfo();
  renderServerList();
  react('……！', 'orooro');
}

function mockAddParent() {
  const s = server($('#mServer').value);
  if (!s || state?.busy || !$('#clear').hidden) return;
  const base = EXTRA_PARENTS[extraCount % EXTRA_PARENTS.length];
  extraCount += 1;
  const parent = { ...base, server: s, zombies: [randPid(base.pid + 10), randPid(base.pid + 500)] };
  s.parents.push(parent);
  renderServerList();
  if (!s.online) return;
  if (emptyKind()) {
    openBattle();
  } else {
    state.parents.push(parent);
    renderInfo();
  }
}

function mockToggleOffline() {
  const s = server($('#mServer').value);
  if (!s || state?.busy) return;
  s.online = !s.online;
  s.lastSeen = s.online ? 0 : 3;
  renderServerList();
  renderMockServers();
  openBattle();
}

function mockReset() {
  if (state?.busy) return;
  servers = seedServers();
  renderServerList();
  renderMockServers();
  openBattle();
}

$('#mSpawn').addEventListener('click', mockSpawn);
$('#mAddParent').addEventListener('click', mockAddParent);
$('#mOffline').addEventListener('click', mockToggleOffline);
$('#mReset').addEventListener('click', mockReset);
$('#mLock').addEventListener('click', showLock);

// ---------- Screens ----------
function showLock() {
  if (state?.busy) return;
  showTab('battle');
  $('#battle').hidden = true;
  $('#phone').classList.add('locked');
  renderPush();
  const lock = $('#lock');
  lock.hidden = false;
  lock.classList.remove('leaving');
}

function openFromPush() {
  $('#lock').classList.add('leaving');
  $('#phone').classList.remove('locked');
  $('#battle').hidden = false;
  showTab('battle');
  openBattle();
  setTimeout(() => { $('#lock').hidden = true; }, 350);
}

$('#push').addEventListener('click', openFromPush);
$('#tabBattle').addEventListener('click', () => showTab('battle'));
$('#tabSettings').addEventListener('click', () => showTab('settings'));
$('#addServerBtn').addEventListener('click', openAddSheet);
$('#cancelAddBtn').addEventListener('click', () => { $('#addSheet').hidden = true; });
$('#closeClearBtn').addEventListener('click', () => { $('#clear').hidden = true; $('#killy').className = 'killy'; renderBattle(); });

// ---------- Boot ----------
$('#parentArt').innerHTML = parentSVG();
$('#killyArt').innerHTML = killySVG();
$('#pushIcon').innerHTML = killySVG();
applyI18n();
requestAnimationFrame(drawThreads);
