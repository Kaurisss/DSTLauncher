// A local SSE fixture exercises the real EventSource and log buffers; all game APIs are intercepted.
import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let vite;
let browser;
let origin;
const streams = new Set();
const opened = [];
before(async () => {
  vite = await createServer({
    root: fileURLToPath(new URL('..', import.meta.url)),
    server: { host: '127.0.0.1', port: 0 },
    plugins: [{
      name: 'console-stream-fixture',
      configureServer(server) {
        server.middlewares.use((request, response, next) => {
          const url = new URL(request.url, 'http://localhost');
          if (url.pathname !== '/api/logs/stream') return next();
          const stream = { room: url.searchParams.get('room'), shard: url.searchParams.get('shard'), response };
          opened.push({ room: stream.room, shard: stream.shard });
          streams.add(stream);
          response.writeHead(200, { 'Content-Type': 'text/event-stream', 'Cache-Control': 'no-cache' });
          for (let i = 0; i < 80; i++) {
            response.write(`data: ${JSON.stringify({ type: 'log', shard: stream.shard, message: `房间 ${stream.room} ${stream.shard} 第 ${i} 行` })}\n\n`);
          }
          response.on('close', () => streams.delete(stream));
        });
      },
    }],
  });
  await vite.listen();
  origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => {
  for (const stream of streams) stream.response.end();
  await browser?.close();
  await vite?.close();
});

async function openPanel(width = 1440, dark = false) {
  const page = await browser.newPage({ viewport: { width, height: 1000 } });
  page.setDefaultTimeout(10000);
  const state = { commands: [], commandError: '', errors: [] };
  page.on('pageerror', error => state.errors.push(error.message));
  await page.addInitScript(dark => {
    localStorage.setItem('dst-tab', 'overview');
    localStorage.setItem('dst-room', '1');
    localStorage.setItem('dst-theme', dark ? 'dark' : 'light');
  }, dark);
  await page.route('**/api/**', route => {
    const url = new URL(route.request().url());
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (url.pathname === '/api/logs/stream') return route.continue();
    if (url.pathname === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (url.pathname === '/api/rooms') return ok([1, 2].map(id => ({
      id, key: `Cluster_${id}`, name: id === 1 ? '四分片测试房间' : '另一房间',
      shards: (id === 1 ? ['Master', 'Caves', 'Island', 'Volcano'] : ['Master']).map(name => ({ name, isMaster: name === 'Master' })),
      locations: { Master: 'forest', Caves: 'cave', Island: 'shipwrecked', Volcano: 'volcanoworld' },
    })));
    if (url.pathname === '/api/rooms/meta') return ok({});
    if (url.pathname === '/api/install') return ok({ installed: true });
    if (url.pathname === '/api/host') return ok({});
    if (url.pathname === '/api/players') return ok({ players: [], errors: {} });
    if (url.pathname === '/api/status') return ok(['Master', 'Caves', 'Island', 'Volcano'].map(shard => ({ shard, state: 'running' })));
    if (url.pathname === '/api/cmd') {
      state.commands.push(Object.fromEntries(url.searchParams));
      if (state.commandError) return route.fulfill({ json: { code: 500, message: state.commandError } });
      return ok(null);
    }
    return ok([]);
  });
  await page.goto(origin);
  const console = page.getByRole('region', { name: '实时日志', exact: true });
  await console.getByText('房间 1 Master 第 79 行', { exact: true }).waitFor();
  return { page, console, state };
}

function emit(room, shard, message) {
  for (const stream of streams) {
    if (stream.room === room && stream.shard === shard) {
      stream.response.write(`data: ${JSON.stringify({ type: 'log', shard, message })}\n\n`);
    }
  }
}

test('单窗口 Tabs 切换不重连，后台分片持续接收，草稿、自动滚动及滚动位置独立保留', async () => {
  const { page, console, state } = await openPanel();
  try {
    const tabs = console.getByRole('tab');
    assert.equal(await tabs.count(), 4);
    assert.equal(await console.locator('[data-slot="code-block"]').count(), 1);
    assert.equal(await console.locator('[data-slot="code-block-content"]').count(), 1);
    assert.equal(await console.locator('[data-slot="code-block-code"]').evaluate(el => window.getComputedStyle(el).backgroundColor), 'rgba(0, 0, 0, 0)');
    await console.getByRole('tab', { name: /火山 Volcano 已连接/ }).waitFor();
    assert.equal(await console.locator('.console').count(), 1);
    const count = opened.length;
    const master = console.getByRole('tab', { name: /森林 Master/ });
    const caves = console.getByRole('tab', { name: /洞穴 Caves/ });
    await console.getByRole('textbox', { name: '森林 控制台命令', exact: true }).fill('c_save()');
    await console.getByRole('region', { name: '森林 日志输出', exact: true }).evaluate(el => {
      el.scrollTop = 100;
      el.dispatchEvent(new window.Event('scroll'));
    });
    await console.getByRole('button', { name: '森林 回到最新日志', exact: true }).waitFor();
    await caves.click();
    await console.getByText('房间 1 Caves 第 79 行', { exact: true }).waitFor();
    assert.equal(await console.getByText('房间 1 Master 第 79 行', { exact: true }).count(), 0);
    assert.equal(await console.getByRole('button', { name: '洞穴 回到最新日志', exact: true }).count(), 0);
    await console.getByRole('textbox', { name: '洞穴 控制台命令', exact: true }).fill('c_listallplayers()');
    emit('1', 'Master', '森林后台仍在接收');
    await master.click();
    await console.getByText('森林后台仍在接收', { exact: true }).waitFor();
    assert.equal(await console.getByRole('textbox', { name: '森林 控制台命令', exact: true }).inputValue(), 'c_save()');
    await console.getByRole('button', { name: '森林 回到最新日志', exact: true }).waitFor();
    await page.waitForFunction(() => document.querySelector('.console').scrollTop === 100);
    assert.equal(opened.length, count, '换 Tab 不改变 SSE 连接');
    await caves.click();
    assert.equal(await console.getByRole('textbox', { name: '洞穴 控制台命令', exact: true }).inputValue(), 'c_listallplayers()');
    assert.equal(await console.locator('.console').count(), 1);
    await console.scrollIntoViewIfNeeded();
    await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/console-code-block.png', import.meta.url)) });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('清屏只作用于当前分片，命令发送到选定分片，失败保留输入，换房清理草稿和日志', async () => {
  const { page, console, state } = await openPanel();
  try {
    await console.getByRole('tab', { name: /洞穴 Caves/ }).click();
    const input = console.getByRole('textbox', { name: '洞穴 控制台命令', exact: true });
    await input.fill('c_save()');
    state.commandError = '测试命令未执行';
    await console.getByRole('button', { name: '执行', exact: true }).click();
    await page.getByText('测试命令未执行', { exact: true }).waitFor();
    assert.equal(await input.inputValue(), 'c_save()');
    state.commandError = '';
    await console.getByRole('button', { name: '执行', exact: true }).click();
    await console.locator('.k-cmd').getByText('c_save()', { exact: true }).waitFor();
    assert.deepEqual(state.commands, [
      { room: '1', shard: 'Caves', cmd: 'c_save()' },
      { room: '1', shard: 'Caves', cmd: 'c_save()' },
    ]);
    assert.equal(await input.inputValue(), '');
    await console.getByRole('button', { name: '清屏', exact: true }).click();
    await console.getByText('暂无 洞穴 的日志输出。', { exact: true }).waitFor();
    await console.getByRole('tab', { name: /森林 Master/ }).click();
    await console.getByText('房间 1 Master 第 79 行', { exact: true }).waitFor();
    await console.getByRole('textbox', { name: '森林 控制台命令', exact: true }).fill('不要跨房间保留');
    await page.locator('#roomSelect').click();
    await page.getByRole('option', { name: '另一房间', exact: true }).click();
    await console.getByText('房间 2 Master 第 79 行', { exact: true }).waitFor();
    assert.equal(await console.getByRole('tab').count(), 1);
    assert.equal(await console.getByRole('textbox', { name: '森林 控制台命令', exact: true }).inputValue(), '');
    assert.equal(await console.getByText('房间 1 Master 第 79 行', { exact: true }).count(), 0);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('离开底部后暂停跟随，回到最新滚到末行并恢复自动滚动', async () => {
  const { page, console, state } = await openPanel();
  try {
    const output = console.getByRole('region', { name: '森林 日志输出', exact: true });
    const latest = console.getByRole('button', { name: '森林 回到最新日志', exact: true });
    await page.waitForFunction(() => {
      const el = document.querySelector('.console');
      return el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
    });
    assert.equal(await latest.count(), 0);
    await output.evaluate(el => {
      el.scrollTop = 0;
      el.dispatchEvent(new window.Event('scroll'));
    });
    await latest.waitFor();
    await latest.click();
    await latest.waitFor({ state: 'hidden' });
    await page.waitForFunction(() => {
      const el = document.querySelector('.console');
      return el.scrollTop + el.clientHeight >= el.scrollHeight - 30;
    });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('手机 Tabs 可键盘切换且无横向溢出，连接断开在对应 Tab 报告', async () => {
  const { page, console, state } = await openPanel(375);
  try {
    const tab = console.getByRole('tab', { name: /森林 Master/ });
    await tab.focus();
    await page.keyboard.press('ArrowRight');
    await console.getByText('房间 1 Caves 第 79 行', { exact: true }).waitFor();
    assert.equal(await console.getByRole('tab', { name: /洞穴 Caves/ }).getAttribute('aria-selected'), 'true');
    await page.keyboard.press('ArrowRight');
    await console.getByText('房间 1 Island 第 79 行', { exact: true }).waitFor();
    await page.keyboard.press('ArrowLeft');
    await console.getByText('房间 1 Caves 第 79 行', { exact: true }).waitFor();
    for (const stream of streams) if (stream.room === '1' && stream.shard === 'Caves') stream.response.end();
    await console.getByRole('tab', { name: /洞穴 Caves 重连中/ }).waitFor();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.equal(await console.locator('.console').count(), 1);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});
