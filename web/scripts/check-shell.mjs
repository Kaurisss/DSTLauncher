// 外壳回归：真实 Pinia/主题/SSE 模块，仅替换浏览器存储与 HTTP 边界。
import assert from 'node:assert/strict';
import { after, beforeEach, mock, test } from 'node:test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { runInNewContext } from 'node:vm';
import { setTimeout as wait } from 'node:timers/promises';
import { createPinia, setActivePinia } from 'pinia';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createServer } from 'vite';

const saved = new Map([['dst-theme', 'dark']]);
const mediaListeners = new Set();
const media = {
  matches: false,
  addEventListener: (_, fn) => mediaListeners.add(fn),
};
globalThis.localStorage = {
  getItem: key => saved.get(key) ?? null,
  setItem: (key, value) => saved.set(key, String(value)),
  removeItem: key => saved.delete(key),
};
globalThis.window = { matchMedia: () => media };
globalThis.location = { origin: 'http://fixtures.invalid' };
globalThis.document = { documentElement: { dataset: {} } };
const sources = [];
globalThis.EventSource = class {
  constructor(url) { this.url = url; this.closed = false; sources.push(this); }
  close() { this.closed = true; }
  emit(message) { this.onmessage?.({ data: JSON.stringify({ type: 'log', message }) }); }
};

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)),
  server: { middlewareMode: true, hmr: false },
});
after(() => vite.close());
const { useRoomsStore } = await vite.ssrLoadModule('/src/stores/rooms.js');
const { useUiStore } = await vite.ssrLoadModule('/src/stores/ui.js');
const { procApi, roomApi } = await vite.ssrLoadModule('/src/lib/api.js');
const { useLogs } = await vite.ssrLoadModule('/src/composables/useLogs.js');
const themeModule = await vite.ssrLoadModule('/src/composables/useTheme.js');
const { default: Banner } = await vite.ssrLoadModule('/src/components/CurrentRoomBanner.vue');
const logs = useLogs();
let pinia;
beforeEach(() => {
  mock.restoreAll();
  saved.delete('dst-room');
  saved.delete('dst-tab');
  logs.disconnect();
  logs.clearAll();
  sources.length = 0;
  pinia = createPinia();
  setActivePinia(pinia);
});
after(() => { logs.disconnect(); logs.clearAll(); });

function roomsStore() {
  const rooms = useRoomsStore(pinia);
  rooms.list = [1, 2].map(id => ({
    id, name: `房间 ${id}`, key: `Cluster_${id}`, gameMode: 'endless',
    shards: [{ name: 'Master', isMaster: true }],
  }));
  rooms.currentId = 1;
  return rooms;
}
function deferred() {
  let resolve;
  const promise = new Promise(r => { resolve = r; });
  return { promise, resolve };
}
const running = [{ shard: 'Master', state: 'running' }];
const stopped = [{ shard: 'Master', state: 'stopped' }];

test('切房间立即清空旧状态，迟到响应不会覆盖新房间', async () => {
  const rooms = roomsStore();
  const a = deferred();
  const b = deferred();
  mock.method(procApi, 'status', id => id === 1 ? a.promise : b.promise);
  rooms.statuses = running;
  const first = rooms.refreshStatus();
  rooms.select('2');
  assert.deepEqual(rooms.statuses, []);
  const second = rooms.refreshStatus();
  b.resolve(stopped);
  await second;
  a.resolve(running);
  await first;
  assert.deepEqual(rooms.statuses, stopped);
  assert.equal(rooms.running, false);
  assert.deepEqual(rooms.current.statuses, stopped);
});

test('切回原房间后，上一轮同房间请求仍不能覆盖最新状态', async () => {
  const rooms = roomsStore();
  const old = deferred();
  const fresh = deferred();
  let count = 0;
  mock.method(procApi, 'status', () => ++count === 1 ? old.promise : fresh.promise);
  const first = rooms.refreshStatus();
  rooms.select(2);
  rooms.select(1);
  const second = rooms.refreshStatus();
  fresh.resolve(stopped);
  await second;
  old.resolve(running);
  await first;
  assert.deepEqual(rooms.statuses, stopped);
});

test('同房间慢轮询重叠时仍及时展示状态，再应用较新的响应', async () => {
  const rooms = roomsStore();
  const old = deferred();
  const fresh = deferred();
  let count = 0;
  mock.method(procApi, 'status', () => ++count === 1 ? old.promise : fresh.promise);
  const first = rooms.refreshStatus();
  const second = rooms.refreshStatus();
  old.resolve(running);
  await first;
  assert.deepEqual(rooms.statuses, running);
  fresh.resolve(stopped);
  await second;
  assert.deepEqual(rooms.statuses, stopped);
});

test('房间被清空后，旧状态请求不能复活状态', async () => {
  const rooms = roomsStore();
  const response = deferred();
  mock.method(procApi, 'status', () => response.promise);
  const pending = rooms.refreshStatus();
  rooms.list = [];
  rooms.currentId = null;
  await rooms.refreshStatus();
  response.resolve(running);
  await pending;
  assert.deepEqual(rooms.statuses, []);
});

test('房间选择持久化，列表轮询保留对象身份与当前选择', async () => {
  const rooms = roomsStore();
  rooms.select('2');
  const current = rooms.current;
  assert.equal(saved.get('dst-room'), '2');
  mock.method(roomApi, 'list', async () => rooms.list.map(r => ({ ...r })));
  await rooms.load();
  assert.equal(rooms.current, current);
  const refreshed = useRoomsStore(createPinia());
  assert.equal(refreshed.currentId, 2);
  assert.equal(rooms.select('不存在'), false);
  assert.equal(rooms.currentId, 2);
});

test('七个页面使用同一导航清单，选择持久化且拒绝未知页面', async () => {
  const ui = useUiStore(pinia);
  assert.equal(ui.tabs.length, 7);
  for (const tab of ui.tabs) ui.select(tab.key);
  await Promise.resolve();
  assert.equal(ui.activeTab, 'config');
  assert.equal(saved.get('dst-tab'), 'config');
  ui.select('not-a-page');
  assert.equal(ui.activeTab, 'config');
  assert.equal(useUiStore(createPinia()).activeTab, 'config');
});

test('三态主题与首屏脚本一致，跟随系统并仅注册一次监听', () => {
  const theme = themeModule.useTheme();
  themeModule.initTheme();
  themeModule.initTheme();
  assert.equal(mediaListeners.size, 1);
  assert.equal(theme.mode.value, 'dark');
  assert.equal(document.documentElement.dataset.theme, 'dark');
  const html = readFileSync(new URL('../index.html', import.meta.url), 'utf8');
  const script = html.match(/<script>([\s\S]*?)<\/script>/)[1];
  for (const mode of ['auto', 'light', 'dark']) {
    theme.cycle();
    assert.equal(theme.mode.value, mode);
    assert.equal(saved.get('dst-theme'), mode);
    for (const dark of [false, true]) {
      media.matches = dark;
      for (const fn of mediaListeners) fn();
      const firstPaint = { documentElement: { dataset: {} } };
      runInNewContext(script, { window, localStorage, document: firstPaint });
      assert.equal(document.documentElement.dataset.theme, firstPaint.documentElement.dataset.theme);
      assert.equal(theme.resolved.value, mode === 'auto' ? (dark ? 'dark' : 'light') : mode);
    }
  }
});

test('当前房间标识不展示上一个房间的运行状态', async () => {
  const rooms = roomsStore();
  rooms.statuses = running;
  rooms.select(2);
  const html = await renderToString(createSSRApp(Banner).use(pinia));
  assert.ok(html.includes('房间 2'));
  assert.ok(html.includes('Cluster_2'));
  assert.ok(!html.includes('运行中'));
  assert.ok(html.includes('等待状态'));
});

test('后端成功返回空分片列表时显示已停止，换房后才重新等待', async () => {
  const rooms = roomsStore();
  mock.method(procApi, 'status', async () => []);
  await rooms.refreshStatus();
  const html = await renderToString(createSSRApp(Banner).use(pinia));
  assert.ok(html.includes('已停止'));
  assert.ok(!html.includes('等待状态'));
  rooms.select(2);
  const pending = await renderToString(createSSRApp(Banner).use(pinia));
  assert.ok(pending.includes('等待状态'));
});

test('SSE 重复订阅不重连，增删分片仅更新差异，换房才重建', () => {
  logs.openRoom(1, ['Master']);
  logs.openRoom(1, ['Master']);
  assert.equal(sources.length, 1);
  const master = sources[0];
  logs.openRoom(1, ['Master', 'Caves']);
  assert.equal(sources.length, 2);
  assert.equal(master.closed, false);
  logs.openRoom(1, ['Master']);
  assert.equal(sources[1].closed, true);
  assert.equal(master.closed, false);
  logs.clearAll();
  logs.openRoom(2, ['Master']);
  assert.equal(master.closed, true);
  assert.equal(sources.length, 3);
  assert.match(sources[2].url, /room=2/);
  sources[2].onopen();
  assert.equal(logs.connected.value, true);
  logs.disconnect();
  assert.equal(logs.connected.value, false);
});

test('日志按批落账、分片隔离，达到上限后仍保留最新4000行', async () => {
  logs.openRoom(1, ['Master', 'Caves']);
  for (let i = 0; i < 4005; i++) sources[0].emit(`Master ${i}`);
  sources[1].emit('Caves only');
  assert.equal(logs.channels.Master.lines.length, 0);
  await wait(150);
  assert.equal(logs.channels.Master.lines.length, 4000);
  assert.equal(logs.channels.Master.lines[0].text, 'Master 5');
  assert.equal(logs.channels.Master.lines.at(-1).text, 'Master 4004');
  assert.deepEqual(logs.channels.Caves.lines.map(l => l.text), ['Caves only']);
});

test('清屏和切房清理同时丢弃待落账日志', async () => {
  logs.openRoom(1, ['Master']);
  sources[0].emit('old pending');
  logs.clear('Master');
  await wait(150);
  assert.deepEqual(logs.channels.Master.lines, []);
  sources[0].emit('old room');
  logs.clearAll();
  logs.openRoom(2, ['Master']);
  sources.at(-1).emit('new room');
  await wait(150);
  assert.deepEqual(logs.channels.Master.lines.map(l => l.text), ['new room']);
});
