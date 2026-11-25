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
const resources = { world: '/api/worldconfig', mods: '/api/mods', backups: '/api/backups', saves: '/api/saves' };

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
    if (path === '/api/diagnostics') return ok({ time: new Date().toISOString(), room: 1, checks: [], shards: [], downloads: { findings: [] } });
    if (path === '/api/config') { await state.pending; return ok({ clusterDir: 'D:\\DST\\DST\\Cluster_1', files }); }
    return ok([]);
  });
  await page.goto(origin);
  return { page, state, release };
}

test('世界、模组、备份与存档初次加载都使用骨架屏', async () => {
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

async function openConfigPreview(page) {
  await page.getByRole('button', { name: '故障诊断', exact: true }).click();
  const diag = page.getByRole('dialog', { name: '故障诊断', exact: true });
  await diag.getByRole('button', { name: '配置预览', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: /配置预览/ });
  await dialog.waitFor();
  return dialog;
}

test('配置预览弹窗初次加载使用骨架屏，Code Block 只读并保留落盘与令牌隐藏状态，复制得到原文本而非行号', async () => {
  const { page, state, release } = await openPage('overview', { paused: true, width: 375 });
  try {
    const dialog = await openConfigPreview(page);
    const loading = dialog.getByRole('status', { name: '加载中', exact: true });
    await loading.waitFor();
    assert.ok(await loading.locator('[data-slot="skeleton"]').count() > 0);
    release();
    await loading.waitFor({ state: 'hidden' });

    const preview = dialog.getByRole('region', { name: files[0].path, exact: true });
    await preview.waitFor();
    assert.equal(await dialog.locator('[data-slot="code-block"]').count(), 3);
    const blockStyle = await preview.locator('xpath=ancestor::*[@data-slot="code-block"]').evaluate(el => {
      const style = window.getComputedStyle(el);
      return { radius: style.borderRadius, border: style.borderWidth, ring: style.boxShadow };
    });
    assert.deepEqual({ radius: blockStyle.radius, border: blockStyle.border }, { radius: '14px', border: '1px' });
    assert.match(blockStyle.ring, /0px 0px 0px 1px/);
    assert.equal(await preview.getAttribute('data-numbered'), 'true');
    assert.match(await preview.innerText(), /cluster_name = <img src=x onerror=alert\(1\)>/);
    assert.equal(await preview.locator('img, textarea, input, [contenteditable="true"]').count(), 0);
    await dialog.getByText('令牌已隐藏', { exact: true }).waitFor();
    await dialog.getByText('未生成', { exact: true }).waitFor();
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
  } finally { release(); await page.close(); }
});

test('配置预览弹窗刷新保留现有 Code Block，加载过程中不切回占位', async () => {
  const { page, state } = await openPage('overview');
  let release = () => {};
  try {
    const dialog = await openConfigPreview(page);
    const preview = dialog.getByRole('region', { name: files[0].path, exact: true });
    await preview.waitFor();
    state.pending = new Promise(resolve => { release = resolve; });
    await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === '/api/config'),
      dialog.getByRole('button', { name: '刷新', exact: true }).click(),
    ]);
    assert.equal(await preview.isVisible(), true);
    assert.equal(await dialog.getByRole('status', { name: '加载中', exact: true }).count(), 0);
    assert.equal(await dialog.getByRole('button', { name: '刷新', exact: true }).isDisabled(), true);
    release();
    await dialog.getByRole('button', { name: '刷新', exact: true }).waitFor();
    await page.waitForFunction(() => [...document.querySelectorAll('[role="dialog"] button')].some(button => button.getAttribute('aria-label') === '刷新' && !button.disabled));
    assert.deepEqual(state.errors, []);
  } finally { release(); await page.close(); }
});
