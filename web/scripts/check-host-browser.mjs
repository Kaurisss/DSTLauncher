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

async function openPanel({ data = healthy, width = 1280, room = null, roomList = null, running = false, dark = false, tab = 'overview' } = {}) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(12000);
  const state = { data, failure: false, requests: 0 };
  await page.addInitScript(({ id, dark, tab }) => {
    localStorage.setItem('dst-tab', tab);
    localStorage.setItem('dst-theme', dark ? 'dark' : 'light');
    if (id) localStorage.setItem('dst-room', String(id));
  }, { id: room?.id, dark, tab });
  await page.route('**/api/**', route => {
    const path = new URL(route.request().url()).pathname;
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/host') {
      state.requests++;
      if (state.failure) return route.fulfill({ json: { code: 500, message: '采样服务暂不可用' } });
      return ok(state.data);
    }
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok(roomList || (room ? [room] : []));
    if (path === '/api/status') return ok(running ? room.shards.map(shard => ({
      shard: shard.name, state: 'running', playersReady: true, uptime: '2h 15m',
      cpuPct: 1.2, memBytes: GB, pid: 1234, port: 10999,
    })) : []);
    if (path === '/api/worldstate') return ok(room.shards.map(shard => ({
      shard: shard.name, state: { day: 108, season: 'spring', remainingDays: 18, savedAt: '2026-10-26T12:00:00Z' },
    })));
    if (path === '/api/players') return ok({ players: [], errors: {} });
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    if (path === '/api/rooms/meta') return ok({ panelVersion: 'v0.1.2' });
    if (path === '/api/server') return ok({ installed: true, candidates: [] });
    if (path === '/api/install') return ok({ installed: true, version: '759671' });
    return ok([]);
  });
  await page.goto(origin);
  const section = page.getByRole('region', { name: '宿主机资源' });
  if (tab === 'overview') await section.getByText('宿主机内存', { exact: true }).waitFor();
  else await page.getByRole('heading', { level: 1 }).waitFor();
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

test('四分片概览的资源与快捷操作保持紧凑，桌面首屏可见日志且手机不溢出', async () => {
  const room = {
    id: 2, key: 'Cluster_1', name: 'Skr Wu的世界 单人 双穿', gameMode: 'survival', maxPlayers: 2,
    shards: [{ name: 'Master', isMaster: true }, { name: 'Caves' }, { name: 'Island' }, { name: 'Volcano' }],
    locations: { Master: 'forest', Caves: 'cave', Island: 'shipwrecked', Volcano: 'volcanoworld' },
  };
  for (const scenario of [
    { width: 2240, running: false, dark: false },
    { width: 1440, running: true, dark: true },
    { width: 375, running: true, dark: false },
  ]) {
    const { page, section } = await openPanel({ room, ...scenario });
    try {
      await page.emulateMedia({ reducedMotion: 'reduce' });
      await section.getByText('50.0%', { exact: true }).waitFor();
      await page.getByRole('region', { name: '分片状态', exact: true }).getByText('第 108 天', { exact: true }).first().waitFor();
      await page.getByRole('region', { name: '在线玩家', exact: true }).getByText(scenario.running ? '暂无在线玩家' : '房间未运行', { exact: true }).waitFor();
      const quick = page.getByRole('region', { name: '快捷操作', exact: true });
      assert.ok((await quick.boundingBox()).height < 180, '快捷操作不被拉高');
      for (const card of await section.locator('.stat').all()) {
        assert.ok((await card.boundingBox()).height < 150, '资源指标按内容取高');
        assert.equal(await card.locator('.stat-value').evaluate(el => el.scrollWidth <= el.clientWidth), true, '指标数字不截断');
      }
      if (scenario.width > 1100) {
        const logs = page.getByRole('region', { name: '实时日志', exact: true });
        assert.ok((await logs.boundingBox()).y < 850, '桌面首屏能看见日志入口');
        const left = await page.getByRole('heading', { name: '分片管理', exact: true }).locator('xpath=ancestor::*[@data-slot="card"]').boundingBox();
        const right = await page.getByRole('region', { name: '在线玩家', exact: true }).boundingBox();
        assert.ok(Math.abs(left.y + left.height - right.y - right.height) < 2, '主区左右底部对齐');
        assert.equal(await page.getByRole('region', { name: '世界状态', exact: true }).count(), 0);
        assert.equal(await logs.locator('.console').count(), 1);
      }
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
      if (scenario.running) {
        await page.getByRole('region', { name: '分片状态', exact: true }).getByText('运行中', { exact: true }).first().focus();
        const details = page.getByRole('tooltip');
        await details.waitFor();
        assert.match(await details.innerText(), /CPU（占整机）1\.2%.*内存 1\.00 GB/);
        assert.match(await details.innerText(), /PID 1234/);
        await page.keyboard.press('Escape');
      }
      await page.locator('#page-content').evaluate(el => { el.scrollTop = 0; });
      await page.mouse.move(0, 0);
      await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/overview-refactor-${scenario.width}-${scenario.dark ? 'dark' : 'light'}.png`, import.meta.url)) });
    } finally { await page.close(); }
  }
});

test('概览统一四项资源指标，服务器配置仅在设置页提供', async () => {
  const { page } = await openPanel();
  try {
    assert.deepEqual(await page.locator('.stat-label').allTextContents(), ['运行分片', '进程资源占用', '宿主机内存', '存档盘可用空间']);
    assert.equal(await page.getByRole('progressbar', { name: '宿主机内存使用率' }).getAttribute('aria-valuenow'), '50');
    assert.equal(await page.getByRole('progressbar', { name: '存档盘已用空间' }).getAttribute('aria-valuenow'), '60');
    assert.equal(await page.getByText('更改服务器文件位置', { exact: true }).count(), 0);
    assert.equal(await page.getByRole('button', { name: '检查更新', exact: true }).count(), 0);
    const nav = page.locator('#app-navigation');
    await nav.getByRole('button', { name: '设置', exact: true }).click();
    await page.getByRole('heading', { name: '服务器文件与架构', exact: true }).waitFor();
    await page.getByText('更改服务器文件位置', { exact: true }).click();
    await page.getByLabel('安装目录', { exact: true }).waitFor();
    await page.getByLabel('架构', { exact: true }).waitFor();
    await nav.getByRole('button', { name: '概览', exact: true }).click();
    await page.getByRole('region', { name: '宿主机资源' }).waitFor();
    assert.equal(await page.getByText('更改服务器文件位置', { exact: true }).count(), 0);
  } finally { await page.close(); }
});

test('Unified App Shell：Header 横跨顶部，Sidebar 位于左侧无缝连接，主内容以圆角 Inset 嵌入并独立滚动', async () => {
  const room = { id: 1, key: 'Cluster_1', name: 'Skr Wu的世界 单人 双穿', gameMode: 'survival', maxPlayers: 2, shards: [{ name: 'Master', isMaster: true }] };
  const other = { ...room, id: 2, key: 'Cluster_2', name: '备用房间', gameMode: 'endless', maxPlayers: 6 };
  const { page } = await openPanel({ room, roomList: [room, other], tab: 'config', width: 1440 });
  try {
    await page.emulateMedia({ reducedMotion: 'reduce' });
    const header = page.locator('header[data-slot="app-header"]');
    const headerBox = await header.boundingBox();
    assert.equal(headerBox.x, 0);
    assert.equal(headerBox.y, 0);
    assert.equal(headerBox.width, 1440);

    const sidebar = page.locator('[data-slot="sidebar-container"]');
    const footer = sidebar.locator('[data-slot="sidebar-footer"]');
    await header.getByText('DST 开服控制台', { exact: true }).waitFor();
    await footer.getByText('v0.1.2', { exact: true }).waitFor();
    await footer.getByText('游戏版本 759671', { exact: true }).waitFor();

    const sidebarBox = await sidebar.boundingBox();
    assert.equal(sidebarBox.x, 0);
    assert.equal(sidebarBox.y, headerBox.height);
    assert.equal(sidebarBox.width, 220);

    const main = page.locator('#page-content');
    const card = page.locator('.page-view > [data-slot="card"]');
    await card.waitFor();
    const mainBox = await main.boundingBox();
    assert.ok(mainBox.x >= sidebarBox.width);
    assert.ok(mainBox.y >= headerBox.height);

    const title = header.getByRole('heading', { level: 1, name: '配置预览', exact: true });
    assert.equal(await card.getByRole('heading', { name: '配置预览', exact: true }).count(), 0);

    const logStatus = footer.getByRole('status');
    const version = footer.getByText('游戏版本 759671', { exact: true });
    assert.ok(Math.abs((await logStatus.boundingBox()).y - (await version.boundingBox()).y) < 2, '底部状态保持单行');

    const selector = header.getByRole('combobox', { name: '房间', exact: true });
    assert.match(await selector.innerText(), /Skr Wu的世界 单人 双穿/);
    assert.match(await selector.innerText(), /Cluster_1 · 生存 · 2人/);
    assert.doesNotMatch(await header.innerText(), /survival/);
    assert.equal(await header.getByText('Cluster_1 · 生存 · 2人', { exact: true }).count(), 1);

    await selector.click();
    await page.getByRole('option', { name: '备用房间', exact: true }).click();
    assert.match(await selector.innerText(), /备用房间/);
    assert.match(await selector.innerText(), /Cluster_2 · 无尽 · 6人/);
    assert.equal(await page.evaluate(() => localStorage.getItem('dst-room')), '2');
    assert.equal(await title.isVisible(), true, '换房不改变当前页面');

    await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/layout-1440-light.png', import.meta.url)) });
    const lightText = await title.evaluate(el => window.getComputedStyle(el).color);
    await header.getByRole('button', { name: '切换主题', exact: true }).click();
    await page.waitForFunction(() => document.documentElement.dataset.theme === 'dark');
    await page.evaluate(async () => { await Promise.all(document.getAnimations().map(animation => animation.finished)); });
    assert.notEqual(await title.evaluate(el => window.getComputedStyle(el).color), lightText, '主题切换后标题前景色同步更新');
    await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/layout-1440-dark.png', import.meta.url)) });
  } finally { await page.close(); }
});
test('手机主标题和长房间选择器不溢出，品牌与版本在通顶抽屉内', async () => {
  const room = { id: 1, key: 'Cluster_1', name: 'Skr Wu的世界 单人 双穿 '.repeat(4), gameMode: 'survival', maxPlayers: 2, shards: [{ name: 'Master', isMaster: true }] };
  const { page } = await openPanel({ room, width: 375, tab: 'config' });
  try {
    const header = page.locator('header[data-slot="app-header"]');
    assert.equal(await header.getByRole('heading', { level: 1, name: '配置预览', exact: true }).isVisible(), true);
    assert.equal(await header.getByRole('button', { name: '切换主题', exact: true }).innerText(), '');
    assert.equal(await header.evaluate(el => el.scrollWidth <= el.clientWidth), true);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    await page.locator('.page-view > *').first().waitFor();
    await page.evaluate(async () => { await Promise.all(document.getAnimations().map(animation => animation.finished)); });
    await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/layout-375-light.png', import.meta.url)) });
    await header.getByRole('button', { name: '切换导航', exact: true }).click();
    const drawer = page.locator('[data-mobile="true"]');
    await drawer.getByText('DST 开服控制台', { exact: true }).waitFor();
    await drawer.getByText('v0.1.2', { exact: true }).waitFor();
    assert.equal((await drawer.boundingBox()).y, 0);
    await drawer.getByRole('button', { name: '房间', exact: true }).click();
    await drawer.waitFor({ state: 'hidden' });
    assert.equal(await header.getByRole('heading', { level: 1 }).innerText(), '房间');
    assert.equal(await page.locator('#new-room-button').isVisible(), true);
  } finally { await page.close(); }
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
    assert.equal(before, '切换主题：浅色');
    assert.equal(await theme.innerText(), '', '主题入口仅显示图标');
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

test('折叠导航只重排一次正文，收起按钮与菜单对齐且等距，快速反向切换可恢复', async () => {
  const { page } = await openPanel();
  try {
    await page.emulateMedia({ reducedMotion: 'no-preference' });
    for (const expanded of [false, true]) {
      const result = await page.evaluate(async () => {
        const panel = document.querySelector('#page-content');
        const view = panel.firstElementChild.firstElementChild;
        const icon = document.querySelector('#app-navigation svg');
        const widths = [];
        const positions = [];
        document.querySelector('[data-sidebar="trigger"]').click();
        for (let frame = 0; frame < 24; frame++) {
          await new Promise(window.requestAnimationFrame);
          widths.push(panel.getBoundingClientRect().width);
          positions.push(panel.getBoundingClientRect().left);
        }
        return {
          widths, positions,
          icon: icon.getBoundingClientRect().toJSON(),
          trigger: document.querySelector('[data-sidebar="trigger"] svg').getBoundingClientRect().toJSON(),
          nextIcon: document.querySelectorAll('#app-navigation svg')[1].getBoundingClientRect().toJSON(),
          sameView: view === panel.firstElementChild.firstElementChild,
          edge: document.querySelector('[data-slot="sidebar-container"]').getBoundingClientRect().right,
        };
      });
      assert.ok(Math.max(...result.widths) - Math.min(...result.widths) < 1, '正文宽度不随动画逐帧改变');
      assert.ok(Math.max(...result.positions) - Math.min(...result.positions) > 10, '正文仍有平滑位移动画');
      if (!expanded) {
        await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/layout-sidebar-collapsed.png', import.meta.url)) });
      }
      assert.equal(result.sameView, true, '展开收起不重建业务页面');
      assert.ok(Math.abs(result.positions.at(-1) - result.edge) < 1.5, '动画结束后正文与侧栏边界对齐');
      assert.equal(await page.getByRole('button', { name: '切换导航', exact: true }).getAttribute('aria-expanded'), String(expanded));
    }
    await page.evaluate(async () => {
      const trigger = document.querySelector('[data-sidebar="trigger"]');
      trigger.click();
      await new Promise(resolve => window.setTimeout(resolve, 60));
      trigger.click();
    });
    await page.waitForFunction(() => document.querySelector('[data-slot="main-workspace"]').getAnimations().length === 0);
    assert.equal(await page.getByRole('button', { name: '切换导航', exact: true }).getAttribute('aria-expanded'), 'true');
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.getByRole('button', { name: '切换导航', exact: true }).click();
    assert.equal(await page.locator('[data-slot="main-workspace"]').evaluate(element => element.getAnimations().length), 0);
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
