// Host metrics use intercepted APIs only; no live rooms, passwords or saves are changed.
import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let vite;
let browser;
let origin;
const GB = 1024 ** 3;
const healthy = {
  memory: { totalBytes: 16 * GB, availableBytes: 8 * GB },
  disk: { totalBytes: 100 * GB, availableBytes: 40 * GB },
  diskPath: 'D:\\DST 存档\\DST',
};

before(async () => {
  vite = await createServer({
    root: fileURLToPath(new URL('..', import.meta.url)),
    server: { host: '127.0.0.1', port: 0 },
  });
  await vite.listen();
  origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => { await browser?.close(); await vite?.close(); });

async function openPanel({ data = healthy, width = 1280 } = {}) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(12000);
  const state = { data, failure: false, requests: 0 };
  await page.addInitScript(() => localStorage.setItem('dst-tab', 'overview'));
  await page.route('**/api/**', route => {
    const path = new URL(route.request().url()).pathname;
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/host') {
      state.requests++;
      if (state.failure) return route.fulfill({ json: { code: 500, message: '采样服务暂不可用' } });
      return ok(state.data);
    }
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok([]);
    if (path === '/api/rooms/meta') return ok({});
    if (path === '/api/server') return ok({ installed: true, candidates: [] });
    if (path === '/api/install') return ok({ installed: true });
    return ok([]);
  });
  await page.goto(origin);
  const section = page.getByRole('region', { name: '宿主机资源' });
  await section.getByText('宿主机内存', { exact: true }).waitFor();
  return { page, section, state };
}

test('没有房间也显示宿主机而非分片内存，桌面与手机无横向溢出', async () => {
  for (const width of [1280, 375]) {
    const { page, section } = await openPanel({ width });
    try {
      await section.getByText('50.0%', { exact: true }).waitFor();
      assert.match(await section.innerText(), /已用 8.00 GB \/ 16.00 GB/);
      assert.match(await section.innerText(), /40.00 GB/);
      assert.match(await section.innerText(), /总容量 100.00 GB/);
      assert.equal(await section.getByRole('status').count(), 0);
      assert.equal(await section.locator('[title]').getAttribute('title'), healthy.diskPath);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    } finally { await page.close(); }
  }
});

test('高内存及低磁盘有文字预警，零可用空间显示真实零值', async () => {
  for (const availableBytes of [0, 9 * GB]) {
    const { page, section } = await openPanel({ data: {
      ...healthy,
      memory: { totalBytes: 10 * GB, availableBytes: GB },
      disk: { totalBytes: 100 * GB, availableBytes },
    } });
    try {
      await section.getByText('90.0%', { exact: true }).waitFor();
      assert.match(await section.getByRole('status').innerText(), /内存使用已达 90%/);
      assert.match(await section.getByRole('status').innerText(), /存档盘空间不足/);
      await section.getByText(availableBytes ? '9.00 GB' : '0 B', { exact: true }).waitFor();
    } finally { await page.close(); }
  }
});

test('单项读取失败不遮挡其它资源，也不把未知容量当作零或低空间', async () => {
  const { page, section } = await openPanel({ data: {
    ...healthy, disk: null, diskError: '无权访问存档目录',
  } });
  try {
    await section.getByText('50.0%', { exact: true }).waitFor();
    assert.match(await section.getByRole('status').innerText(), /存档盘读取失败：无权访问存档目录/);
    assert.doesNotMatch(await section.innerText(), /0 B|存档盘空间不足/);
  } finally { await page.close(); }
});

test('轮询更新容量，请求失败清除旧值，恢复后清除错误，离开概览停止轮询', async () => {
  const { page, section, state } = await openPanel();
  try {
    await section.getByText('50.0%', { exact: true }).waitFor();
    state.failure = true;
    await section.getByText('宿主机资源读取失败：采样服务暂不可用', { exact: true }).waitFor();
    assert.equal(await section.getByText('50.0%', { exact: true }).count(), 0);
    state.failure = false;
    state.data = { ...healthy, disk: { totalBytes: 20 * GB, availableBytes: 4 * GB } };
    await section.getByText('4.00 GB', { exact: true }).waitFor();
    assert.match(await section.getByRole('status').innerText(), /存档盘空间不足/);
    assert.doesNotMatch(await section.innerText(), /读取失败/);
    await page.locator('#app-navigation').getByRole('button', { name: '房间', exact: true }).click();
    await section.waitFor({ state: 'hidden' });
    const count = state.requests;
    // Observe a full refresh interval to prove unmount actually stops polling.
    await new Promise(resolve => setTimeout(resolve, 5500));
    assert.equal(state.requests, count);
  } finally { await page.close(); }
});
