// 运行真实 store 的自动安装→开服回归检查；仅替换 HTTP 边界，不下载游戏。
import assert from 'node:assert/strict';
import { after, beforeEach, mock, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createPinia, setActivePinia } from 'pinia';
import { createSSRApp, h } from 'vue';
import { toast as notify } from 'vue-sonner';
import { renderToString } from 'vue/server-renderer';
import { createServer } from 'vite';

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)),
  server: { middlewareMode: true, hmr: false },
});
after(() => vite.close());

const { useRoomsStore } = await vite.ssrLoadModule('/src/stores/rooms.js');
const { useSystemStore } = await vite.ssrLoadModule('/src/stores/system.js');
const { installApi, procApi } = await vite.ssrLoadModule('/src/lib/api.js');
const { default: InstallPanel } = await vite.ssrLoadModule('/src/components/overview/InstallPanel.vue');
const { default: UiCard } = await vite.ssrLoadModule('/src/components/ui/UiCard.vue');
const { guard } = await vite.ssrLoadModule('/src/lib/toast.js');

let pinia;
beforeEach(() => {
  mock.restoreAll();
  pinia = createPinia();
  setActivePinia(pinia);
});

function roomStore() {
  const rooms = useRoomsStore(pinia);
  rooms.list = [{ id: 1, name: '测试房间', needToken: false }];
  rooms.currentId = 1;
  return rooms;
}

test('未探测到已有安装时仍显示安装入口，不因空候选列表崩溃', async () => {
  for (const candidates of [null, []]) {
    useSystemStore(pinia).server = { candidates, archOptions: ['auto', '64', '32'] };
    const app = createSSRApp(InstallPanel).use(pinia);
    const html = await renderToString(app);
    assert.match(html, /未找到专用服务器/);
    assert.match(html, /仅安装服务器/);
    assert.match(html, /aria-haspopup="dialog"/);
  }
});

test('安装卡片显示各阶段百分比和对应进度条，未知进度不伪造数字', async () => {
  const system = useSystemStore(pinia);
  system.installing = true;
  for (const progress of ['预分配磁盘空间 0.00%', '预分配磁盘空间 35.27%', '下载服务器文件 2.50%', '下载服务器文件 100.00%', '校验服务器文件 4.05%']) {
    system.progress = progress;
    const html = await renderToString(createSSRApp(InstallPanel).use(pinia));
    const [stage, number] = progress.split(' ');
    const percent = Number.parseFloat(number);
    assert.ok(html.includes(stage));
    assert.ok(html.includes(`${percent}%`));
    assert.ok(html.includes('role="progressbar"'));
    assert.ok(html.includes(`aria-valuenow="${percent}"`));
  }
  system.progress = '正在连接 Steam…';
  const html = await renderToString(createSSRApp(InstallPanel).use(pinia));
  assert.ok(html.includes('正在连接 Steam…'));
  assert.ok(!html.includes('aria-valuenow='));
  assert.ok(!html.includes('>0%</'));
  assert.ok(!html.includes('>100%</'));
});

test('兼容卡片保留原有插槽、标题语义和无内边距模式', async () => {
  const html = await renderToString(createSSRApp({
    render: () => h(UiCard, { flush: true }, {
      title: () => '测试卡片',
      hint: () => '说明文字',
      actions: () => h('button', '测试操作'),
      default: () => '内容区域',
    }),
  }).use(pinia));
  for (const text of ['测试卡片', '说明文字', '测试操作', '内容区域']) assert.ok(html.includes(text));
  assert.match(html, /role="heading" aria-level="2"/);
  assert.match(html, /data-slot="card-content"[^>]*class="[^"]*\bp-0\b/);
});

test('结果提示换为 Sonner 后仍保留 guard 成功与错误契约', async () => {
  const success = mock.method(notify, 'success', () => 'success-toast');
  const error = mock.method(notify, 'error', () => 'error-toast');
  const good = await guard(async () => 42, { success: '安装任务已启动' });
  assert.deepEqual(good, { ok: true, data: 42 });
  assert.deepEqual(success.mock.calls[0].arguments, ['安装任务已启动', { duration: 3200 }]);
  const failure = new Error('测试下载失败');
  const bad = await guard(async () => { throw failure; });
  assert.equal(bad.ok, false);
  assert.equal(bad.error, failure);
  assert.deepEqual(error.mock.calls[0].arguments, ['测试下载失败', { duration: 6000 }]);
});

test('首次启动先安装，安装成功后才启动原分片', async () => {
  const calls = [];
  let installed = false;
  mock.method(installApi, 'status', async () => {
    calls.push('status');
    return { installed, installing: false };
  });
  mock.method(installApi, 'start', async () => {
    calls.push('install');
    installed = true;
  });
  mock.method(procApi, 'start', async (id, shard) => {
    assert.equal(id, 1);
    assert.equal(shard, 'Master');
    calls.push('start');
  });

  await roomStore().start('Master');
  assert.deepEqual(calls, ['status', 'install', 'status', 'start']);
});

test('已有可用安装直接开服，不重复下载', async () => {
  const status = mock.method(installApi, 'status', async () => ({ installed: true }));
  const install = mock.method(installApi, 'start', async () => {});
  const start = mock.method(procApi, 'start', async () => {});

  await roomStore().start('');
  assert.equal(status.mock.callCount(), 1);
  assert.equal(install.mock.callCount(), 0);
  assert.deepEqual(start.mock.calls[0].arguments, [1, '']);
});

test('已有安装任务时等待完成，不创建第二个任务', async () => {
  let checks = 0;
  mock.method(installApi, 'status', async () => ({
    installed: ++checks > 1, installing: checks === 1,
  }));
  const install = mock.method(installApi, 'start', async () => {});
  const start = mock.method(procApi, 'start', async () => {
    assert.ok(checks > 1, '下载结束前不应开服');
  });

  await roomStore().start('Master');
  assert.equal(install.mock.callCount(), 0);
  assert.equal(start.mock.callCount(), 1);
});

test('安装失败不启动，即使已下载出部分可执行文件', async () => {
  let attempted = false;
  mock.method(installApi, 'status', async () => attempted
    ? { installed: true, installing: false, error: 'SteamCMD 下载中断' }
    : { installed: false });
  mock.method(installApi, 'start', async () => { attempted = true; });
  const start = mock.method(procApi, 'start', async () => {});
  const rooms = roomStore();

  await assert.rejects(rooms.start('Master'), /SteamCMD 下载中断/);
  assert.equal(start.mock.callCount(), 0);
  assert.equal(rooms.starting, false);
  assert.equal(useSystemStore(pinia).error, 'SteamCMD 下载中断');
});

test('安装失败后再次点击会重试成功，不沿用旧错误', async () => {
  let attempts = 0;
  mock.method(installApi, 'status', async () => ({
    installed: attempts >= 1,
    error: attempts === 1 ? '下载中断' : '',
  }));
  mock.method(installApi, 'start', async () => { attempts++; });
  const start = mock.method(procApi, 'start', async () => {});
  const rooms = roomStore();

  await assert.rejects(rooms.start(''), /下载中断/);
  await rooms.start('');
  assert.equal(attempts, 2);
  assert.equal(start.mock.callCount(), 1);
});

test('安装期间切换房间和重复点击不会启动错房间或重复安装', async () => {
  let installed = false;
  const rooms = roomStore();
  rooms.list.push({ id: 2, name: '另一个房间', needToken: false });
  mock.method(installApi, 'status', async () => ({ installed }));
  const install = mock.method(installApi, 'start', async () => {
    rooms.select(2);
    installed = true;
  });
  const start = mock.method(procApi, 'start', async () => {});

  const pending = rooms.start('Master');
  assert.equal(rooms.starting, true);
  await assert.rejects(rooms.start('Caves'), /正在准备开服/);
  await pending;
  assert.equal(install.mock.callCount(), 1);
  assert.deepEqual(start.mock.calls[0].arguments, [1, 'Master']);
  assert.equal(rooms.currentId, 2);
  assert.equal(rooms.starting, false);
});

test('SteamCMD 结束但文件不存在时不启动', async () => {
  mock.method(installApi, 'status', async () => ({ installed: false }));
  mock.method(installApi, 'start', async () => {});
  const start = mock.method(procApi, 'start', async () => {});

  await assert.rejects(roomStore().start(''), /未找到服务器文件/);
  assert.equal(start.mock.callCount(), 0);
});

test('安装进度连接失败时终止自动开服，不永久等待', async () => {
  let attempted = false;
  mock.method(installApi, 'status', async () => {
    if (attempted) throw new Error('连接断开');
    return { installed: false };
  });
  mock.method(installApi, 'start', async () => { attempted = true; });
  const start = mock.method(procApi, 'start', async () => {});
  const rooms = roomStore();

  await assert.rejects(rooms.start(''), /连接断开/);
  assert.equal(start.mock.callCount(), 0);
  assert.equal(rooms.starting, false);
  assert.match(useSystemStore(pinia).error, /请刷新页面重试/);
});

test('在线房间缺令牌时先提示配置，不下载也不开服', async () => {
  const install = mock.method(installApi, 'start', async () => {});
  const start = mock.method(procApi, 'start', async () => {});
  const rooms = roomStore();
  rooms.current.needToken = true;

  await assert.rejects(rooms.start(''), /Klei 服务器令牌/);
  assert.equal(install.mock.callCount(), 0);
  assert.equal(start.mock.callCount(), 0);
});
