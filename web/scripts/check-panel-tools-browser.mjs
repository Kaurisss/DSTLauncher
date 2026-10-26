// All write requests use isolated HTTP fixtures, never live game data.
import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let vite, browser, origin;
before(async () => {
  vite = await createServer({ root: fileURLToPath(new URL('..', import.meta.url)), server: { host: '127.0.0.1', port: 0 } });
  await vite.listen();
  origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => { await browser?.close(); await vite?.close(); });

function room(id) {
  return { id, name: `试验房间${id}`, key: `Cluster_${id}`, gameMode: 'endless', language: 'zh', maxPlayers: 12,
    maxSnapshots: 10, tickRate: 30, masterIp: '127.0.0.1', offline: true, lanOnly: true, needToken: false,
    shards: [{ name: 'Surface', isMaster: true, worldType: 'forest', gameId: 1 }, { name: 'Below', worldType: 'cave', gameId: 2 }],
    statuses: [{ shard: 'Surface', state: 'running' }, { shard: 'Below', state: 'stopped' }],
  };
}
async function openPanel({ tab = 'overview', width = 1280, stopped = false, dark = false } = {}) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on('pageerror', err => errors.push(err.message));
  await page.addInitScript(({ tab, dark }) => {
    localStorage.setItem('dst-tab', tab);
    localStorage.setItem('dst-room', '2');
    localStorage.setItem('dst-theme', dark ? 'dark' : 'light');
  }, { tab, dark });
  const rooms = [room(2), room(3)];
  if (stopped) rooms.forEach(r => { r.statuses.forEach(s => { s.state = 'stopped'; }); });
  const state = { writes: [], reads: [], lists: { 2: { blocklist: ['KU_existing-1'], whitelist: ['KU_reserved_2'] }, 3: { blocklist: [], whitelist: [] } },
    failList: false, quickResult: null, chatError: false, pending: [] };
  state.hold = name => {
    let release;
    const promise = new Promise(resolve => { release = resolve; });
    const deferred = { promise, release };
    state[name] = deferred;
    state.pending.push(deferred);
    return deferred;
  };
  await page.route('**/api/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname, method = req.method();
    const id = Number(url.searchParams.get('room') || 2);
    const ok = data => route.fulfill({ json: { code: 200, data } });
    const fail = message => route.fulfill({ json: { code: 500, message } });
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok(rooms);
    if (path === '/api/rooms/meta') return ok({ gameModes: ['endless'], languages: ['zh'], tickRates: [30], defaults: room(0) });
    if (path === '/api/status') return ok(rooms.find(r => r.id === id)?.statuses || []);
    if (path === '/api/host') return ok({ memory: { totalBytes: 16e9, availableBytes: 12e9 }, disk: { totalBytes: 100e9, availableBytes: 80e9 } });
    if (path === '/api/server' || path === '/api/install') return ok({ installed: true, candidates: [] });
    if (path === '/api/players') return ok({ players: [], errors: {} });
    if (path === '/api/admins') return ok({ admins: [], file: '/fixture/adminlist.txt', needRestart: true });
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    if (path === '/api/player-lists') {
      const kind = url.searchParams.get('kind'), ids = [...state.lists[id][kind]];
      if (method === 'GET') {
        state.reads.push({ path, id, kind });
        if (state.delayList) await state.delayList.promise;
        if (state.failList) return fail('名单暂不可读');
      } else {
        const body = req.postDataJSON();
        state.writes.push({ path, id, kind, method, body });
        const normalized = body.kleiId.replace(/^ku_/i, 'KU_');
        if (method === 'POST' && !ids.includes(normalized)) ids.push(normalized);
        if (method === 'DELETE') ids.splice(ids.indexOf(normalized), 1);
        state.lists[id][kind] = ids;
      }
      return ok({ ids, file: `/fixture/Cluster_${id}/${kind}.txt`, needRestart: true, reservedSlots: state.lists[id].whitelist.length });
    }
    if (path === '/api/quick-action') {
      const body = req.postDataJSON();
      state.writes.push({ path, id, method, body });
      if (state.delayQuick) await state.delayQuick.promise;
      return ok(state.quickResult || { action: body.action, shards: ['Surface'], skipped: ['Below'], errors: {} });
    }
    if (path === '/api/chat-log') {
      const shard = url.searchParams.get('shard'), kind = url.searchParams.get('kind'), search = url.searchParams.get('search');
      state.reads.push({ path, id, shard, kind, search });
      if (state.delayChat) await state.delayChat.promise;
      if (state.chatError) return fail('聊天日志暂不可读');
      const rows = [{ offset: 100, time: '00:01:02', kind: 'chat', userid: 'KU_chat-1', name: `房间${id}-${shard}`, message: '<img src=x onerror="window.fixtureInjected=true"> 你好世界' },
        { offset: 200, time: '00:01:03', kind: 'join', name: '加入玩家', message: '进入游戏' }];
      return ok({ entries: rows.filter(row => (kind === 'all' || kind === row.kind) && (!search || `${row.name} ${row.userid} ${row.message}`.includes(search))), exists: true, truncated: true });
    }
    return ok([]);
  });
  await page.goto(origin);
  await page.getByRole('heading', { name: tab === 'rooms' ? '房间列表' : '快捷操作', exact: true }).waitFor();
  return { page, state, close: async () => {
    state.pending.forEach(d => d.release());
    await page.close();
    assert.deepEqual(errors, [], 'no unhandled browser errors');
  } };
}
async function edit(page) {
  await page.getByRole('article').filter({ has: page.getByRole('heading', { name: '试验房间2', exact: true }) }).getByRole('button', { name: '编辑', exact: true }).click();
  await page.getByRole('heading', { name: '玩家准入名单', exact: true }).waitFor();
}
async function selectRoom(page, id) {
  await page.locator('#roomSelect').click();
  await page.getByRole('option', { name: `试验房间${id}`, exact: true }).click();
}
const quickWrites = state => state.writes.filter(r => r.path === '/api/quick-action');

test('名单校验兼容短横线与下划线，按编辑房间写入，移除需要确认', async () => {
  const { page, state, close } = await openPanel({ tab: 'rooms' });
  try {
    await edit(page);
    const black = page.getByRole('region', { name: '黑名单', exact: true });
    await black.getByText('KU_existing-1', { exact: true }).waitFor();
    await black.getByRole('textbox', { name: '黑名单 Klei ID' }).fill('not valid');
    await black.getByRole('button', { name: '添加到黑名单' }).click();
    assert.equal(state.writes.length, 0);
    await black.getByRole('textbox', { name: '黑名单 Klei ID' }).fill('ku_new-_1');
    await black.getByRole('button', { name: '添加到黑名单' }).click();
    await black.getByText('KU_new-_1', { exact: true }).waitFor();
    await page.getByRole('button', { name: '白名单', exact: true }).click();
    const white = page.getByRole('region', { name: '白名单', exact: true });
    await white.getByText('KU_reserved_2', { exact: true }).waitFor();
    await selectRoom(page, 3);
    await white.getByRole('textbox', { name: '白名单 Klei ID' }).fill('KU_second');
    await white.getByRole('button', { name: '添加到白名单' }).click();
    await white.getByText('KU_second', { exact: true }).waitFor();
    assert.equal(state.writes.at(-1).id, 2);
    assert.equal(state.lists[3].whitelist.length, 0);
    await white.getByRole('button', { name: '从白名单移除 KU_second' }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(state.writes.length, 2);
    await white.getByRole('button', { name: '从白名单移除 KU_second' }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await white.getByText('KU_second', { exact: true }).waitFor({ state: 'hidden' });
    assert.equal(state.writes.at(-1).method, 'DELETE');
  } finally { await close(); }
});

test('名单读取失败可重试，迟到的旧类型响应不能覆盖新类型', async () => {
  const { page, state, close } = await openPanel({ tab: 'rooms' });
  try {
    state.failList = true;
    await edit(page);
    await page.getByRole('alert').getByText(/名单暂不可读/).waitFor();
    state.failList = false;
    const old = state.hold('delayList');
    const sent = page.waitForRequest(r => r.url().includes('/api/player-lists'));
    await page.getByRole('button', { name: '刷新名单' }).click();
    await sent;
    state.delayList = null;
    await page.getByRole('button', { name: '白名单', exact: true }).click();
    const white = page.getByRole('region', { name: '白名单', exact: true });
    await white.getByText('KU_reserved_2', { exact: true }).waitFor();
    const arrived = page.waitForResponse(r => r.url().includes('kind=blocklist'));
    old.release(); await arrived;
    assert.equal(await white.getByText('KU_existing-1', { exact: true }).count(), 0);
  } finally { await close(); }
});

test('快捷保存如实报告部分失败；公告只提交一次且绑定原房间', async () => {
  const { page, state, close } = await openPanel();
  try {
    state.quickResult = { action: 'save', shards: ['Surface'], skipped: [], errors: { Below: '存档写入失败' } };
    await page.getByRole('button', { name: '立即保存', exact: true }).click();
    const panel = page.getByRole('region', { name: '快捷操作', exact: true });
    await panel.getByText(/存档写入失败/).waitFor();
    assert.deepEqual(quickWrites(state)[0].body, { action: 'save' });
    state.quickResult = null;
    const text = '欢迎 "quoted" \\ 中文 😀';
    await page.getByRole('textbox', { name: '全服公告', exact: true }).fill(text);
    const pending = state.hold('delayQuick');
    const sent = page.waitForRequest(r => r.url().includes('/api/quick-action'));
    await page.getByRole('button', { name: '发送全服公告', exact: true }).click();
    await sent;
    assert.equal(await page.getByRole('textbox', { name: '全服公告', exact: true }).isDisabled(), true);
    await selectRoom(page, 3);
    const arrived = page.waitForResponse(r => r.url().includes('/api/quick-action'));
    pending.release(); await arrived;
    await page.getByRole('button', { name: '发送全服公告', exact: true }).waitFor();
    assert.equal(quickWrites(state).length, 2);
    assert.equal(quickWrites(state)[1].id, 2);
    assert.equal(quickWrites(state)[1].body.message, text);
    assert.equal(await panel.getByText(/公告已提交/).count(), 0);
  } finally { await close(); }
});

test('快进需确认，取消不请求；主分片未运行时禁用操作', async () => {
  const { page, state, close } = await openPanel();
  try {
    await page.getByRole('button', { name: '主世界快进一天', exact: true }).click();
    await page.getByRole('alertdialog').getByText(/不能撤销/).waitFor();
    await page.getByRole('alertdialog').getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(quickWrites(state).length, 0);
    await page.getByRole('button', { name: '主世界快进一天', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await page.getByRole('region', { name: '快捷操作', exact: true }).getByText(/已执行快进/).waitFor();
    assert.deepEqual(quickWrites(state)[0].body, { action: 'skipday' });
  } finally { await close(); }
  const other = await openPanel({ stopped: true });
  try { for (const name of ['立即保存', '主世界快进一天', '发送全服公告']) assert.equal(await other.page.getByRole('button', { name, exact: true }).isDisabled(), true); }
  finally { await other.close(); }
});

test('玩家日志筛选框铺满字段宽度，菜单左对齐且选项留有内边距', async () => {
  for (const width of [1280, 375]) {
    const { page, close } = await openPanel({ width });
    try {
      if (width < 768) await page.getByRole('button', { name: '切换导航', exact: true }).click();
      const nav = page.locator(width < 768 ? '[data-mobile="true"]' : '#app-navigation');
      await nav.getByRole('button', { name: '玩家日志', exact: true }).click();
      const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
      await panel.getByText('房间2-Surface', { exact: true }).waitFor();
      for (const name of ['记录分片', '记录类型']) {
        const trigger = panel.getByRole('combobox', { name, exact: true });
        const field = await trigger.evaluate(el => {
          const box = el.getBoundingClientRect();
          const parent = el.closest('.field');
          return { width: box.width, fieldWidth: parent.getBoundingClientRect().width,
            labelBottom: parent.querySelector('label').getBoundingClientRect().bottom, top: box.top };
        });
        assert.ok(Math.abs(field.width - field.fieldWidth) < 1, `${name}应铺满字段宽度`);
        assert.ok(field.labelBottom <= field.top, `${name}标签应在控件上方`);
        const anchor = await trigger.boundingBox();
        await trigger.click();
        const menu = page.getByRole('listbox');
        await menu.waitFor();
        await menu.evaluate(async el => { await Promise.all(el.getAnimations().map(animation => animation.finished)); });
        const bounds = await menu.boundingBox();
        const option = await menu.getByRole('option').first().boundingBox();
        assert.ok(Math.abs(bounds.x - anchor.x) < 1, `${name}菜单应与控件左对齐`);
        assert.ok(option.x >= bounds.x + 3, `${name}选项不应贴住菜单边缘`);
        await page.keyboard.press('Escape');
      }
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    } finally { await close(); }
  }
});

test('聊天筛选按房间与分片隔离，转义文本，忽略旧响应，离开后停止轮询', async () => {
  const { page, state, close } = await openPanel();
  try {
    await page.locator('#app-navigation').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    await panel.getByText('房间2-Surface', { exact: true }).waitFor();
    assert.equal(await page.evaluate(() => !!window.fixtureInjected), false);
    assert.equal(await panel.locator('img').count(), 0);
    await panel.getByRole('combobox', { name: '记录类型', exact: true }).click();
    await page.getByRole('option', { name: '聊天', exact: true }).click();
    await panel.getByText('加入玩家', { exact: true }).waitFor({ state: 'hidden' });
    await panel.getByRole('textbox', { name: '搜索最近记录', exact: true }).fill('KU_chat-1');
    await panel.getByText('房间2-Surface', { exact: true }).waitFor();
    assert.equal(state.reads.at(-1).search, 'KU_chat-1');
    const old = state.hold('delayChat');
    const sent = page.waitForRequest(r => r.url().includes('/api/chat-log'));
    await panel.getByRole('button', { name: '刷新记录', exact: true }).click();
    await sent;
    state.delayChat = null;
    await selectRoom(page, 3);
    await panel.getByText('房间3-Surface', { exact: true }).waitFor();
    const arrived = page.waitForResponse(r => r.url().includes('/api/chat-log') && r.url().includes('room=2'));
    old.release(); await arrived;
    assert.equal(await panel.getByText('房间2-Surface', { exact: true }).count(), 0);
    const before = state.reads.length;
    await page.locator('#app-navigation').getByRole('button', { name: '概览', exact: true }).click();
    await page.waitForTimeout(5200);
    assert.equal(state.reads.length, before);
  } finally { await close(); }
});

test('窄屏深色主题无横向溢出，聊天读取错误可刷新恢复', async () => {
  const { page, state, close } = await openPanel({ width: 375, dark: true });
  try {
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    state.chatError = true;
    await page.getByRole('button', { name: '切换导航', exact: true }).click();
    await page.locator('[data-mobile="true"]').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    await panel.getByRole('alert').waitFor();
    state.chatError = false;
    await panel.getByRole('button', { name: '刷新记录', exact: true }).click();
    await panel.getByText('房间2-Surface', { exact: true }).waitFor();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
  } finally { await close(); }
});
