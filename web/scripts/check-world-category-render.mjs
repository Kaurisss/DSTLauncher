// 浏览器回归：需已安装 Playwright Chromium，或用 PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH 指定已有浏览器。
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)),
  server: { host: '127.0.0.1', port: 0, open: false },
});
let browser;
try {
  await vite.listen();
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
  const page = await browser.newPage({ viewport: { width: 1600, height: 1100 } });
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  let worldRequests = 0;
  await page.addInitScript(() => {
    localStorage.setItem('dst-tab', 'world');
    localStorage.setItem('dst-theme', 'light');
    window.EventSource = class { close() {} };
  });
  const groups = ['settings', 'worldgen'].flatMap(category => Array.from({ length: category === 'settings' ? 6 : 3 }, (_, group) => ({
    id: `${category}_${group}`, category, label: `${category} ${group}`,
    options: Array.from({ length: group === 0 ? (category === 'settings' ? 26 : 24) : 20 }, (_, i) => ({
      name: `${category}_${group * 30 + i}`, category, label: `${category}选项 ${group * 30 + i}`, default: 'default',
      values: category === 'settings' && group === 0 && i === 0 ? [] : [{ data: 'default', label: '默认' }, { data: 'more', label: '较多' }],
    })),
  })));
  await page.route('**/api/**', async route => {
    assert.equal(route.request().method(), 'GET', '回归不得写入真实房间数据');
    const path = new URL(route.request().url()).pathname;
    let data = {};
    if (path === '/api/rooms') data = [{ id: 1, name: '渲染回归', key: 'Cluster_1', shards: [{ name: 'Master', worldType: 'forest' }] }];
    if (path === '/api/status') data = [];
    if (path === '/api/worldconfig') {
      worldRequests++;
      data = { file: 'fixture/worldgenoverride.lua', hasSave: false, groups, presets: {}, config: {
        overrideEnabled: true, worldgenPreset: '', settingsPreset: '', overrides: { settings_0: 'saved-draft' },
      } };
    }
    await route.fulfill({ json: { code: 200, data } });
  });
  await page.goto(vite.resolvedUrls.local[0]);
  await page.locator('#opt-settings_0').waitFor();
  assert.equal(await page.locator('.opt-group .opt-row').count(), 126);
  const select = page.locator('#opt-settings_1');
  await select.click();
  await page.getByRole('option', { name: '较多', exact: true }).click();
  assert.equal((await select.innerText()).trim(), '较多', '新增草稿后下拉框立即显示新值');
  assert.ok((await page.locator('.opt-row:has(#opt-settings_1)').getAttribute('class')).includes('is-changed'));
  await select.click();
  assert.equal(await page.getByRole('option', { name: '较多', exact: true }).getAttribute('data-state'), 'checked');
  await page.getByRole('option', { name: '默认', exact: true }).click();
  assert.equal((await select.innerText()).trim(), '默认');
  assert.equal((await page.locator('.opt-row:has(#opt-settings_1)').getAttribute('class')).includes('is-changed'), false);
  await select.click();
  await page.getByRole('option', { name: '较多', exact: true }).focus();
  await page.keyboard.press('Enter');
  assert.equal((await select.innerText()).trim(), '较多', '删除草稿后再次用键盘选择仍能更新');
  const onlyChanged = page.locator('.world-toolbar-sub').getByRole('switch');
  await onlyChanged.click();
  assert.equal(await page.locator('.opt-group .opt-row').count(), 2);
  await select.click();
  await page.getByRole('option', { name: '默认', exact: true }).click();
  assert.equal(await select.count(), 0, '恢复默认后立即退出只看已改动列表');
  await onlyChanged.click();
  await select.click();
  await page.getByRole('option', { name: '较多', exact: true }).click();
  await page.locator('#opt-settings_0').fill('unsaved-draft');
  const requestsBeforeSwitch = worldRequests;
  const firstFrame = await page.evaluate(async () => {
    const start = window.performance.now();
    document.querySelectorAll('.seg-btn')[1].click();
    await Promise.resolve();
    return new Promise(resolve => requestAnimationFrame(() => resolve({
      ms: window.performance.now() - start,
      skeleton: !!document.querySelector('.world-category-skeleton'),
      rows: document.querySelectorAll('.opt-group .opt-row').length,
      selected: document.querySelectorAll('.seg-btn')[1].getAttribute('aria-pressed'),
    })));
  });
  console.log('first frame:', firstFrame);
  assert.equal(firstFrame.skeleton, true, '切换后第一帧先呈现骨架屏');
  assert.equal(firstFrame.rows, 0, '新选项不得阻塞骨架屏的首帧');
  assert.equal(firstFrame.selected, 'true');
  await page.waitForFunction(() => document.querySelectorAll('.opt-group .opt-row').length === 64);
  assert.equal(worldRequests, requestsBeforeSwitch, '分类切换不重复请求接口');

  // 暂停应用的 rAF，确定性验证连续切换和卸载时的清理，而非靠定时器碰运气。
  await page.evaluate(() => {
    window.originalRAF = window.requestAnimationFrame;
    window.originalCancelRAF = window.cancelAnimationFrame;
    window.frames = new Map();
    let id = 0;
    window.requestAnimationFrame = callback => { window.frames.set(++id, callback); return id; };
    window.cancelAnimationFrame = key => window.frames.delete(key);
    window.advanceFrame = () => {
      const callbacks = [...window.frames.values()];
      window.frames.clear();
      callbacks.forEach(callback => callback(window.performance.now()));
    };
  });
  for (const index of [0, 1, 0]) {
    await page.evaluate(async index => { document.querySelectorAll('.seg-btn')[index].click(); await Promise.resolve(); }, index);
    assert.equal(await page.locator('.world-category-skeleton').count(), 1);
    assert.equal(await page.evaluate(() => window.frames.size), 1, '过期渲染帧已取消');
  }
  const placeholdersVisible = await page.locator('.world-category-skeleton [data-slot="skeleton"]').evaluateAll(elements =>
    elements.every(el => window.getComputedStyle(el).backgroundColor !== window.getComputedStyle(el.closest('.opt-row')).backgroundColor));
  assert.equal(placeholdersVisible, true, '图标、标签和控件占位不能融入卡片背景');
  await page.locator('.world-options-card').screenshot({ path: fileURLToPath(new URL('../../.pi/world-category-skeleton.png', import.meta.url)) });
  await page.setViewportSize({ width: 390, height: 844 });
  const mobile = await page.locator('.world-category-skeleton').evaluate(el => ({
    columns: window.getComputedStyle(el.querySelector('.opt-group-body')).gridTemplateColumns.split(' ').length,
    icon: el.querySelector('.opt-row-icon').getBoundingClientRect().width,
    overflow: el.scrollWidth > el.clientWidth,
  }));
  assert.deepEqual(mobile, { columns: 1, icon: 56, overflow: false }, '窄屏骨架布局匹配真实选项');
  await page.setViewportSize({ width: 1600, height: 1100 });
  await page.evaluate(async () => { window.advanceFrame(); await Promise.resolve(); });
  assert.equal(await page.locator('.world-category-skeleton').count(), 1, '保留一帧供骨架屏绘制');
  await page.evaluate(async () => { window.advanceFrame(); await Promise.resolve(); });
  assert.equal(await page.locator('.opt-group .opt-row').count(), 126);
  assert.equal(await page.locator('#opt-settings_0').inputValue(), 'unsaved-draft', '分类切换保留编辑草稿');
  assert.equal((await select.innerText()).trim(), '较多', '分类切换保留下拉框草稿');
  await page.evaluate(async () => { document.querySelectorAll('.seg-btn')[0].click(); await Promise.resolve(); });
  assert.equal(await page.locator('.world-category-skeleton').count(), 0, '重复点击当前分类不闪骨架屏');
  await page.evaluate(async () => {
    document.querySelectorAll('.seg-btn')[1].click();
    await Promise.resolve();
    [...document.querySelectorAll('#app-navigation button')].find(button => button.textContent.includes('房间')).click();
    await Promise.resolve();
  });
  assert.equal(await page.evaluate(() => window.frames.size), 0, '离开页面时取消待渲染帧');
  await page.evaluate(() => {
    window.requestAnimationFrame = window.originalRAF;
    window.cancelAnimationFrame = window.originalCancelRAF;
  });
  assert.deepEqual(errors, []);
  console.log('PASS: 下拉选值/恢复默认/键盘/改动过滤、190项首帧骨架屏、连续切换、草稿保留、不重复请求、卸载清理');
} finally {
  await browser?.close();
  await vite.close();
}
