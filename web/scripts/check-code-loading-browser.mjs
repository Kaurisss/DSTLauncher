// All API responses are local fixtures; loading and copy never touch real saves or configuration.
import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let vite;
let browser;
let origin;
before(async () => {
  vite = await createServer({ root: fileURLToPath(new URL('..', import.meta.url)), server: { host: '127.0.0.1', port: 0 } });
  await vite.listen();
  origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => { await browser?.close(); await vite?.close(); });

const files = [
  { path: 'Cluster_1/cluster.ini', onDisk: true, content: '[NETWORK]\ncluster_name = <img src=x onerror=alert(1)>\n\n[GAMEPLAY]\ngame_mode = survival\n' },
  { path: 'Cluster_1/Master/server.ini', onDisk: false, content: '[NETWORK]\nserver_port = 10999\n' },
  { path: 'Cluster_1/cluster_token.txt', onDisk: true, redacted: true, content: '[令牌已隐藏]' },
];
const resources = { world: '/api/worldconfig', mods: '/api/mods', backups: '/api/backups', saves: '/api/saves', config: '/api/config' };

async function openPage(tab, { paused = false, width = 1280 } = {}) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(10000);
  const state = { pending: Promise.resolve(), requests: [], errors: [] };
  let release = () => {};
  if (paused) state.pending = new Promise(resolve => { release = resolve; });
  page.on('pageerror', error => state.errors.push(error.message));
  await page.addInitScript(tab => {
    localStorage.setItem('dst-tab', tab);
    localStorage.setItem('dst-room', '1');
  }, tab);
  await page.route('**/api/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    state.requests.push({ path, method: request.method() });
    if (path === resources[tab]) await state.pending;
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok([{ id: 1, key: 'Cluster_1', name: '组件测试房间', gameMode: 'survival', maxPlayers: 4, shards: [{ name: 'Master', isMaster: true, worldType: 'forest' }] }]);
    if (path === '/api/rooms/meta') return ok({});
    if (path === '/api/status') return ok([]);
    if (path === '/api/install') return ok({ installed: true });
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    if (path === '/api/worldconfig') return ok({
      shard: 'Master', exists: true, location: 'forest', groups: [],
      config: { overrideEnabled: true, overrides: {} }, presets: { settings: [], worldgen: [] },
    });
    if (path === '/api/mods/update') return ok({ running: false, output: [] });
    if (path === '/api/saves') return ok({ root: 'D:\\DST\\DST', items: [] });
    if (path === '/api/config') return ok({ clusterDir: 'D:\\DST\\DST\\Cluster_1', files });
    return ok([]);
  });
  await page.goto(origin);
  return { page, state, release };
}

test('世界、模组、备份、存档与配置初次加载都使用骨架屏', async () => {
  for (const tab of Object.keys(resources)) {
    const { page, state, release } = await openPage(tab, { paused: true });
    try {
      const loading = page.getByRole('status', { name: '加载中', exact: true });
      await loading.waitFor();
      assert.ok(await loading.locator('[data-slot="skeleton"]').count() > 0, tab);
      assert.equal(await loading.locator('svg').count(), 0, tab);
      release();
      await loading.waitFor({ state: 'hidden' });
      assert.deepEqual(state.errors, [], tab);
    } finally { release(); await page.close(); }
  }
});

test('配置 Code Block 只读并保留落盘与令牌隐藏状态，复制得到原文本而非行号', async () => {
  const { page, state } = await openPage('config', { width: 375 });
  try {
    const preview = page.getByRole('region', { name: files[0].path, exact: true });
    await preview.waitFor();
    assert.equal(await page.locator('[data-slot="code-block"]').count(), 3);
    const blockStyle = await preview.locator('xpath=ancestor::*[@data-slot="code-block"]').evaluate(el => {
      const style = window.getComputedStyle(el);
      return { radius: style.borderRadius, border: style.borderWidth, ring: style.boxShadow };
    });
    assert.deepEqual({ radius: blockStyle.radius, border: blockStyle.border }, { radius: '14px', border: '1px' });
    assert.match(blockStyle.ring, /0px 0px 0px 1px/);
    assert.equal(await preview.getAttribute('data-numbered'), 'true');
    assert.match(await preview.innerText(), /cluster_name = <img src=x onerror=alert\(1\)>/);
    assert.equal(await preview.locator('img, textarea, input, [contenteditable="true"]').count(), 0);
    await page.getByText('令牌已隐藏', { exact: true }).waitFor();
    await page.getByText('未生成', { exact: true }).waitFor();
    assert.equal(await preview.locator('[data-slot="code-block-code"]').evaluate(el => window.getComputedStyle(el).backgroundColor), 'rgba(0, 0, 0, 0)');
    const widths = await preview.locator('[data-slot="code-block-code"]').evaluate(el => {
      const style = window.getComputedStyle(el);
      const canvas = document.createElement('canvas');
      const context = canvas.getContext('2d');
      context.font = `${style.fontSize} ${style.fontFamily}`;
      return ['iiii', 'WWWW', '0000'].map(text => context.measureText(text).width);
    });
    assert.ok(Math.max(...widths) - Math.min(...widths) < 0.1, '代码字符使用等宽字形');
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    const block = preview.locator('xpath=ancestor::*[@data-slot="code-block"]');
    await block.getByRole('button', { name: 'Copy', exact: true }).click();
    await block.getByRole('button', { name: 'Copied', exact: true }).waitFor();
    assert.equal((await page.evaluate(() => window.navigator.clipboard.readText())).replace(/\r\n/g, '\n'), files[0].content);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.equal(state.requests.some(request => request.method !== 'GET'), false);
    assert.deepEqual(state.errors, []);
    for (const { width, dark } of [{ width: 375, dark: false }, { width: 1440, dark: true }]) {
      await page.setViewportSize({ width, height: 900 });
      await page.evaluate(dark => localStorage.setItem('dst-theme', dark ? 'dark' : 'light'), dark);
      await page.reload();
      await preview.waitFor();
      await page.locator('[data-slot="page-scroll"]').evaluate(el => { el.scrollTop = 0; });
      await page.mouse.move(0, 0);
      await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/code-preview-${width}-${dark ? 'dark' : 'light'}.png`, import.meta.url)) });
    }
  } finally { await page.close(); }
});

test('配置刷新保留现有 Code Block，加载过程中不切回占位', async () => {
  const { page, state } = await openPage('config');
  let release = () => {};
  try {
    const preview = page.getByRole('region', { name: files[0].path, exact: true });
    await preview.waitFor();
    state.pending = new Promise(resolve => { release = resolve; });
    await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === '/api/config'),
      page.getByRole('button', { name: '刷新', exact: true }).click(),
    ]);
    assert.equal(await preview.isVisible(), true);
    assert.equal(await page.getByRole('status', { name: '加载中', exact: true }).count(), 0);
    assert.equal(await page.getByRole('button', { name: '刷新', exact: true }).isDisabled(), true);
    release();
    await page.waitForFunction(() => [...document.querySelectorAll('button')].some(button => button.getAttribute('aria-label') === '刷新' && !button.disabled));
    assert.deepEqual(state.errors, []);
  } finally { release(); await page.close(); }
});
