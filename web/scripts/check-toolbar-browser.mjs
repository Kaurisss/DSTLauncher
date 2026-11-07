// 页面工具栏与设置提交回归；所有 API 写入仅修改本文件的隔离数据。
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

async function openPage(tab, width = 1280, long = false) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.addInitScript(({ tab, width }) => {
    localStorage.setItem('dst-tab', tab);
    localStorage.setItem('dst-room', '1');
    localStorage.setItem('dst-theme', width < 768 ? 'dark' : 'light');
  }, { tab, width });
  const state = {
    requests: [], errors: [],
    auth: { enabled: false, authenticated: true, envManaged: false },
    backupRoot: 'C:\\DST\\backups\\Cluster_1',
    automation: { config: { backupHours: 0, keep: 10, restartAt: '', updateMinutes: 0, updateMode: 'notify' }, jobs: {} },
  };
  page.on('pageerror', error => state.errors.push(error.message));
  await page.route('**/api/**', route => {
    const request = route.request(), path = new URL(request.url()).pathname;
    state.requests.push({ path, method: request.method(), body: request.postData() });
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/auth/status') return ok(state.auth);
    if (path === '/api/auth/password') { state.auth.enabled = !!request.postDataJSON().newPassword; return ok(state.auth); }
    if (path === '/api/rooms') return ok(Array.from({ length: long && tab === 'rooms' ? 12 : 2 }, (_, i) => i + 1).map(id => ({ id, key: `Cluster_${id}`, name: `工具栏测试房间 ${id}`, gameMode: 'survival', maxPlayers: 2, shards: [{ name: 'Master', isMaster: true }], needToken: false })));
    if (path === '/api/rooms/meta') return ok({ panelVersion: 'v0.1.2' });
    if (path === '/api/status') return ok([{ shard: 'Master', state: 'stopped' }]);
    if (path === '/api/install') return ok({ installed: true });
    if (path === '/api/server') return ok({ installed: true, candidates: [], archOptions: ['auto'] });
    if (path === '/api/backups') return ok(Array.from({ length: long ? 25 : 1 }, (_, i) => ({ name: long ? `fixture-${i}.zip` : 'fixture.zip', createdAt: '2026-10-26T12:00:00Z', size: 1024, path: state.backupRoot + '/fixture.zip' })));
    if (path === '/api/saves') return ok({ root: 'C:\\DST\\DST', items: Array.from({ length: long ? 25 : 2 }, (_, i) => i + 1).map(id => ({ cluster: `Cluster_${id}`, roomName: '测试房间', roomExists: id === 1, size: 1024, modTime: '2026-10-26T12:00:00Z', shards: [{ name: 'Master' }] })) });
    if (path === '/api/config') return ok({ clusterDir: 'C:\\DST\\DST\\Cluster_1', files: Array.from({ length: long ? 12 : 1 }, (_, i) => ({ path: long ? `Cluster_1/config_${i}.ini` : 'Cluster_1/cluster.ini', onDisk: true, content: '[NETWORK]\ncluster_name = fixture' })) });
    if (path === '/api/mods') return ok(Array.from({ length: long ? 32 : 2 }, (_, i) => i + 1).map(id => ({ id: `local-${id}`, name: `测试模组 ${id}`, enabled: id === 1, installed: true })));
    if (path === '/api/mods/update') return ok({ running: false, output: [] });
    if (path === '/api/chat-log') return ok({ entries: Array.from({ length: long ? 10 : 1 }, (_, i) => ({ id: i + 1, time: '00:01:00', kind: 'chat', userid: 'KU_FIXTURE', name: '测试玩家', message: long ? '测试聊天 ' + (i + 1) : '测试聊天' })), exists: true, total: long ? 10 : 1, page: 1, pageSize: 10 });
    if (path === '/api/automation') {
      if (request.method() === 'PUT') state.automation.config = request.postDataJSON();
      return ok(state.automation);
    }
    if (path === '/api/workshop/key') return ok({ steamApiKey: '' });
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    return ok([]);
  });
  await page.goto(origin);
  return { page, state };
}

for (const [tab, label] of [['rooms', '房间'], ['mods', '模组'], ['logs', '玩家日志'], ['backups', '备份'], ['saves', '存档'], ['config', '配置预览']]) {
  test(`${label}统计筛选与操作合并，桌面单行、手机不溢出`, async () => {
    for (const width of (['mods', 'logs'].includes(tab) ? [640, 768, 820, 900, 1024, 1280, 1536, 375, 320] : [1280, 375])) {
      const { page, state } = await openPage(tab, width);
      try {
        const toolbar = page.getByRole('region', { name: `${label}工具栏`, exact: true });
        await toolbar.waitFor();
        if (tab === 'rooms') await toolbar.getByText('共 2 个房间', { exact: true }).waitFor();
        if (tab === 'mods') await page.getByRole('article', { name: '测试模组 1', exact: true }).waitFor();
        if (tab === 'logs') await page.getByText('测试聊天', { exact: true }).waitFor();
        if (tab === 'backups') await toolbar.getByText('共 1 个备份', { exact: true }).waitFor();
        if (tab === 'saves') await toolbar.getByText(/共 2 个存档/).waitFor();
        if (tab === 'config') await toolbar.getByText('C:\\DST\\DST\\Cluster_1', { exact: true }).waitFor();
        const layout = await toolbar.evaluate(el => {
          const controls = [...el.querySelectorAll('button,input,[role="switch"]')].filter(control => control.getBoundingClientRect().height);
          return {
            centers: controls.map(control => { const box = control.getBoundingClientRect(); return box.y + box.height / 2; }),
            emptyHeaders: el.closest('[data-slot="card"]').querySelectorAll('[data-slot="card-header"]').length,
            noOverflow: el.scrollWidth <= el.clientWidth,
          };
        });
        assert.equal(layout.emptyHeaders, 0, '主卡片没有残留的空 Header 行');
        if (width >= 1024 || ['mods', 'logs'].includes(tab)) assert.ok(Math.max(...layout.centers) - Math.min(...layout.centers) < 2, `${label}工具栏保持单行（${width}px）`);
        assert.equal(layout.noOverflow, true, '工具栏不换行，也不产生横向滚动条');
        if (['mods', 'logs'].includes(tab) && width <= 900) {
          const trigger = toolbar.getByRole('button', { name: tab === 'mods' ? '模组筛选' : '日志筛选', exact: true });
          await trigger.click();
          const popover = page.getByRole('dialog', { name: tab === 'mods' ? '模组筛选' : '日志筛选', exact: true });
          await popover.waitFor();
          assert.equal(await popover.getByRole('combobox').count(), 2, '分片和类型整合在同一个筛选面板');
          if (width === 375) {
            await popover.getByRole('combobox', { name: tab === 'mods' ? '模组状态' : '记录类型' }).click();
            await page.getByRole('option', { name: tab === 'mods' ? /^已启用/ : '聊天', exact: tab === 'logs' }).click();
            await page.waitForFunction(label => document.activeElement?.getAttribute('aria-label') === label, tab === 'mods' ? '模组状态' : '记录类型');
          }
          await page.keyboard.press('Escape');
          await page.waitForFunction(label => document.activeElement?.getAttribute('aria-label') === label, tab === 'mods' ? '模组筛选' : '日志筛选');
          if (width === 375) {
            await page.setViewportSize({ width: 1280, height: 900 });
            if (tab === 'mods') {
              await page.waitForFunction(() => document.querySelector('.mods-filters button[aria-pressed="true"]')?.textContent.includes('已启用'));
            } else {
              await page.waitForFunction(() => document.querySelector('#chat_kind')?.textContent.includes('聊天'));
            }
            await page.setViewportSize({ width, height: 900 });
            await trigger.click();
            assert.match(await popover.getByRole('combobox', { name: tab === 'mods' ? '模组状态' : '记录类型' }).innerText(), tab === 'mods' ? /已启用/ : /聊天/);
            await page.keyboard.press('Escape');
          }
        }
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
        await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/toolbar-${tab}-${width}.png`, import.meta.url)) });
        assert.deepEqual(state.errors, []);
      } finally { await page.close(); }
    }
  });
}

for (const [tab, label] of [['rooms', '房间'], ['mods', '模组'], ['logs', '玩家日志'], ['backups', '备份'], ['saves', '存档'], ['config', '配置预览']]) {
  test(`${label}仅卡片列表滚动，外框、工具栏和Topbar固定，末行与底部边距可达`, async () => {
    for (const width of [1280, 375]) {
      const { page, state } = await openPage(tab, width, true);
      try {
        const list = page.locator('[data-slot="page-scroll"]');
        await list.waitFor();
        const last = ['rooms', 'mods'].includes(tab) ? list.getByRole('article').last()
          : tab === 'config' ? list.locator('[data-slot="code-block"]').last() : list.locator('tbody tr').last();
        if (tab === 'mods') await list.getByRole('article', { name: '测试模组 32', exact: true }).waitFor();
        else if (tab === 'logs') await list.getByText('测试聊天 10', { exact: true }).waitFor();
        else if (tab === 'rooms') await list.getByRole('heading', { name: '工具栏测试房间 12', exact: true }).waitFor();
        else if (tab === 'backups') await page.getByText('共 25 个备份', { exact: true }).waitFor();
        else if (tab === 'config') await list.getByText('Cluster_1/config_11.ini', { exact: true }).waitFor();
        else await list.getByText('Cluster_10', { exact: true }).waitFor();
        await page.setViewportSize({ width, height: 520 });
        await page.waitForFunction(() => document.querySelector('#page-content').getBoundingClientRect().bottom === window.innerHeight);
        const toolbar = page.getByRole('region', { name: label + '工具栏', exact: true });
        assert.equal(await toolbar.evaluate(el => window.getComputedStyle(el).boxShadow), 'none', '列表顶部不显示工具栏阴影');
        const before = await toolbar.boundingBox();
        const topbar = await page.locator('header[data-slot="app-header"]').boundingBox();
        const card = await list.locator('xpath=ancestor::*[@data-slot="card"]').boundingBox();
        const main = await page.locator('#page-content').boundingBox();
        assert.ok(card.y >= main.y + 11.5 && card.y + card.height <= main.y + main.height - 11.5, '卡片四周保留边距：' + JSON.stringify({ width, card, main }));
        assert.ok((await list.boundingBox()).y >= before.y + before.height - 1, '滚动条起点在工具栏下方');
        await list.hover();
        await page.mouse.wheel(0, 400);
        await page.waitForFunction(() => document.querySelector('[data-slot="page-scroll"]').scrollTop > 0);
        await page.waitForFunction(() => {
          const card = document.querySelector('[data-slot="page-scroll"]').closest('[data-slot="card"]');
          const toolbar = card.querySelector('.ui-toolbar, .mods-toolbar');
          return window.getComputedStyle(toolbar).boxShadow !== 'none';
        });
        await list.evaluate(el => { el.scrollTop = el.scrollHeight; });
        const after = await toolbar.boundingBox();
        assert.equal(after.y, before.y);
        assert.deepEqual(await page.locator('header[data-slot="app-header"]').boundingBox(), topbar);
        const final = await last.boundingBox(), bounds = await list.boundingBox();
        assert.ok(final.y >= bounds.y && final.y + final.height <= bounds.y + bounds.height + 1, '末行完整可见');
        const outer = await page.evaluate(() => ({
          window: window.scrollY, body: document.body.scrollTop, main: document.querySelector('#page-content').scrollTop,
          bodyHeight: document.body.scrollHeight, viewport: window.innerHeight,
          overflow: window.getComputedStyle(document.querySelector('#page-content')).overflowY,
          shellHeight: document.querySelector('[data-slot="sidebar-wrapper"]').getBoundingClientRect().height,
        }));
        assert.deepEqual([outer.window, outer.body, outer.main], [0, 0, 0]);
        assert.equal(outer.bodyHeight, outer.viewport);
        assert.equal(outer.shellHeight, outer.viewport);
        assert.equal(outer.overflow, 'hidden');
        await list.evaluate(el => { el.scrollTop = 0; });
        await page.waitForFunction(() => {
          const card = document.querySelector('[data-slot="page-scroll"]').closest('[data-slot="card"]');
          return window.getComputedStyle(card.querySelector('.ui-toolbar, .mods-toolbar')).boxShadow === 'none';
        });
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
        await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/app-shell-${tab}-${width}.png`, import.meta.url)) });
        assert.deepEqual(state.errors, []);
      } finally { await page.close(); }
    }
  });
}

test('多卡片设置页只滚动Topbar下方main，切换列表页后无残留外层滚动', async () => {
  const { page, state } = await openPage('settings');
  try {
    await page.getByRole('navigation', { name: '设置导航' }).getByRole('button', { name: '自动维护', exact: true }).click();
    await page.getByRole('button', { name: '保存计划', exact: true }).waitFor();
    await page.setViewportSize({ width: 1280, height: 360 });
    const topbar = page.locator('header[data-slot="app-header"]');
    const before = await topbar.boundingBox();
    const main = page.locator('#page-content');
    await main.hover();
    await page.mouse.wheel(0, 600);
    await page.waitForFunction(() => document.querySelector('#page-content').scrollTop > 0);
    assert.deepEqual(await topbar.boundingBox(), before);
    assert.equal(await page.evaluate(() => window.scrollY + document.body.scrollTop), 0);
    assert.ok(await main.evaluate(el => el.getBoundingClientRect().top) >= before.y + before.height);
    await page.getByRole('button', { name: '模组', exact: true }).click();
    await page.locator('[data-slot="page-scroll"]').waitFor();
    assert.equal(await main.evaluate(el => el.scrollTop), 0);
    assert.deepEqual(await topbar.boundingBox(), before);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('存档目录可复制，备份目录使用真实路径并兼容两种分隔符', async () => {
  const saves = await openPage('saves');
  try {
    await saves.page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    const copy = saves.page.getByRole('button', { name: '复制存档根目录', exact: true });
    await copy.click();
    assert.equal(await saves.page.evaluate(() => window.navigator.clipboard.readText()), 'C:\\DST\\DST');
  } finally { await saves.page.close(); }
  const { page, state } = await openPage('backups');
  try {
    const path = page.getByRole('region', { name: '备份工具栏' }).getByLabel('备份目录');
    await path.waitFor();
    assert.equal(await path.innerText(), 'C:\\DST\\backups\\Cluster_1');
    state.backupRoot = '/data/backups/Cluster_1';
    await page.reload();
    await path.waitFor();
    assert.equal(await path.innerText(), '/data/backups/Cluster_1');
  } finally { await page.close(); }
});

test('设置保存按钮位于各自Header，仍提交对应表单并保留原生校验及回车提交', async () => {
  const { page, state } = await openPage('settings');
  try {
    const settingsNav = page.getByRole('navigation', { name: '设置导航' });
    for (const [nav, name, form] of [['自动维护', '保存计划', 'automation-plan-form'], ['访问密码', '保存密码', 'settings-password-form'], ['API Key', '保存 Key', 'settings-api-key-form']]) {
      await settingsNav.getByRole('button', { name: nav, exact: true }).click();
      const button = page.getByRole('button', { name, exact: true });
      await button.waitFor();
      assert.equal(await button.evaluate(el => !!el.closest('[data-slot="card-header"]') && !el.closest('form')), true);
      assert.equal(await button.evaluate(el => el.form.id), form);
    }
    await settingsNav.getByRole('button', { name: '自动维护', exact: true }).click();
    const hours = page.getByLabel('所有房间备份间隔（小时，0 关闭）');
    await hours.fill('-1');
    await page.getByRole('button', { name: '保存计划', exact: true }).click();
    assert.equal(await page.getByRole('alertdialog').count(), 0);
    assert.equal(state.requests.filter(r => r.path === '/api/automation' && r.method === 'PUT').length, 0);
    await hours.fill('6');
    await hours.press('Enter');
    const confirmation = page.getByRole('alertdialog');
    await confirmation.getByRole('button', { name: '确定', exact: true }).click();
    await page.getByText('自动维护计划已保存', { exact: true }).waitFor();
    assert.equal(state.automation.config.backupHours, 6);
    await settingsNav.getByRole('button', { name: '访问密码', exact: true }).click();
    const password = page.getByRole('button', { name: '保存密码', exact: true });
    assert.equal(await password.isDisabled(), true);
    await page.locator('#set-new-pwd').fill('fixture-password');
    await password.click();
    await page.getByText('面板密码已保存', { exact: true }).waitFor();
    const savedPassword = state.requests.find(r => r.path === '/api/auth/password' && r.method === 'PUT');
    assert.deepEqual(JSON.parse(savedPassword.body), { currentPassword: '', newPassword: 'fixture-password' });
    await settingsNav.getByRole('button', { name: 'API Key', exact: true }).click();
    await page.locator('#set-steam-key').fill('0123456789abcdef0123456789abcdef');
    await page.locator('#set-steam-key').press('Enter');
    await page.getByText('Steam API Key 已保存', { exact: true }).waitFor();
    const savedKey = state.requests.find(r => r.path === '/api/workshop/key' && r.method === 'PUT');
    assert.equal(JSON.parse(savedKey.body).steamApiKey, '0123456789abcdef0123456789abcdef');
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});
