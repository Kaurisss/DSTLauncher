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

async function openPanel({ states = ['running', 'running'], failure = false, delay = false } = {}) {
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
  page.setDefaultTimeout(10000);
  page.on('pageerror', error => console.error('回档页面异常:', error.message));
  page.on('console', event => { if (event.type() === 'error') console.error('回档页面控制台:', event.text()); });
  await page.addInitScript(() => {
    localStorage.setItem('dst-room', '2');
    localStorage.setItem('dst-tab', 'overview');
  });
  const requests = [];
  let release;
  const pending = delay ? new Promise(resolve => { release = resolve; }) : Promise.resolve();
  const shards = [{ name: 'Surface', isMaster: true }, { name: 'Below' }];
  await page.route('**/api/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/rooms') return ok([{ id: 2, name: '回档验证房间', maxSnapshots: 10, shards }]);
    if (path === '/api/status') return ok(shards.map((s, i) => ({ shard: s.name, state: states[i] })));
    if (path === '/api/rooms/meta') return ok({});
    if (path === '/api/server') return ok({ installed: true, version: '756039', candidates: [] });
    if (path === '/api/install') return ok({ installed: true });
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    if (path === '/api/rollback') {
      requests.push({ method: request.method(), room: new URL(request.url()).searchParams.get('room'), body: request.postDataJSON() });
      await pending;
      if (failure) return route.fulfill({ json: { code: 500, message: '可用历史存档不足' } });
      return ok({ requested: true, retryAfterSeconds: 60 });
    }
    return ok([]);
  });
  await page.goto(origin);
  await page.getByRole('button', { name: '回档', exact: true }).waitFor();
  return { page, requests, release };
}

test('回档有明确确认；取消不请求，非法次数不可提交，备份页同样提供入口', async () => {
  const { page, requests } = await openPanel();
  try {
    await page.getByRole('button', { name: '回档', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '游戏存档回档' });
    await dialog.waitFor();
    assert.match(await dialog.innerText(), /当前未保存进度/);
    await dialog.getByLabel('回退存档次数').fill('0');
    assert.equal(await dialog.getByRole('button', { name: '确认回档', exact: true }).isDisabled(), true);
    await dialog.getByLabel('回退存档次数').fill('1.5');
    assert.equal(await dialog.getByRole('button', { name: '确认回档', exact: true }).isDisabled(), true);
    await dialog.getByRole('button', { name: '取消' }).click();
    await dialog.waitFor({ state: 'hidden' });
    await page.getByRole('button', { name: '回档', exact: true }).click();
    await dialog.getByRole('button', { name: '关闭', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    assert.equal(requests.length, 0);
    await page.locator('#app-navigation').getByRole('button', { name: '备份', exact: true }).click();
    await page.getByRole('button', { name: '回档', exact: true }).waitFor();
  } finally { await page.close(); }
});

test('部分分片未运行时禁止回档并说明启动要求', async () => {
  const { page, requests } = await openPanel({ states: ['running', 'stopped'] });
  try {
    await page.getByRole('button', { name: '回档', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '游戏存档回档' });
    await dialog.getByText(/请先在概览页启动房间所有启用的世界/).waitFor();
    assert.equal(await dialog.getByRole('button', { name: '确认回档' }).isDisabled(), true);
    assert.equal(requests.length, 0);
  } finally { await page.close(); }
});

test('确认只发送一次 POST，等待游戏回执后关闭并阻止重复回档', async () => {
  const { page, requests, release } = await openPanel({ delay: true });
  try {
    await page.getByRole('button', { name: '回档', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '游戏存档回档' });
    await dialog.getByLabel('回退存档次数').fill('2');
    await dialog.getByRole('button', { name: '确认回档' }).click();
    await dialog.getByRole('button', { name: '正在请求回档…' }).waitFor();
    assert.equal(await dialog.getByRole('button', { name: '正在请求回档…' }).isDisabled(), true);
    assert.equal(await dialog.getByRole('button', { name: '关闭', exact: true }).isDisabled(), true);
    assert.equal(await dialog.getByRole('button', { name: '取消', exact: true }).isDisabled(), true);
    await page.keyboard.press('Escape');
    assert.equal(await dialog.isVisible(), true);
    assert.deepEqual(requests, [{ method: 'POST', room: '2', body: { count: 2 } }]);
    release();
    await dialog.waitFor({ state: 'hidden' });
    assert.equal(await page.getByRole('button', { name: /^回档中/ }).isDisabled(), true);
    assert.equal(requests.length, 1);
  } finally { release(); await page.close(); }
});

test('游戏拒绝回档时保留输入并展示原因，可重新提交', async () => {
  const { page, requests } = await openPanel({ failure: true });
  try {
    await page.getByRole('button', { name: '回档', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '游戏存档回档' });
    await dialog.getByLabel('回退存档次数').fill('2');
    await dialog.getByRole('button', { name: '确认回档' }).click();
    await page.getByText('可用历史存档不足', { exact: true }).waitFor();
    assert.equal(await dialog.isVisible(), true);
    assert.equal(await dialog.getByLabel('回退存档次数').inputValue(), '2');
    assert.equal(await dialog.getByRole('button', { name: '确认回档' }).isEnabled(), true);
    assert.equal(requests.length, 1);
  } finally { await page.close(); }
});
