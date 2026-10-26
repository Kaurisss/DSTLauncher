// All maintenance requests are intercepted; no live games or saves are changed.
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

async function openPanel(tab, width = 1280) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  const state = {
    config: { backupHours: 0, keep: 10, restartAt: '', updateMinutes: 0, updateMode: 'notify' },
    jobs: {}, running: '', timezone: 'CST (UTC+08:00)',
  };
  const requests = [];
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.addInitScript(tab => { localStorage.setItem('dst-tab', tab); localStorage.setItem('dst-room', '1'); }, tab);
  await page.route('**/api/**', async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/automation') {
      if (request.method() === 'PUT') { state.config = request.postDataJSON(); requests.push(state.config); }
      return ok(state);
    }
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok([{ id: 1, name: '测试房间', shards: [{ name: 'Master', isMaster: true }, { name: 'Caves' }], locations: { Master: 'forest', Caves: 'cave' } }]);
    if (path === '/api/rooms/meta') return ok({});
    if (path === '/api/server') return ok({ installed: true, candidates: [] });
    if (path === '/api/install') return ok({ installed: true });
    if (path === '/api/host') return ok({});
    if (path === '/api/worldstate') return ok([
      { shard: 'Master', state: { day: 16, season: 'winter', remainingDays: 0, savedAt: '2026-10-25T12:00:00+08:00' } },
      { shard: 'Caves', state: null, error: '快照尚未写完' },
    ]);
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    return ok([]);
  });
  await page.goto(origin);
  return { page, state, requests, errors };
}

test('计划默认关闭，确认后保存数值与无人更新模式，取消不写入', async () => {
  const { page, requests, errors } = await openPanel('settings');
  try {
    const hours = page.getByLabel('所有房间备份间隔（小时，0 关闭）');
    await hours.waitFor();
    assert.equal(await hours.inputValue(), '0');
    await hours.fill('6');
    await page.getByLabel('每个房间保留自动备份份数').fill('4');
    await page.getByRole('switch', { name: /每日重启/ }).click();
    await page.getByLabel('每日重启时间').fill('05:30');
    await page.getByLabel('更新检查间隔（分钟，0 关闭）').fill('30');
    await page.getByLabel('发现服务端或已启用模组更新时').selectOption('idle');
    await page.getByRole('button', { name: '保存计划', exact: true }).click();
    const confirm = page.getByRole('alertdialog');
    await confirm.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(requests.length, 0);
    await page.getByRole('button', { name: '保存计划', exact: true }).click();
    await confirm.getByRole('button', { name: '确定', exact: true }).click();
    await page.getByText('自动维护计划已保存', { exact: true }).waitFor();
    assert.deepEqual(requests, [{ backupHours: 6, keep: 4, restartAt: '05:30', updateMinutes: 30, updateMode: 'idle' }]);
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});

test('轮询更新结果但不覆盖编辑中的计划，移动端不溢出', async () => {
  const { page, state, errors } = await openPanel('settings', 375);
  try {
    const hours = page.getByLabel('所有房间备份间隔（小时，0 关闭）');
    await hours.fill('8');
    state.jobs.update = { result: '发现模组更新，等待无人连接' };
    await page.getByText('发现模组更新，等待无人连接', { exact: true }).waitFor();
    assert.equal(await hours.inputValue(), '8');
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});

test('概览显示存档天数和零天剩余，失败分片单独报错', async () => {
  const { page, errors } = await openPanel('overview');
  try {
    const region = page.getByRole('region', { name: '分片状态', exact: true });
    await region.getByText('第 16 天', { exact: true }).waitFor();
    assert.match(await region.innerText(), /冬季/);
    assert.match(await region.innerText(), /剩 0 天/);
    assert.match(await region.innerText(), /存档于/);
    assert.match(await region.innerText(), /读取失败：快照尚未写完/);
    assert.deepEqual(errors, []);
  } finally { await page.close(); }
});
