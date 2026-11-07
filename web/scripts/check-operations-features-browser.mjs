import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let vite, browser, origin;
before(async () => {
  vite = await createServer({ root: fileURLToPath(new URL('..', import.meta.url)), server: { host: '127.0.0.1', port: 0 } });
  await vite.listen();
  origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => { await browser?.close(); await vite?.close(); });

function room(id, name = '配置测试房间') {
  return { id, name, key: `Cluster_${id}`, gameMode: 'endless', language: 'zh', maxPlayers: 6, maxSnapshots: 10, tickRate: 30,
    masterIp: '127.0.0.1', offline: false, needToken: true, running: false, hasSave: false,
    shards: [{ name: 'Master', isMaster: true, worldType: 'forest', gameId: 1 }, { name: 'Caves', worldType: 'cave', gameId: 2 }], statuses: [] };
}

async function openPage(tab, width = 1280, roomCount = 1) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(10000);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.addInitScript(tab => { localStorage.setItem('dst-tab', tab); localStorage.setItem('dst-room', '1'); }, tab);
  const state = { rooms: Array.from({ length: roomCount }, (_, index) => room(index + 1, index === 0 ? '配置测试房间' : '另一个房间')), profiles: [], writes: [], errors: [], notification: { enabled: false, url: '', secretSet: false, events: ['crash_limit', 'backup_failed'], rooms: [], results: [] } };
  page.on('pageerror', error => state.errors.push(error.message));
  await page.route('**/api/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname, method = req.method();
    const ok = data => route.fulfill({ json: { code: 200, data } });
    const body = method === 'POST' || method === 'PUT' ? req.postDataJSON() : null;
    if (method !== 'GET') state.writes.push({ path, method, body });
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok(state.rooms);
    if (path === '/api/rooms/meta') return ok({ defaults: room(0), gameModes: ['endless'], languages: ['zh'], tickRates: [30] });
    if (path === '/api/status') return ok([]);
    if (path === '/api/host') return ok({ memory: { totalBytes: 16e9, availableBytes: 12e9 }, disk: { totalBytes: 100e9, availableBytes: 80e9 } });
    if (path === '/api/server' || path === '/api/install') return ok({ installed: true, candidates: [] });
    if (path === '/api/players') return ok({ players: [], errors: {} });
    if (path === '/api/admins') return ok({ admins: [], seen: [], file: 'adminlist.txt' });
    if (path === '/api/player-lists') return ok({ ids: [], file: 'list.txt', reservedSlots: 0 });
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    if (path === '/api/profiles') {
      if (method === 'POST') { const p = { id: 'saved', name: body.name, shards: 2 }; state.profiles.push(p); return ok(p); }
      return ok(state.profiles);
    }
    if (path === '/api/profiles/import') { const p = { id: 'imported', name: body.name, shards: 2 }; state.profiles.push(p); return ok(p); }
    if (path === '/api/profiles/apply' || path === '/api/rooms/copy') { const created = room(state.rooms.length + 1, body.name); state.rooms.push(created); return ok(created); }
    if (path === '/api/diagnostics') return ok({ time: new Date().toISOString(), room: 1, checks: [], shards: [{ shard: 'Master', state: 'stopped', findings: [{ code: 'lua', stage: '脚本加载或运行', title: 'Lua 脚本报错', suggestion: '检查明确关联的模组', evidence: ['mods/workshop-123456/modmain.lua:10: <img src=x onerror="window.injected=true">'], mods: [{ id: 'workshop-123456', name: '故障模组' }] }] }], downloads: { findings: [] } });
    if (path === '/api/notifications') {
      if (method === 'PUT') { state.notification = { ...state.notification, ...body, secretSet: !!body.secret || state.notification.secretSet }; delete state.notification.secret; }
      return ok(state.notification);
    }
    if (path === '/api/notifications/test') {
      state.notification.results = [{ event: 'test', time: new Date().toISOString(), status: 503, error: '接收端返回 HTTP 503' }];
      return route.fulfill({ json: { code: 500, message: '接收端返回 HTTP 503' } });
    }
    if (path === '/api/automation') return ok({ config: { backupHours: 0, keep: 10, restartAt: '', updateMinutes: 0, updateMode: 'notify' }, jobs: {} });
    if (path === '/api/workshop/key') return ok({ steamApiKey: '' });
    return ok([]);
  });
  await page.goto(origin);
  return { page, state };
}

test('配置方案保存、导入、导出入口和应用后进入新房间编辑', async () => {
  const { page, state } = await openPage('rooms', 1280, 2);
  try {
    await page.getByRole('button', { name: '配置方案', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '配置方案', exact: true });
    assert.equal(await dialog.locator('#profile-source').evaluate(el => el.tagName), 'BUTTON', '源房间使用项目 Select 组件');
    await dialog.getByRole('combobox', { name: '源房间', exact: true }).click();
    await page.getByRole('option', { name: '另一个房间', exact: true }).click();
    await dialog.getByLabel('方案名称', { exact: true }).fill('海难四层方案');
    await dialog.getByRole('button', { name: '保存为方案', exact: true }).click();
    await dialog.getByText('海难四层方案', { exact: true }).waitFor();
    await dialog.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/room-profiles.png', import.meta.url)) });
    assert.deepEqual(state.writes.find(item => item.path === '/api/profiles').body, { room: 2, name: '海难四层方案' });
    assert.equal(await dialog.getByRole('link', { name: '导出海难四层方案', exact: true }).getAttribute('href'), '/api/profiles?id=saved');
    await dialog.locator('input[type="file"]').setInputFiles({ name: 'profile.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify({ version: 1, name: '导入方案', room: room(0), files: {} })) });
    await dialog.getByText('导入方案', { exact: true }).waitFor();
    await dialog.getByRole('button', { name: '从导入方案新建房间', exact: true }).click();
    await page.getByRole('textbox', { name: '房间名', exact: true }).waitFor();
    assert.equal(await page.getByRole('textbox', { name: '房间名', exact: true }).inputValue(), '导入方案 副本');
    assert.equal(state.writes.find(item => item.path === '/api/profiles/apply').body.id, 'imported');
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('复制房间使用源房间 ID，窄屏方案对话框不溢出', async () => {
  const { page, state } = await openPage('rooms', 375);
  try {
    await page.getByRole('button', { name: '配置方案', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '配置方案', exact: true });
    await dialog.getByRole('button', { name: '复制房间', exact: true }).click();
    await page.getByRole('textbox', { name: '房间名', exact: true }).waitFor();
    assert.equal(state.writes.find(item => item.path === '/api/rooms/copy').body.room, 1);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('诊断展示工坊链接与原始日志，日志内容按文本显示', async () => {
  const { page, state } = await openPage('overview');
  try {
    await page.getByRole('button', { name: '故障诊断', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '故障诊断', exact: true });
    await dialog.getByText('Lua 脚本报错', { exact: true }).waitFor();
    assert.equal(await dialog.getByRole('heading', { name: '森林', exact: true }).count(), 1, '诊断分组显示正确的世界名称');
    assert.equal(await dialog.getByRole('link', { name: '故障模组' }).getAttribute('href'), 'https://steamcommunity.com/sharedfiles/filedetails/?id=123456');
    await dialog.getByText('原始日志依据', { exact: true }).click();
    await dialog.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/diagnostics.png', import.meta.url)) });
    assert.match(await dialog.locator('pre').innerText(), /workshop-123456/);
    assert.equal(await page.evaluate(() => window.injected), undefined);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('通知保存事件、房间和密钥，测试失败反馈保留可见', async () => {
  const { page, state } = await openPage('settings', 375);
  try {
    await page.getByRole('navigation', { name: '设置导航' }).getByRole('button', { name: '事件通知', exact: true }).click();
    await page.getByRole('textbox', { name: 'Webhook 地址', exact: true }).fill('https://example.com/hook');
    await page.getByLabel('签名密钥', { exact: true }).fill('test-signing-secret');
    await page.getByRole('switch', { name: '启用通知', exact: true }).click();
    await page.getByRole('checkbox', { name: '配置测试房间', exact: true }).click();
    await page.getByRole('button', { name: '保存通知设置', exact: true }).click();
    await page.waitForFunction(() => document.querySelector('#notification-secret').placeholder.includes('已保存'));
    const sent = state.writes.find(item => item.path === '/api/notifications');
    assert.equal(sent.body.secret, 'test-signing-secret');
    assert.equal(sent.body.enabled, true);
    assert.deepEqual(sent.body.rooms, [1]);
    await page.getByRole('button', { name: '测试已保存的通知设置', exact: true }).click();
    await page.getByRole('region', { name: '最近通知结果' }).getByText('接收端返回 HTTP 503', { exact: true }).waitFor();
    await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/notification-settings-375.png', import.meta.url)) });
    assert.equal(await page.getByLabel('签名密钥', { exact: true }).inputValue(), '');
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});
