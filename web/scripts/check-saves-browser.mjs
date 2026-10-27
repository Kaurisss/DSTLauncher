// All save reads/deletes are intercepted; no real save directories are touched.
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

function archives(count = 12) {
  return Array.from({ length: count }, (_, i) => ({
    cluster: `Cluster_${i + 1}`, roomName: i === 0 ? '测试房间甲' : '', roomExists: i === 0,
    running: i === 1, hasSave: i !== 2,
    shards: [{ name: 'Master' }, { name: 'Caves' }],
    size: i === 0 ? 9 : i === 1 ? 1024 : (i + 1) * 1024 ** 2,
    modTime: `2026-10-${String(i + 1).padStart(2, '0')}T12:00:00Z`,
  }));
}

async function openSaves({ items = archives(), width = 1280, failure = '' } = {}) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(10000);
  const state = { items, failure, deleteFailure: '', requests: [], errors: [] };
  page.on('pageerror', error => state.errors.push(error.message));
  await page.addInitScript(() => localStorage.setItem('dst-tab', 'saves'));
  await page.route('**/api/**', route => {
    const request = route.request();
    const url = new URL(request.url());
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (url.pathname === '/api/saves') {
      if (request.method() === 'DELETE') {
        const cluster = url.searchParams.get('cluster');
        state.requests.push(cluster);
        if (state.deleteFailure) return route.fulfill({ json: { code: 500, message: state.deleteFailure } });
        state.items = state.items.filter(item => item.cluster !== cluster);
        return ok(null);
      }
      if (state.failure) return route.fulfill({ json: { code: 500, message: state.failure } });
      return ok({ root: 'D:\\DST 存档\\DST', items: state.items });
    }
    if (url.pathname === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (url.pathname === '/api/rooms/meta') return ok({});
    return ok([]);
  });
  await page.goto(origin);
  const table = page.getByRole('table', { name: '存档列表' });
  await table.waitFor();
  await page.locator('[aria-busy="false"]').waitFor();
  return { page, table, state };
}

const rowFor = (table, name) => table.getByRole('row').filter({ hasText: new RegExp(`^${name}(?:运行中|房间|测试|\\s)`) });

test('搜索房间、目录和分片，翻页后筛选回到首页，空结果可恢复', async () => {
  const { page, table, state } = await openSaves();
  try {
    assert.equal(await table.locator('tbody tr').count(), 10);
    await page.getByRole('button', { name: '下一页', exact: true }).click();
    await rowFor(table, 'Cluster_12').waitFor();
    assert.equal(await table.locator('tbody tr').count(), 2);
    const search = page.getByRole('textbox', { name: '搜索存档' });
    await search.fill('测试房间甲');
    await rowFor(table, 'Cluster_1').waitFor();
    assert.equal(await table.locator('tbody tr').count(), 1);
    assert.match(await page.getByRole('navigation', { name: '存档分页' }).innerText(), /第 1 \/ 1 页/);
    await search.fill('cluster_12');
    await rowFor(table, 'Cluster_12').waitFor();
    await search.fill('没有这份存档');
    await table.getByText('没有匹配的存档', { exact: true }).waitFor();
    await search.fill('caves');
    await table.getByText('Cluster_1', { exact: true }).waitFor();
    assert.equal(await table.getByText('Cluster_3', { exact: true }).count(), 0);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('大小和时间按原始值排序，目录按数字自然排序，表头报告排序方向', async () => {
  const { page, table } = await openSaves();
  try {
    const header = table.getByRole('columnheader', { name: /大小/ });
    await header.getByRole('button').click();
    if (await header.getAttribute('aria-sort') === 'descending') await header.getByRole('button').click();
    assert.equal(await table.locator('tbody tr').first().locator('td').nth(3).innerText(), '9 B');
    await header.getByRole('button').click();
    if (await header.getAttribute('aria-sort') !== 'descending') await header.getByRole('button').click();
    assert.equal(await table.locator('tbody tr').first().locator('code').innerText(), 'Cluster_12');
    await table.getByRole('button', { name: /最后修改/ }).click();
    const dateHeader = table.getByRole('columnheader', { name: /最后修改/ });
    const descending = await dateHeader.getAttribute('aria-sort') === 'descending';
    assert.equal(await table.locator('tbody tr').first().locator('code').innerText(), descending ? 'Cluster_12' : 'Cluster_1');
    await table.getByRole('button', { name: /存档目录/ }).click();
    assert.deepEqual(await table.locator('tbody code').allTextContents(), Array.from({ length: 10 }, (_, i) => `Cluster_${i + 1}`));
  } finally { await page.close(); }
});

test('已关联或运行中的存档禁止删除，取消不请求，确认只删除对应目录并刷新', async () => {
  const { page, table, state } = await openSaves();
  try {
    assert.equal(await rowFor(table, 'Cluster_1').getByRole('button', { name: '删除', exact: true }).isDisabled(), true);
    assert.equal(await rowFor(table, 'Cluster_2').getByRole('button', { name: '删除', exact: true }).isDisabled(), true);
    const remove = rowFor(table, 'Cluster_3').getByRole('button', { name: '删除', exact: true });
    await remove.click();
    const dialog = page.getByRole('alertdialog');
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.deepEqual(state.requests, []);
    await remove.click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await rowFor(table, 'Cluster_3').waitFor({ state: 'hidden' });
    assert.deepEqual(state.requests, ['Cluster_3']);
    await rowFor(table, 'Cluster_11').waitFor();
  } finally { await page.close(); }
});

test('删除失败保留存档，最后一页删空后仍回到有数据的页', async () => {
  const { page, table, state } = await openSaves({ items: archives(11) });
  try {
    await page.getByRole('button', { name: '下一页', exact: true }).click();
    const remove = rowFor(table, 'Cluster_11').getByRole('button', { name: '删除', exact: true });
    await remove.waitFor();
    state.deleteFailure = '存档被占用';
    await remove.click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await page.getByText('存档被占用', { exact: true }).waitFor();
    await rowFor(table, 'Cluster_11').waitFor();
    state.deleteFailure = '';
    await remove.click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await rowFor(table, 'Cluster_1').waitFor();
    assert.equal(await table.locator('tbody tr').count(), 10);
    assert.deepEqual(state.requests, ['Cluster_11', 'Cluster_11']);
  } finally { await page.close(); }
});

test('读取失败可刷新恢复，空列表有明确状态，手机只在表格内横向滚动', async () => {
  const { page, table, state } = await openSaves({ failure: '无权读取目录', width: 375 });
  try {
    await table.getByRole('alert').getByText('读取失败：无权读取目录', { exact: true }).waitFor();
    state.failure = '';
    state.items = [];
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await table.getByText('磁盘上还没有任何存档', { exact: true }).waitFor();
    state.items = archives();
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await rowFor(table, 'Cluster_1').waitFor();
    await page.getByRole('button', { name: '复制存档根目录', exact: true }).hover();
    const rootPath = page.getByRole('tooltip').getByText('D:\\DST 存档\\DST', { exact: true });
    await rootPath.waitFor();
    for (const theme of ['light', 'dark']) {
      await page.evaluate(value => document.documentElement.dataset.theme = value, theme);
      assert.equal(await rootPath.evaluate(el => window.getComputedStyle(el).backgroundColor), 'rgba(0, 0, 0, 0)');
    }
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.equal(await page.locator('[data-slot="table-container"]').evaluate(el => el.scrollWidth > el.clientWidth), true);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});
