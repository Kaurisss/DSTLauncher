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
  await page.emulateMedia({ reducedMotion: 'reduce' });
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
  assert.deepEqual(await page.locator('.seg-count').allInnerTexts(), ['126', '64']);
  assert.equal(await page.getByRole('navigation', { name: '配置分组' }).getByRole('button').count(), 6);
  assert.equal(await page.locator('.opt-group .opt-row').count(), 26);
  const row = page.locator('.opt-row:has(#opt-settings_1)');
  const value = row.locator('.opt-row-value');
  const previous = row.getByRole('button', { name: 'settings选项 1 上一项', exact: true });
  const next = row.getByRole('button', { name: 'settings选项 1 下一项', exact: true });
  await next.click();
  assert.equal((await value.innerText()).trim(), '较多', '箭头选值后立即显示新值');
  assert.ok((await row.getAttribute('class')).includes('is-changed'));
  await previous.click();
  assert.equal((await value.innerText()).trim(), '默认');
  assert.equal((await row.getAttribute('class')).includes('is-changed'), false);
  await next.focus();
  await page.keyboard.press('Enter');
  assert.equal((await value.innerText()).trim(), '较多', '删除草稿后再次用键盘选择仍能更新');
  const onlyChanged = page.getByRole('switch', { name: '只看已改动', exact: true });
  await onlyChanged.click();
  assert.equal(await page.locator('.opt-group .opt-row').count(), 2);
  await previous.click();
  assert.equal(await row.count(), 0, '恢复默认后立即退出只看已改动列表');
  await onlyChanged.click();
  await next.click();
  await page.locator('#opt-settings_0').fill('unsaved-draft');
  const requestsBeforeSwitch = worldRequests;
  // 数据早已在内存，切换必须在同一帧内换完，不得插入骨架屏或先清空再重建。
  const firstFrame = await page.evaluate(async () => {
    document.querySelectorAll('.seg-btn')[1].click();
    await Promise.resolve();
    return {
      skeleton: !!document.querySelector('.world-category-skeleton'),
      rows: document.querySelectorAll('.opt-group .opt-row').length,
      selected: document.querySelectorAll('.seg-btn')[1].getAttribute('aria-pressed'),
    };
  });
  console.log('first frame:', firstFrame);
  assert.equal(firstFrame.skeleton, false, '分类切换不再插骨架屏');
  assert.equal(firstFrame.rows, 24, '新分类首组在同一帧内就位，不先清空');
  assert.equal(firstFrame.selected, 'true');
  assert.equal(worldRequests, requestsBeforeSwitch, '分类切换不重复请求接口');

  // 连续快速切换必须停在最后一个分类上。
  for (const index of [0, 1, 0]) {
    await page.evaluate(async index => { document.querySelectorAll('.seg-btn')[index].click(); await Promise.resolve(); }, index);
    assert.equal(await page.locator('.world-category-skeleton').count(), 0, '切换过程不出现骨架屏');
  }
  assert.equal(await page.locator('.opt-group .opt-row').count(), 26);
  assert.equal(await page.locator('#opt-settings_0').inputValue(), 'unsaved-draft', '分类切换保留编辑草稿');
  assert.equal((await value.innerText()).trim(), '较多', '分类切换保留选项草稿');
  await page.evaluate(async () => { document.querySelectorAll('.seg-btn')[0].click(); await Promise.resolve(); });
  assert.equal(await page.locator('.opt-group .opt-row').count(), 26, '重复点当前分类仍保留首组');
  const navigation = page.getByRole('navigation', { name: '配置分组' });
  let total = 0;
  for (let group = 0; group < 6; group++) {
    await navigation.getByRole('button', { name: `settings ${group}`, exact: group !== 0 }).click();
    total += await page.locator('.opt-group .opt-row').count();
  }
  assert.equal(total, 126, '每个分组均可到达，未丢失任何配置项');
  await page.getByPlaceholder('搜索选项（中英文均可）').fill('settings选项 31');
  await page.locator('.opt-row-label').getByText('settings选项 31', { exact: true }).waitFor();
  assert.equal(await page.locator('.opt-row').count(), 1, '搜索可自动定位其他分组的选项');
  assert.equal(await navigation.getByRole('button', { name: 'settings 1', exact: true }).getAttribute('aria-current'), 'true');
  assert.deepEqual(errors, []);
  console.log('PASS: 箭头选值/恢复默认/键盘/改动过滤、同帧分类切换、连续切换、草稿保留、不重复请求');
  await page.close();
} finally {
  await browser?.close();
  await vite.close();
}
