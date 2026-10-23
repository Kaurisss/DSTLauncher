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
const { confirmAction, confirmState, handleCancel, handleConfirm } = await vite.ssrLoadModule('/src/lib/confirm.js');
const { useProcessControl } = await vite.ssrLoadModule('/src/composables/useProcessControl.js');
const { default: Banner } = await vite.ssrLoadModule('/src/components/CurrentRoomBanner.vue');
const { default: RoomCard } = await vite.ssrLoadModule('/src/components/rooms/RoomCard.vue');
const { default: ShardList } = await vite.ssrLoadModule('/src/components/overview/ShardList.vue');
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

test('房间卡片使用整卡选择按钮，独立操作与分片信息保持可用', async () => {
  const room = {
    id: 2, name: '测试房间', key: 'Cluster_2', gameMode: 'survival',
    tickRate: 30, maxSnapshots: 10, hasSave: true, needToken: true,
    shards: [{ name: 'Master', serverPort: 10999, masterServerPort: 27018, authenticationPort: 8768 },
      { name: 'Caves', disabled: true }],
  };
  for (const selected of [false, true]) {
    const html = await renderToString(createSSRApp(RoomCard, { room, selected }));
    const selection = html.match(/<button\b[^>]*class="room-select"[^>]*>[\s\S]*?<\/button>/)?.[0];
    assert.ok(selection, '整卡选择使用原生按钮');
    assert.match(selection, /type="button"/);
    assert.match(selection, /aria-label="选择房间 测试房间"/);
    assert.ok(selection.includes(`aria-pressed="${selected}"`));
    assert.equal((html.match(/<button\b/g) || []).length, 4, '选择按钮与三个操作按钮为同级控件');
    assert.equal((selection.match(/<button\b/g) || []).length, 1, '禁止嵌套按钮');
    assert.doesNotMatch(html, />\s*选中\s*<\/button>/);
    for (const text of ['编辑', '上传存档', '删除', '10999 / 27018 / 8768', '未启用', 'Cluster_2', '缺令牌']) {
      assert.ok(html.includes(text), `保留 ${text}`);
    }
  }
});

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

test('回档绑定原房间，拦截并发请求与冷却期重复提交，拒绝非法次数', async () => {
  const rooms = roomsStore();
  const response = deferred();
  const send = mock.method(procApi, 'rollback', (id, count) => {
    assert.equal(id, 1);
    assert.equal(count, 2);
    return response.promise;
  });
  const task = rooms.rollback(2);
  rooms.select(2);
  await assert.rejects(rooms.rollback(1), /正在回档/);
  response.resolve({ requested: true, retryAfterSeconds: 60 });
  await task;
  assert.ok(rooms.rollbackDeadlines[1] > Date.now());
  assert.equal(rooms.rollbackDeadlines[2], undefined);
  rooms.select(1);
  await assert.rejects(rooms.rollback(1), /正在回档/);
  for (const n of [0, 1.5, 100]) await assert.rejects(rooms.rollback(n), /整数/);
  assert.equal(send.mock.calls.length, 1);
});
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

test('八个页面使用同一导航清单，选择持久化且拒绝未知页面', async () => {
  const ui = useUiStore(pinia);
  assert.equal(ui.tabs.length, 8);
  for (const tab of ui.tabs) ui.select(tab.key);
  await Promise.resolve();
  assert.equal(ui.activeTab, 'settings');
  assert.equal(saved.get('dst-tab'), 'settings');
  ui.select('not-a-page');
  assert.equal(ui.activeTab, 'settings');
  assert.equal(useUiStore(createPinia()).activeTab, 'settings');
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

test('只启动一个分片时，其余分片仍保留启动按钮', async () => {
  const rooms = useRoomsStore(pinia);
  rooms.list = [{
    id: 1, name: '房间 1', key: 'Cluster_1', gameMode: 'survival',
    shards: ['Master', 'Caves', 'Island', 'Volcano'].map(name => ({ name })),
  }];
  rooms.currentId = 1;
  // 后端只返回有进程记录的分片：只开了 Master
  rooms.statuses = [{ shard: 'Master', state: 'running', pid: 28924, port: 11002 }];

  const html = await renderToString(createSSRApp(ShardList).use(pinia));
  const rows = html.match(/class="shard-row"/g) || [];
  assert.equal(rows.length, 4, '每个启用分片都要有一行');
  for (const name of ['Master', 'Caves', 'Island', 'Volcano']) {
    assert.ok(html.includes(name), `缺少 ${name} 行`);
  }
  // Master 一个停止+重启，其余三个各一个启动（未装服务器时文案是「安装并启动」）。
  // SSR 会把文本节点包在 <!--[-->…<!--]--> 里，所以按「包含」数而不是结尾匹配。
  const buttons = html.match(/<button\b[^>]*>[\s\S]*?<\/button>/g) || [];
  assert.equal(buttons.length, 5, '每行都要有可操作按钮');
  const count = text => buttons.filter(b => b.includes(text)).length;
  assert.equal(count('停止'), 1);
  assert.equal(count('重启'), 1);
  assert.equal(count('启动'), 3, '其余分片要有启动按钮');
  assert.ok(html.includes('未启动'), '未启动的分片显示未启动');
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

test('通用确认弹窗支持异步取消与确认，危险动作为红，分片启停先走统一弹窗', async () => {
  const cancelPromise = confirmAction('删除「测试房间」？\n\n磁盘存档会保留。');
  assert.equal(confirmState.open, true);
  assert.equal(confirmState.title, '删除「测试房间」？');
  assert.equal(confirmState.description, '磁盘存档会保留。');
  assert.equal(confirmState.destructive, true);
  handleCancel();
  assert.equal(await cancelPromise, false);
  assert.equal(confirmState.open, false);

  const confirmPromise = confirmAction({ title: '确认更新', description: '拉取最新补丁' });
  assert.equal(confirmState.open, true);
  assert.equal(confirmState.destructive, false);
  handleConfirm();
  assert.equal(await confirmPromise, true);
  assert.equal(confirmState.open, false);

  const proc = useProcessControl();
  let stopCalled = false;
  mock.method(procApi, 'stop', async () => { stopCalled = true; return { ok: true }; });

  const stopPromise = proc.stop('Master');
  assert.equal(confirmState.open, true);
  assert.match(confirmState.title, /停止Master分片/);
  handleCancel();
  const stopOk = await stopPromise;
  assert.equal(stopOk, false);
  assert.equal(stopCalled, false);
});
