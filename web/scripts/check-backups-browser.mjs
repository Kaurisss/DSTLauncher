// All backup operations are intercepted; no real archives or game saves are touched.
import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let vite;
let browser;
let origin;
before(async () => {
  vite = await createServer({
    root: fileURLToPath(new URL('..', import.meta.url)),
    server: { host: '127.0.0.1', port: 0 },
    plugins: [{
      name: 'backup-download-fixture',
      configureServer(server) {
        // Chromium downloads bypass Playwright routing, so serve this fixture before Vite's API proxy.
        server.middlewares.use((request, response, next) => {
          if (new URL(request.url, 'http://localhost').pathname !== '/api/backups/download') return next();
          response.setHeader('Content-Type', 'application/zip');
          response.setHeader('Content-Disposition', 'attachment');
          response.end('test backup');
        });
      },
    }],
  });
  await vite.listen();
  origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => { await browser?.close(); await vite?.close(); });

function archives() {
  return Array.from({ length: 12 }, (_, i) => ({
    name: `房间备份 ${i + 1}.zip`,
    createdAt: `2026-10-${String(i + 1).padStart(2, '0')}T12:00:00Z`,
    size: i === 0 ? 9 : i === 1 ? 1024 : (i + 1) * 1024 ** 2,
  }));
}

async function openBackups({ width = 1280, items = archives() } = {}) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(10000);
  const state = { items, requests: [], errors: [], deleteFailure: '', check: null, nextCheck: 0 };
  page.on('pageerror', error => state.errors.push(error.message));
  await page.addInitScript(() => {
    localStorage.setItem('dst-tab', 'backups');
    localStorage.setItem('dst-room', '2');
  });
  await page.context().route('**/api/**', route => {
    const url = new URL(route.request().url());
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (url.pathname === '/api/rooms') return ok([{ id: 2, name: '备份测试房间', shards: [{ name: 'Master', isMaster: true }] }]);
    if (url.pathname === '/api/rooms/meta') return ok({});
    if (url.pathname === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (url.pathname === '/api/backups') return ok(state.items);
    if (url.pathname === '/api/backup-remote') return ok({ enabled: false, results: [] });
    if (url.pathname === '/api/backups/check') {
      if (route.request().method() === 'POST') {
        const name = route.request().postDataJSON().name;
        state.check = { id: `check-${++state.nextCheck}`, name, room: 2, state: 'ready', report: { name, sha256: 'a'.repeat(64), canRestore: true, createdAt: state.items.find(item => item.name === name).createdAt, files: 12, bytes: 1024, shards: [], warnings: [] } };
        return ok({ id: state.check.id });
      }
      if (route.request().method() === 'DELETE') return ok(null);
      return ok(state.check);
    }
    if (url.pathname === '/api/backups/restore' && route.request().method() === 'POST') {
      state.requests.push({ action: 'restore', room: url.searchParams.get('room'), name: state.check.name });
      state.check = { ...state.check, state: 'restored', previousDirectory: 'isolated-before-restore' };
      return ok({ id: state.check.id });
    }
    if (url.pathname.startsWith('/api/backups/')) {
      const name = url.searchParams.get('name');
      const action = url.pathname.split('/').at(-1);
      state.requests.push({ action, room: url.searchParams.get('room'), name });

      if (action === 'delete') {
        if (state.deleteFailure) return route.fulfill({ json: { code: 500, message: state.deleteFailure } });
        state.items = state.items.filter(item => item.name !== name);
      }
      return ok(null);
    }
    return ok([]);
  });
  await page.goto(origin);
  const table = page.getByRole('table', { name: '备份列表' });
  await table.waitFor();
  return { page, table, state };
}

const firstSize = table => table.locator('tbody tr').first().locator('td').nth(1).innerText();

test('备份默认按最新时间排列，时间与大小按原始值排序，每页十条', async () => {
  const { page, table, state } = await openBackups();
  try {
    assert.equal(await table.locator('tbody tr').count(), 10);
    assert.equal(await firstSize(table), '12.0 MB');
    const timeHeader = table.getByRole('columnheader', { name: /时间/ });
    assert.equal(await timeHeader.getAttribute('aria-sort'), 'descending');
    assert.equal(await page.getByRole('button', { name: '上一页', exact: true }).isDisabled(), true);
    await page.getByRole('button', { name: '下一页', exact: true }).click();
    assert.equal(await table.locator('tbody tr').count(), 2);
    assert.equal(await firstSize(table), '1.0 KB');
    assert.equal(await page.getByRole('button', { name: '下一页', exact: true }).isDisabled(), true);
    await page.getByRole('button', { name: '上一页', exact: true }).click();
    await timeHeader.getByRole('button').click();
    assert.equal(await timeHeader.getAttribute('aria-sort'), 'ascending');
    assert.equal(await firstSize(table), '9 B');
    const sizeHeader = table.getByRole('columnheader', { name: /大小/ });
    await sizeHeader.getByRole('button').click();
    if (await sizeHeader.getAttribute('aria-sort') === 'descending') await sizeHeader.getByRole('button').click();
    assert.equal(await firstSize(table), '9 B');
    await sizeHeader.getByRole('button').click();
    assert.equal(await sizeHeader.getAttribute('aria-sort'), 'descending');
    assert.equal(await firstSize(table), '12.0 MB');
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('翻页后的操作对应正确备份：导出、回滚确认、删除失败保留及成功后刷新', async () => {
  const { page, table, state } = await openBackups();
  try {
    await page.getByRole('button', { name: '下一页', exact: true }).click();
    const row = table.locator('tbody tr').first();
    const download = page.waitForEvent('download');
    await row.getByRole('button', { name: '导出', exact: true }).click();
    const downloaded = await download;
    assert.equal(await downloaded.failure(), null);
    assert.equal(downloaded.suggestedFilename(), '房间备份 2.zip');
    const downloadUrl = new URL(downloaded.url());
    assert.equal(downloadUrl.pathname, '/api/backups/download');
    assert.equal(downloadUrl.searchParams.get('room'), '2');
    assert.equal(downloadUrl.searchParams.get('name'), '房间备份 2.zip');
    const dialog = page.getByRole('alertdialog');
    await row.getByRole('button', { name: '回滚', exact: true }).click();
    const inspection = page.getByRole('dialog', { name: '备份恢复检查', exact: true });
    await inspection.getByRole('button', { name: '确认恢复此备份', exact: true }).click();
    await dialog.getByText(/回滚到 房间备份 2.zip/).waitFor();
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(state.requests.length, 0);
    await inspection.getByRole('button', { name: '确认恢复此备份', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await inspection.getByText(/恢复完成/).waitFor();
    await inspection.getByRole('button', { name: '完成', exact: true }).click();
    await table.getByRole('cell', { name: '12.0 MB', exact: true }).waitFor();
    assert.deepEqual(state.requests.at(-1), { action: 'restore', room: '2', name: '房间备份 2.zip' });
    await page.getByRole('button', { name: '下一页', exact: true }).click();
    state.deleteFailure = '备份文件被占用';
    await row.getByRole('button', { name: '删除', exact: true }).click();
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(state.requests.length, 1);
    await row.getByRole('button', { name: '删除', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await page.getByText('备份文件被占用', { exact: true }).waitFor();
    assert.equal(await firstSize(table), '1.0 KB');
    state.deleteFailure = '';
    await row.getByRole('button', { name: '删除', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await table.waitFor();
    await page.getByRole('navigation', { name: '备份分页' }).getByText('第 1 / 2 页').waitFor();
    assert.deepEqual(state.requests.slice(-2), [
      { action: 'delete', room: '2', name: '房间备份 2.zip' },
      { action: 'delete', room: '2', name: '房间备份 2.zip' },
    ]);
    assert.equal(state.items.length, 11);
    await page.getByRole('button', { name: '下一页', exact: true }).click();
    assert.equal(await table.locator('tbody tr').count(), 1);
    assert.equal(await firstSize(table), '9 B');
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('手机表格内部横向滚动，删空后显示原有空状态', async () => {
  const { page, table, state } = await openBackups({ width: 375, items: archives().slice(-1) });
  try {
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.equal(await table.locator('..').evaluate(el => el.scrollWidth > el.clientWidth), true);
    await table.getByRole('button', { name: '删除', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await page.getByText('暂无备份', { exact: true }).waitFor();
    assert.equal(await page.getByRole('navigation', { name: '备份分页' }).count(), 0);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});
