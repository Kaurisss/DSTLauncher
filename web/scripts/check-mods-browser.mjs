// 模组页回归：所有接口和封面均使用隔离 fixture，不读写真实房间或模组。
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
    server: { port: 0, host: '127.0.0.1' },
  });
  await vite.listen();
  origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch();
});
after(async () => {
  await browser?.close();
  await vite?.close();
});

const mods = [
  { id: 'workshop-1185229307', name: 'workshop-1185229307', enabled: true, installed: false },
  { id: 'workshop-1207269058', name: '本地已下载名称', enabled: false, installed: true },
  { id: 'workshop-1271089343', name: 'workshop-1271089343', enabled: false, installed: false },
  { id: 'local-test', name: '本地测试模组', enabled: false, installed: true },
];
const details = [
  { id: '1185229307', title: '工坊模组 <测试>', previewUrl: 'https://fixtures.invalid/cover.svg' },
  { id: '1207269058', title: '已下载的工坊标题', previewUrl: 'https://fixtures.invalid/broken.png' },
];

async function openMods({
  failDetails = false, running = false, delayDetails = false, delayMods = false, width = 1280,
  updateStatuses = [{ running: false, output: [] }],
} = {}) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(10000);
  await page.addInitScript(() => {
    localStorage.setItem('dst-tab', 'mods');
    localStorage.setItem('dst-room', '2');
  });
  let releaseDetails;
  let releaseMods;
  const waitForDetails = delayDetails ? new Promise(resolve => { releaseDetails = resolve; }) : Promise.resolve();
  const waitForMods = delayMods ? new Promise(resolve => { releaseMods = resolve; }) : Promise.resolve();
  const requests = [];
  let currentMods = globalThis.structuredClone(mods);
  let updateStatusIndex = 0;
  await page.route('https://fixtures.invalid/**', route => {
    if (route.request().url().endsWith('broken.png')) return route.fulfill({ status: 404, body: '' });
    return route.fulfill({
      contentType: 'image/svg+xml',
      body: '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="80"><rect width="80" height="80" fill="#397461"/></svg>',
    });
  });
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    requests.push({ path: url.pathname, params: Object.fromEntries(url.searchParams), method: route.request().method() });
    const ok = data => route.fulfill({
      contentType: 'application/json', body: JSON.stringify({ code: 200, data }),
    });
    if (url.pathname === '/api/rooms') return ok([{
      id: 2, name: '测试房间', key: 'Cluster_2', gameMode: 'survival', maxPlayers: 4,
      shards: [{ name: 'Master', worldType: 'forest', isMaster: true }],
    }]);
    if (url.pathname === '/api/status') return ok([{ shard: 'Master', state: running ? 'running' : 'stopped' }]);
    if (url.pathname === '/api/mods') {
      await waitForMods;
      return ok(currentMods);
    }
    if (url.pathname === '/api/mods/delete') {
      currentMods = currentMods.filter(mod => mod.id !== url.searchParams.get('id'));
      return ok({});
    }
    if (url.pathname === '/api/workshop/details') {
      await waitForDetails;
      if (failDetails) return route.fulfill({
        contentType: 'application/json', body: JSON.stringify({ code: 500, message: 'Steam 暂时不可达' }),
      });
      return ok(details);
    }
    if (url.pathname === '/api/mods/update') {
      if (route.request().method() === 'POST') return ok({ running: true, output: ['开始'] });
      if (route.request().method() === 'DELETE') return ok({});
      return ok(updateStatuses[Math.min(updateStatusIndex++, updateStatuses.length - 1)]);
    }
    if (url.pathname === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    return ok([]);
  });
  await page.goto(origin);
  await page.locator(delayMods ? '.mod-skeleton' : '.mod-entry-head').first().waitFor();
  return { page, requests, releaseDetails, releaseMods };
}

async function waitForTitle(page) {
  await page.getByRole('link', { name: '工坊模组 <测试>' }).waitFor();
}

test('未下载模组异步补全工坊名称、封面和安全的新标签页链接，刷新复用元数据', async () => {
  const { page, requests, releaseDetails } = await openMods({ delayDetails: true });
  try {
    assert.equal(await page.locator('.mod').count(), 4, '工坊慢响应不阻塞本地列表');
    assert.equal(await page.locator('.mod').first().getByRole('button', { name: '下载', exact: true }).isEnabled(), true);
    releaseDetails();
    await waitForTitle(page);
    const title = page.getByRole('link', { name: '工坊模组 <测试>' });
    assert.equal(await title.getAttribute('href'), 'https://steamcommunity.com/sharedfiles/filedetails/?id=1185229307');
    assert.equal(await title.getAttribute('target'), '_blank');
    assert.match(await title.getAttribute('rel'), /noopener/);
    const cover = page.locator('.mod').first().locator('img');
    assert.equal(await cover.getAttribute('src'), details[0].previewUrl);
    await page.waitForFunction(() => document.querySelector('.mod img')?.naturalWidth > 0);
    assert.match(await page.locator('.mod').nth(1).innerText(), /已下载的工坊标题/);
    assert.match(await page.locator('.mod').nth(2).innerText(), /workshop-1271089343/);
    assert.equal(await page.locator('.mod').nth(3).getByRole('link').count(), 0, '本地模组无伪造的工坊链接');
    const detailRequests = () => requests.filter(r => r.path === '/api/workshop/details');
    assert.equal(detailRequests().length, 1);
    assert.equal(detailRequests()[0].params.input, '1185229307,1207269058,1271089343');
    await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/api/mods'),
      page.getByRole('button', { name: '刷新', exact: true }).click(),
    ]);
    assert.equal(detailRequests().length, 1, '刷新列表不重复查 Steam');

    const toggle = page.locator('.mod').first().getByRole('switch');
    assert.equal(await toggle.getAttribute('aria-checked'), 'true');
    await toggle.click();
    await page.waitForFunction(() => document.querySelector('.mod [role="switch"]')?.getAttribute('aria-checked') === 'false');
    assert.equal(requests.find(r => r.path === '/api/mods/toggle')?.params.enabled, 'false');

    await page.locator('.mod').nth(1).getByRole('button', { name: '配置', exact: true }).click();
    await page.getByText('该模组没有可配置项。').waitFor();
    assert.equal(requests.some(r => r.path === '/api/mods/schema'), true);

    await page.setViewportSize({ width: 390, height: 844 });
    const overflow = await page.locator('.mod').evaluateAll(items => items.some(el => el.scrollWidth > el.clientWidth));
    assert.equal(overflow, false, '窄屏模组卡片不横向溢出');
  } finally {
    releaseDetails();
    await page.close();
  }
});

test('已下载模组删除：取消不发请求，确认后删除并刷新列表', async () => {
  const { page, requests } = await openMods();
  try {
    await waitForTitle(page);
    const item = page.locator('.mod').filter({ hasText: '已下载的工坊标题' });
    await item.getByRole('button', { name: '删除', exact: true }).click();
    const dialog = page.getByRole('alertdialog');
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(requests.filter(r => r.path === '/api/mods/delete').length, 0);
    await item.getByRole('button', { name: '删除', exact: true }).click();
    await dialog.getByRole('button', { name: '确定', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    await page.waitForLoadState('networkidle');
    const deletes = requests.filter(r => r.path === '/api/mods/delete');
    assert.equal(deletes.length, 1, '确认后应提交删除请求');
    assert.equal(deletes[0].params.id, 'workshop-1207269058');
    assert.equal(deletes[0].params.room, '2');
    assert.equal(await item.count(), 0, '已删除条目应从列表消失');
  } finally {
    await page.close();
  }
});

test('封面加载失败保留占位图，运行中冻结修改但标题仍可访问', async () => {
  const { page } = await openMods({ running: true });
  try {
    await waitForTitle(page);
    await page.getByText('服务器运行中，模组修改已冻结。', { exact: false }).waitFor();
    const broken = page.locator('.mod').nth(1);
    await page.waitForFunction(() => !document.querySelectorAll('.mod')[1].querySelector('img'));
    assert.equal(await broken.locator('.mod-cover svg').count(), 1);
    assert.equal(await broken.getByRole('switch').isDisabled(), true);
    assert.equal(await broken.getByRole('button', { name: '更新', exact: true }).isDisabled(), true);
    assert.equal(await broken.getByRole('button', { name: '删除', exact: true }).isDisabled(), true);
    assert.equal(await broken.getByRole('link').getAttribute('target'), '_blank');
  } finally {
    await page.close();
  }
});

test('封面列表的骨架行与真实条目在桌面、窄屏下保持同高和相同行距', async () => {
  for (const width of [1280, 390]) {
    const { page, releaseMods } = await openMods({ delayMods: true, width });
    try {
      const measure = selector => page.locator(selector).evaluateAll(items => {
        const rects = items.map(el => el.getBoundingClientRect());
        return { height: rects[0].height, gap: rects[1].top - rects[0].bottom };
      });
      const skeleton = await measure('.mod-skeleton .mod');
      releaseMods();
      await waitForTitle(page);
      const content = await measure('.mod');
      assert.ok(Math.abs(skeleton.height - content.height) <= 1, `${width}px：骨架 ${skeleton.height}px，内容 ${content.height}px`);
      assert.equal(skeleton.gap, content.gap, `${width}px：条目间距一致`);
    } finally {
      releaseMods();
      await page.close();
    }
  }
});

test('空闲更新状态不弹出全部模组均已在本地的成功提示，也不重复加载列表', async () => {
  const { page, requests } = await openMods();
  try {
    await page.waitForLoadState('networkidle');
    assert.equal(await page.getByText('全部模组均已在本地，无需更新', { exact: true }).count(), 0);
    assert.equal(requests.filter(r => r.path === '/api/mods').length, 1);
  } finally {
    await page.close();
  }
});

test('更新任务结束仍缺文件时，即使只有两行输出也应警告未下载而不是报成功', async () => {
  const { page } = await openMods({
    updateStatuses: [
      { running: true, output: ['开始'] },
      { running: false, output: ['开始', '结束'] },
    ],
  });
  try {
    const result = page.getByText(/全部模组均已在本地，无需更新|更新结束，但仍有/).first();
    await result.waitFor();
    assert.match(await result.innerText(), /更新结束，但仍有 1 个模组未下载/);
  } finally {
    await page.close();
  }
});

test('更新终端在模态框显示，关闭不停止轮询，可重开并中止任务', async () => {
  const { page, requests } = await openMods({
    updateStatuses: [
      { running: false, output: [] },
      { running: true, progress: '25%', output: ['开始下载', '正在处理模组'] },
    ],
  });
  try {
    await page.waitForLoadState('networkidle');
    await page.locator('.mod').first().getByRole('button', { name: '下载', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '模组更新', exact: true });
    await dialog.waitFor();
    await dialog.getByText('25%', { exact: true }).waitFor();
    assert.match(await dialog.locator('pre').innerText(), /正在处理模组/);
    assert.equal(await page.locator('#page-content pre.mod-update-log').count(), 0, '主列表不再承载终端');
    await page.keyboard.press('Escape');
    await dialog.waitFor({ state: 'hidden' });
    const reopen = page.getByRole('button', { name: '更新日志', exact: true });
    await page.waitForFunction(() => document.activeElement?.textContent?.trim() === '更新日志', null, { timeout: 3000 });
    assert.equal(await reopen.evaluate(el => document.activeElement === el), true, '关闭后焦点返回日志入口');
    await page.waitForResponse(r => new URL(r.url()).pathname === '/api/mods/update' && r.request().method() === 'GET');
    assert.equal(await dialog.count(), 0, '后台轮询不强行重开窗口');
    assert.equal(requests.some(r => r.path === '/api/mods/update' && r.method === 'DELETE'), false);
    await reopen.click();
    await dialog.waitFor();
    await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/api/mods/update' && r.request().method() === 'DELETE'),
      dialog.getByRole('button', { name: '中止', exact: true }).click(),
    ]);
    assert.equal(await dialog.isVisible(), true, '中止请求不隐藏终端');
    await page.setViewportSize({ width: 390, height: 844 });
    const bounds = await dialog.boundingBox();
    assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= 390 && bounds.y >= 0 && bounds.y + bounds.height <= 844);
  } finally {
    await page.close();
  }
});

test('历史更新输出只通过日志入口查看，进入页面不自动弹出模态框', async () => {
  const { page } = await openMods({ updateStatuses: [{ running: false, output: ['历史更新输出'] }] });
  try {
    await page.waitForLoadState('networkidle');
    assert.equal(await page.getByRole('dialog').count(), 0);
    assert.equal(await page.locator('pre.mod-update-log').count(), 0);
    await page.getByRole('button', { name: '更新日志', exact: true }).click();
    await page.getByRole('dialog').getByText('历史更新输出', { exact: true }).waitFor();
  } finally {
    await page.close();
  }
});

test('工坊查询失败保留本地名称、ID 和操作，刷新可以重试', async () => {
  const { page, requests } = await openMods({ failDetails: true });
  try {
    await page.getByText(/创意工坊信息暂未加载/).waitFor();
    assert.match(await page.locator('.mod').nth(1).innerText(), /本地已下载名称/);
    assert.equal(await page.locator('.mod').first().getByRole('button', { name: '下载', exact: true }).isEnabled(), true);
    assert.equal(await page.locator('.mod').first().getByRole('link').getAttribute('href'), 'https://steamcommunity.com/sharedfiles/filedetails/?id=1185229307');
    await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/api/workshop/details'),
      page.getByRole('button', { name: '刷新', exact: true }).click(),
    ]);
    assert.equal(requests.filter(r => r.path === '/api/workshop/details').length, 2);
  } finally {
    await page.close();
  }
});
