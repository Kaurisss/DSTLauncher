// All write requests use isolated HTTP fixtures, never live game data.
import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';
import { CHARACTERS } from '../src/lib/format.js';

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
        if (state.delayListWrite) await state.delayListWrite.promise;
        if (state.failListWrite) return fail('黑名单写入失败');
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
      const key = `${id}/${shard}`;
      const rows = state.chatEntries || [{ id: 1, offset: 100, time: '00:01:02', kind: 'chat', userid: 'KU_chat-1', name: `房间${id}-${shard}`, message: '<img src=x onerror="window.fixtureInjected=true"> 你好世界',
        prefab: 'wilson', steamid: '76561198000000001', ip: '192.0.2.1', recordedAt: '2026-01-02T03:04:05Z', collectedAt: '2026-02-03T04:05:06Z' },
        { id: 2, offset: 200, time: '00:01:03', kind: 'join', name: '加入玩家', message: '进入游戏', collectedAt: '2026-02-03T04:05:06Z' }];
      state.deletedChat ||= {};
      const removed = state.deletedChat[key] ||= [];
      if (method === 'DELETE') {
        assert.ok(url.searchParams.has('room') && shard, '写入必须指定房间与分片');
        const body = req.postDataJSON();
        state.writes.push({ path, id, shard, method, body });
        if (state.delayChatWrite) await state.delayChatWrite.promise;
        if (state.failChatWrite) return fail('历史删除失败');
        const ids = body.ids.filter(value => rows.some(row => row.id === value) && !removed.includes(value));
        removed.push(...ids);
        return ok({ deleted: ids.length });
      }
      const page = Number(url.searchParams.get('page')), limit = Number(url.searchParams.get('limit'));
      assert.ok(page >= 1 && limit >= 1 && limit <= 500, '显式使用服务端分页');
      state.reads.push({ path, id, shard, kind, search, page, limit });
      if (state.delayChat) await state.delayChat.promise;
      if (state.chatError) return fail('聊天日志暂不可读');
      const filtered = rows.filter(row => !removed.includes(row.id) && (kind === 'all' || kind === row.kind) && (!search || `${row.name} ${row.userid} ${row.message}`.includes(search)));
      return ok({ entries: filtered.slice((page - 1) * limit, page * limit), total: filtered.length, page, pageSize: limit, exists: true, truncated: false, modified: '2026-02-03T04:05:06Z' });
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
    assert.equal(await panel.locator('img:not([src^="/character-avatars/"])').count(), 0);
    await panel.getByRole('combobox', { name: '记录类型', exact: true }).click();
    await page.getByRole('option', { name: '聊天', exact: true }).click();
    await panel.getByText('加入玩家', { exact: true }).waitFor({ state: 'hidden' });
    await panel.getByRole('textbox', { name: '搜索历史记录', exact: true }).fill('KU_chat-1');
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

test('玩家日志表格分页、调整条数、筛选及重置保持正确记录范围', async () => {
  const { page, state, close } = await openPanel();
  try {
    state.chatEntries = Array.from({ length: 45 }, (_, i) => ({
      id: i + 1, offset: i, time: '00:01:02', kind: 'chat', userid: `KU_test-${i}`, name: `日志玩家${i}`, message: `消息 ${i}`,
    }));
    await page.locator('#app-navigation').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    const table = panel.getByRole('table', { name: '游戏聊天记录' });
    await table.getByText('日志玩家0', { exact: true }).waitFor();
    assert.equal(await table.locator('tbody tr').count(), 10);
    await panel.getByRole('button', { name: '下一页', exact: true }).click();
    await table.getByText('日志玩家10', { exact: true }).waitFor();
    assert.equal(state.reads.at(-1).page, 2);
    assert.equal(state.reads.at(-1).limit, 10);
    await panel.getByText('第 11–20 条，共 45 条', { exact: true }).waitFor();
    await panel.getByRole('combobox', { name: '每页日志条数', exact: true }).click();
    await page.getByRole('option', { name: '20 条 / 页', exact: true }).click();
    await table.getByText('日志玩家0', { exact: true }).waitFor();
    assert.equal(await table.locator('tbody tr').count(), 20);
    assert.equal(state.reads.at(-1).page, 1);
    assert.equal(state.reads.at(-1).limit, 20);
    await panel.getByRole('button', { name: '下一页', exact: true }).click();
    await table.getByText('日志玩家20', { exact: true }).waitFor();
    await panel.getByRole('textbox', { name: '搜索历史记录' }).fill('日志玩家42');
    await table.getByText('日志玩家42', { exact: true }).waitFor();
    assert.equal(await table.locator('tbody tr').count(), 1);
    assert.equal(await panel.getByRole('button', { name: '下一页', exact: true }).isDisabled(), true);
    await panel.getByRole('button', { name: '重置筛选' }).click();
    await table.getByText('日志玩家0', { exact: true }).waitFor();
    await panel.getByRole('button', { name: '下一页', exact: true }).click();
    await panel.getByRole('combobox', { name: '记录分片', exact: true }).click();
    await page.getByRole('option', { name: 'Below', exact: true }).click();
    await table.getByText('日志玩家0', { exact: true }).waitFor();
    assert.equal(state.reads.at(-1).shard, 'Below');
    assert.equal(await panel.getByRole('button', { name: '上一页', exact: true }).isDisabled(), true);
  } finally { await close(); }
});

test('玩家身份、事件与事件时间如实显示，Steam 链接仅接受数字', async () => {
  const { page, state, close } = await openPanel();
  try {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    state.chatEntries = [
      { id: 71, offset: 0, kind: 'death', name: '死亡玩家', prefab: 'wilson', steamid: '76561198000000001', ip: '192.0.2.7', time: '00:02:03', recordedAt: '2026-01-02T03:04:05Z', collectedAt: '2026-02-03T04:05:06Z', message: '被猎犬击杀' },
      { id: 72, offset: 20, kind: 'resurrect', prefab: 'modded_hero', steamid: 'javascript:alert(1)', time: '00:02:04', collectedAt: '2026-02-03T04:05:06Z', message: '复活事件' },
      { id: 73, offset: 30, kind: 'join', name: '等待选角', time: '00:02:05', message: '进入游戏' },
    ];
    await page.locator('#app-navigation').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    const death = panel.getByRole('row').filter({ hasText: '死亡玩家' });
    const portrait = death.getByRole('img', { name: '威尔逊', exact: true });
    await portrait.waitFor();
    assert.equal(await portrait.getAttribute('src'), '/character-avatars/wilson.png');
    await portrait.evaluate(image => image.decode());
    await page.evaluate(async prefabs => {
      await Promise.all(prefabs.map(prefab => {
        const avatar = new window.Image();
        avatar.src = `/character-avatars/${prefab}.png`;
        return avatar.decode();
      }));
    }, [...Object.keys(CHARACTERS), 'unknown']);
    await portrait.focus();
    await page.getByRole('tooltip', { name: '威尔逊', exact: true }).waitFor();
    await page.keyboard.press('Escape');
    await death.getByText('死亡', { exact: true }).waitFor();
    await death.getByText('192.0.2.7', { exact: true }).waitFor();
    assert.equal(await death.locator('time').getAttribute('datetime'), '2026-01-02T03:04:05Z');
    const link = death.getByRole('link');
    assert.equal(await link.getAttribute('href'), 'https://steamcommunity.com/profiles/76561198000000001');
    assert.equal(await link.getAttribute('target'), '_blank');
    assert.equal(await link.getAttribute('rel'), 'noopener noreferrer');
    await death.getByRole('button', { name: '复制 Steam ID 76561198000000001' }).click();
    await page.getByText('Steam ID 已复制', { exact: true }).waitFor();
    assert.equal(await page.evaluate(() => window.navigator.clipboard.readText()), '76561198000000001');
    const revived = panel.getByRole('row').filter({ hasText: '复活事件' });
    await revived.getByText('复活', { exact: true }).waitFor();
    const unknown = revived.getByRole('img', { name: 'modded_hero', exact: true });
    assert.equal(await unknown.getAttribute('src'), '/character-avatars/unknown.png');
    await unknown.evaluate(image => image.decode());
    assert.equal(await panel.getByRole('row').filter({ hasText: '等待选角' }).getByRole('img').count(), 0);
    await revived.getByText('运行时长 00:02:04', { exact: true }).waitFor();
    assert.equal(await revived.getByRole('link').count(), 0);
    assert.equal(await revived.locator('time').count(), 0);
    assert.equal(await panel.locator('time[datetime="2026-02-03T04:05:06Z"]').count(), 0);
    for (const [label, kind] of [['死亡', 'death'], ['复活', 'resurrect']]) {
      await panel.getByRole('combobox', { name: '记录类型', exact: true }).click();
      await page.getByRole('option', { name: label, exact: true }).click();
      await panel.getByRole('table').getByText(label, { exact: true }).waitFor();
      assert.equal(state.reads.at(-1).kind, kind);
    }
  } finally { await close(); }
});

test('玩家历史删除需确认、失败保留选择、批量删除后校正越界页', async () => {
  const { page, state, close } = await openPanel();
  try {
    state.chatEntries = Array.from({ length: 12 }, (_, i) => ({ id: i + 1, offset: i, kind: 'chat', time: '00:01:02', name: `删除玩家${i}`, message: '历史' }));
    await page.locator('#app-navigation').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    await panel.getByRole('checkbox', { name: '选择日志 1', exact: true }).click();
    await panel.getByRole('button', { name: '删除所选（1）', exact: true }).waitFor();
    await panel.getByRole('button', { name: '下一页', exact: true }).click();
    await panel.getByText('删除玩家10', { exact: true }).waitFor();
    assert.equal(await panel.getByRole('button', { name: '删除所选（0）', exact: true }).isDisabled(), true);
    await panel.getByRole('button', { name: '删除日志 11', exact: true }).click();
    const dialog = page.getByRole('alertdialog');
    await dialog.getByText(/只删面板历史、不删游戏原始日志/).waitFor();
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(state.writes.length, 0);
    await panel.getByRole('checkbox', { name: '选择本页日志', exact: true }).click();
    state.failChatWrite = true;
    await panel.getByRole('button', { name: '删除所选（2）', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await page.getByText('历史删除失败', { exact: true }).waitFor();
    assert.equal(await panel.getByRole('checkbox', { name: '选择日志 11', exact: true }).isChecked(), true);
    state.failChatWrite = false;
    await panel.getByRole('button', { name: '删除所选（2）', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await panel.getByText('删除玩家0', { exact: true }).waitFor();
    await panel.getByText('第 1–10 条，共 10 条', { exact: true }).waitFor();
    assert.equal(state.reads.at(-1).page, 1);
    assert.deepEqual(state.writes.at(-1).body, { ids: [11, 12] });
    assert.equal(state.writes.at(-1).id, 2);
    assert.equal(state.writes.at(-1).shard, 'Surface');
    await panel.getByRole('button', { name: '删除日志 1', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await panel.getByText('第 1–9 条，共 9 条', { exact: true }).waitFor();
    assert.deepEqual(state.writes.at(-1).body, { ids: [1] });
  } finally { await close(); }
});

test('删除等待期间禁用操作，切房间或筛选不携带选择且迟到结果不刷新新查询', async () => {
  const { page, state, close } = await openPanel();
  try {
    await page.locator('#app-navigation').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    await panel.getByRole('checkbox', { name: '选择日志 1', exact: true }).click();
    await panel.getByRole('textbox', { name: '搜索历史记录', exact: true }).fill('你好');
    await panel.getByRole('button', { name: '删除所选（0）', exact: true }).waitFor();
    await panel.getByRole('checkbox', { name: '选择日志 1', exact: true }).click();
    await panel.getByRole('combobox', { name: '记录分片', exact: true }).click();
    await page.getByRole('option', { name: 'Below', exact: true }).click();
    await panel.getByText('房间2-Below', { exact: true }).waitFor();
    assert.equal(await panel.getByRole('checkbox', { name: '选择日志 1', exact: true }).isChecked(), false);
    await panel.getByRole('switch', { name: '自动刷新玩家日志' }).click();
    const pending = state.hold('delayChatWrite');
    await panel.getByRole('button', { name: '删除日志 1', exact: true }).click();
    const sent = page.waitForRequest(r => r.url().includes('/api/chat-log') && r.method() === 'DELETE');
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await sent;
    assert.equal(await panel.getByRole('button', { name: '删除日志 1', exact: true }).isDisabled(), true);
    assert.equal(await panel.getByRole('checkbox', { name: '选择日志 1', exact: true }).isDisabled(), true);
    await selectRoom(page, 3);
    await panel.getByText('房间3-Surface', { exact: true }).waitFor();
    const before = state.reads.length;
    const arrived = page.waitForResponse(r => r.url().includes('/api/chat-log') && r.request().method() === 'DELETE');
    pending.release(); await arrived;
    await panel.getByRole('button', { name: '删除日志 1', exact: true }).waitFor({ state: 'visible' });
    assert.equal(state.reads.length, before);
    assert.equal(await panel.getByRole('checkbox', { name: '选择日志 1', exact: true }).isChecked(), false);
    assert.deepEqual(state.deletedChat['2/Below'], [1]);
    assert.deepEqual(state.deletedChat['3/Surface'], []);
  } finally { await close(); }
});

test('自动刷新可暂停，暂停期间仍可手动刷新，恢复后立即取新记录', async () => {
  const { page, state, close } = await openPanel();
  try {
    await page.locator('#app-navigation').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    await panel.getByText('房间2-Surface', { exact: true }).waitFor();
    const toggle = page.getByRole('switch', { name: '自动刷新玩家日志' });
    await toggle.click();
    const response = page.waitForResponse(r => r.url().includes('/api/chat-log'));
    await page.getByRole('button', { name: '刷新记录', exact: true }).click();
    await response;
    const count = state.reads.length;
    await page.waitForTimeout(5200);
    assert.equal(state.reads.length, count);
    const resumed = page.waitForRequest(r => r.url().includes('/api/chat-log'));
    await toggle.click();
    await resumed;
    assert.equal(await toggle.getAttribute('aria-checked'), 'true');
    assert.equal(state.reads.length, count + 1);
  } finally { await close(); }
});

test('日志 ID 可复制，加入黑名单需确认，失败可重试且迟到结果不串房间', async () => {
  const { page, state, close } = await openPanel();
  try {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.locator('#app-navigation').getByRole('button', { name: '玩家日志', exact: true }).click();
    const panel = page.getByRole('region', { name: '聊天与进出记录', exact: true });
    const player = panel.getByRole('row').filter({ hasText: 'KU_chat-1' });
    await player.getByRole('button', { name: '复制 Klei ID KU_chat-1' }).click();
    await page.getByText('Klei ID 已复制', { exact: true }).waitFor();
    assert.equal(await page.evaluate(() => window.navigator.clipboard.readText()), 'KU_chat-1');
    assert.equal(await panel.getByRole('row').filter({ hasText: '加入玩家' }).getByRole('button', { name: '加入黑名单', exact: true }).count(), 0);
    await player.getByRole('button', { name: '加入黑名单', exact: true }).click();
    const dialog = page.getByRole('alertdialog');
    await dialog.getByText(/不会立即踢出在线玩家/).waitFor();
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(state.writes.length, 0);
    state.failListWrite = true;
    await player.getByRole('button', { name: '加入黑名单', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await page.getByText('黑名单写入失败', { exact: true }).waitFor();
    state.failListWrite = false;
    const pending = state.hold('delayListWrite');
    await player.getByRole('button', { name: '加入黑名单', exact: true }).click();
    const sent = page.waitForRequest(r => r.url().includes('/api/player-lists') && r.method() === 'POST');
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await sent;
    assert.equal(await player.getByRole('button', { name: '处理中…', exact: true }).isDisabled(), true);
    await selectRoom(page, 3);
    await panel.getByText('房间3-Surface', { exact: true }).waitFor();
    const arrived = page.waitForResponse(r => r.url().includes('/api/player-lists'));
    pending.release(); await arrived;
    assert.equal(await panel.getByRole('button', { name: '已加入黑名单', exact: true }).count(), 0);
    assert.equal(state.lists[2].blocklist.includes('KU_chat-1'), true);
    assert.equal(state.lists[3].blocklist.includes('KU_chat-1'), false);
    assert.deepEqual(state.writes.map(({ id, kind, method, body }) => ({ id, kind, method, body })), [
      { id: 2, kind: 'blocklist', method: 'POST', body: { kleiId: 'KU_chat-1' } },
      { id: 2, kind: 'blocklist', method: 'POST', body: { kleiId: 'KU_chat-1' } },
    ]);
    await panel.getByRole('button', { name: '加入黑名单', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await panel.getByRole('button', { name: '已加入黑名单', exact: true }).waitFor();
    assert.equal(await panel.getByRole('button', { name: '已加入黑名单', exact: true }).isDisabled(), true);
    assert.equal(state.writes.at(-1).id, 3);
    await page.evaluate(() => Object.defineProperty(window.navigator, 'clipboard', { value: undefined, configurable: true }));
    await panel.getByRole('button', { name: '复制 Klei ID KU_chat-1' }).click();
    await page.getByText('当前连接不支持剪贴板访问，请选中 ID 手动复制', { exact: true }).waitFor();
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
