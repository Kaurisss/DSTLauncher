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
      await section.locator('.stat').filter({ hasText: '存档盘可用空间' }).hover();
      await page.getByRole('tooltip').waitFor();
      assert.equal(await page.getByRole('tooltip').innerText(), healthy.diskPath);
      await page.keyboard.press('Escape');
      await page.getByRole('tooltip').waitFor({ state: 'hidden' });
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    } finally { await page.close(); }
  }
});

test('导航和主题使用组件提示，支持键盘焦点、Escape 和动态内容', async () => {
  const { page } = await openPanel();
  try {
    assert.equal(await page.locator('[title]').count(), 0);
    await page.getByRole('button', { name: '切换导航', exact: true }).hover();
    await page.getByRole('tooltip', { name: '展开或收起导航', exact: true }).waitFor();
    await page.keyboard.press('Escape');
    await page.getByRole('tooltip').waitFor({ state: 'hidden' });
    await page.mouse.move(0, 0);
    const theme = page.getByRole('button', { name: '切换主题', exact: true });
    await page.keyboard.press('Tab');
    await theme.focus();
    await page.getByRole('tooltip').waitFor();
    const before = await page.getByRole('tooltip').innerText();
    assert.match(before, /当前：.*\s+点击切换为：/);
    assert.equal(await page.locator('[data-slot="tooltip-content"]').evaluate(element => window.getComputedStyle(element).whiteSpace), 'pre-line');
    await page.keyboard.press('Enter');
    await page.getByRole('tooltip').waitFor({ state: 'hidden' });
    await page.mouse.move(0, 0);
    await theme.hover();
    await page.getByRole('tooltip').waitFor();
    assert.notEqual(await page.getByRole('tooltip').innerText(), before);
    await page.keyboard.press('Escape');
    await page.getByRole('tooltip').waitFor({ state: 'hidden' });
  } finally { await page.close(); }
});

test('页面切换入场后内容可见，减少动态效果时停用动画', async () => {
  const { page } = await openPanel();
  try {
    await page.emulateMedia({ reducedMotion: 'no-preference' });
    await page.evaluate(() => {
      window.pageEntries = 0;
      document.addEventListener('animationstart', event => {
        if (event.target.parentElement?.classList.contains('page-view')) window.pageEntries++;
      });
    });
    const nav = page.locator('#app-navigation');
    await nav.getByRole('button', { name: '房间', exact: true }).click();
    await page.waitForFunction(() => window.pageEntries > 0);
    await nav.getByRole('button', { name: '概览', exact: true }).click();
    await page.getByRole('region', { name: '宿主机资源' }).waitFor();
    await page.waitForFunction(() => window.pageEntries > 1);
    const content = page.locator('.page-view > *').first();
    await content.evaluate(async element => {
      await Promise.all(element.getAnimations().map(animation => animation.finished));
    });
    assert.equal(await content.evaluate(element => window.getComputedStyle(element).opacity), '1');
    assert.equal(await content.evaluate(element => window.getComputedStyle(element).transform), 'none');
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await nav.getByRole('button', { name: '房间', exact: true }).click();
    await page.getByRole('region', { name: '宿主机资源' }).waitFor({ state: 'hidden' });
    assert.equal(await content.evaluate(element => window.getComputedStyle(element).animationName), 'none');
  } finally { await page.close(); }
});

test('折叠导航只重排一次正文，图标不跳动，快速反向切换可恢复', async () => {
  const { page } = await openPanel();
  try {
    await page.emulateMedia({ reducedMotion: 'no-preference' });
    for (const expanded of [false, true]) {
      const result = await page.evaluate(async () => {
        const panel = document.querySelector('#page-content');
        const view = panel.firstElementChild.firstElementChild;
        const icon = document.querySelector('#app-navigation svg');
        const top = icon.getBoundingClientRect().top;
        const widths = [];
        const positions = [];
        document.querySelector('[data-sidebar="trigger"]').click();
        for (let frame = 0; frame < 24; frame++) {
          await new Promise(window.requestAnimationFrame);
          widths.push(panel.getBoundingClientRect().width);
          positions.push(panel.getBoundingClientRect().left);
        }
        return {
          widths, positions, top, finalTop: icon.getBoundingClientRect().top,
          sameView: view === panel.firstElementChild.firstElementChild,
          edge: document.querySelector('[data-slot="sidebar-container"]').getBoundingClientRect().right,
        };
      });
      assert.ok(Math.max(...result.widths) - Math.min(...result.widths) < 1, '正文宽度不随动画逐帧改变');
      assert.ok(Math.max(...result.positions) - Math.min(...result.positions) > 10, '正文仍有平滑位移动画');
      assert.equal(result.top, result.finalTop, '导航图标保持原来的垂直位置');
      assert.equal(result.sameView, true, '展开收起不重建业务页面');
      assert.ok(Math.abs(result.positions.at(-1) - result.edge) < 1, '动画结束后正文与侧栏边界对齐');
      assert.equal(await page.getByRole('button', { name: '切换导航', exact: true }).getAttribute('aria-expanded'), String(expanded));
    }
    await page.evaluate(async () => {
      const trigger = document.querySelector('[data-sidebar="trigger"]');
      trigger.click();
      await new Promise(resolve => window.setTimeout(resolve, 60));
      trigger.click();
    });
    await page.waitForFunction(() => document.querySelector('#page-content').getAnimations().length === 0);
    assert.equal(await page.getByRole('button', { name: '切换导航', exact: true }).getAttribute('aria-expanded'), 'true');
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.getByRole('button', { name: '切换导航', exact: true }).click();
    assert.equal(await page.locator('#page-content').evaluate(element => element.getAnimations().length), 0);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
  } finally { await page.close(); }
});

test('移动端导航仍使用抽屉，选页后关闭且不平移正文', async () => {
  const { page } = await openPanel({ width: 375 });
  try {
    await page.getByRole('button', { name: '切换导航', exact: true }).click();
    const drawer = page.locator('[data-mobile="true"]');
    await drawer.waitFor({ state: 'visible' });
    await drawer.getByRole('button', { name: '房间', exact: true }).click();
    await drawer.waitFor({ state: 'hidden' });
    assert.equal(await page.locator('#page-content').evaluate(element => window.getComputedStyle(element).transform), 'none');
    assert.equal(await page.getByRole('button', { name: '切换导航', exact: true }).getAttribute('aria-expanded'), 'false');
  } finally { await page.close(); }
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

test('容器内存独立显示限额和剩余空间，配额耗尽仍显示零且发出提醒', async () => {
  const data = { ...healthy, containerMemory: { totalBytes: 2 * GB, availableBytes: 0 } };
  const { page, section } = await openPanel({ data, width: 375 });
  try {
    await section.getByText('容器内存配额', { exact: true }).waitFor();
    assert.match(await section.innerText(), /剩余可用 \/ 限额 2.00 GB/);
    assert.match(await section.innerText(), /容器剩余内存配额/);
    assert.match(await section.innerText(), /16.00 GB/);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
  } finally { await page.close(); }
});
