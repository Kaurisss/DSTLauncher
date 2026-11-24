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

async function openPage(tab, width = 1280) {
  const page = await browser.newPage({ viewport: { width, height: 900 } });
  page.setDefaultTimeout(10000);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.addInitScript(tab => { localStorage.setItem('dst-tab', tab); localStorage.setItem('dst-room', '1'); }, tab);
  const rooms = [1, 2].map(id => ({ id, key: `Cluster_${id}`, name: id === 1 ? '迁移测试房间' : '另一房间', running: false,
    gameMode: 'endless', locations: { Master: 'forest', Caves: 'cave' }, shards: [{ name: 'Master', isMaster: true }, { name: 'Caves', disabled: true }], statuses: [] }));
  const state = { writes: [], errors: [], mapCalls: [], delayOldMap: false, job: null };
  const fixturePng = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEUlEQVR4nAAEAPv/Aii+RgMAAkYBL+tbOigAAAAASUVORK5CYII=', 'base64');
  const fixtureOceanPng = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEUlEQVR4nAAEAPv/Ah5a0gMAAewBTc+LjegAAAAASUVORK5CYII=', 'base64');
  const mapTiles = Buffer.alloc(400 * 320 * 2);
  for (let y = 0; y < 320; y++) for (let x = 0; x < 400; x++) mapTiles.writeUInt16LE(x < 200 ? 7 : 10504, 2 * (y * 400 + x));
  const preview = { createdAt: '2026-11-07T10:00:00Z', rooms: rooms.map(room => ({ name: room.name, shards: room.shards })), profiles: 2, files: 42, bytes: 1024 * 1024, hasPlan: true };
  page.on('pageerror', error => state.errors.push(error.message));
  await page.route('**/api/**', async route => {
    const req = route.request(), url = new URL(req.url()), path = url.pathname, method = req.method();
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (method !== 'GET') state.writes.push({ path, method, body: path === '/api/migration/apply' ? req.postDataJSON() : req.postData() });
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok(rooms);
    if (path === '/api/status') return ok([]);
    if (path === '/api/saves') return ok({ root: 'isolated/DST', items: [] });
    if (path === '/api/backups') return ok([]);
    if (path === '/api/server' || path === '/api/install') return ok({ installed: true, candidates: [] });
    if (path === '/api/host') return ok({ memory: { totalBytes: 16e9, availableBytes: 12e9 }, disk: { totalBytes: 100e9, availableBytes: 80e9 } });
    if (path === '/api/migration/export') { state.job = { id: 'export-job', kind: 'export', state: 'ready', preview }; return ok({ id: state.job.id }); }
    if (path === '/api/migration/upload') { state.job = { id: 'upload-job', kind: 'upload', state: 'ready', preview }; return ok({ id: state.job.id }); }
    if (path === '/api/migration/apply') { state.job = { ...state.job, kind: 'import', roomIds: [3, 4] }; return ok({ id: state.job.id }); }
    if (path === '/api/migration/job') { if (method === 'DELETE') return ok(null); return ok(state.job); }
    if (path === '/api/map/texture') {
      return route.fulfill({
        contentType: 'image/png',
        body: url.searchParams.get('tile') === 'OCEAN_MEDIUM' ? fixtureOceanPng : fixturePng,
      });
    }
    if (path === '/api/map/icon') return route.fulfill({ contentType: 'image/png', body: fixturePng });
    if (path === '/api/map') {
      state.mapCalls.push({ room: url.searchParams.get('room'), shard: url.searchParams.get('shard') });
      const cave = url.searchParams.get('shard') === 'Caves';
      if (cave) return route.fulfill({ json: { code: 500, message: '此分片尚无世界保存点' } });
      return ok({ width: 400, height: 320, savedAt: '2026-11-07T09:00:00Z', kind: 'terrain',
        terrain: { tiles: mapTiles.toString('base64'), specs: [{ id: 7, name: 'GRASS' }, { id: 10504, name: 'OCEAN_MEDIUM' }] },
        textures: ['GRASS', 'OCEAN_MEDIUM'], icons: ['pigking'],
        landmarks: [{ id: 'pigking:0', name: '猪王', kind: 'pigking', x: 140, y: 150 }, { id: 'wormhole:0', name: '虫洞 1', kind: 'wormhole', x: 220, y: 180, target: 'wormhole:1' }, { id: 'wormhole:1', name: '虫洞 1', kind: 'wormhole', x: 90, y: 220, target: 'wormhole:0' }] });
    }
    return ok([]);
  });
  await page.goto(origin);
  return { page, state };
}

test('完整迁移可导出下载，清除临时包后上传预览，默认保留本机计划', async () => {
  const { page, state } = await openPage('backups');
  try {
    await page.getByRole('button', { name: '完整迁移', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '完整迁移', exact: true });
    await dialog.getByRole('button', { name: '导出全部房间', exact: true }).click();
    const download = dialog.getByRole('link', { name: '下载迁移包', exact: true });
    await download.waitFor();
    assert.equal(await download.getAttribute('href'), '/api/migration/download?id=export-job');
    await dialog.getByRole('button', { name: '清除临时包', exact: true }).click();
    await dialog.locator('input[type="file"]').setInputFiles({ name: 'migration.zip', mimeType: 'application/zip', buffer: Buffer.from('isolated-zip-fixture') });
    await dialog.getByRole('list', { name: '迁移房间清单' }).waitFor();
    assert.equal(await dialog.getByRole('checkbox', { name: '应用迁移包中的自动维护计划' }).isChecked(), false);
    await dialog.getByRole('button', { name: '导入为新房间', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await dialog.getByText('已创建 2 个房间。请检查设置并下载所需模组，再启动游戏。', { exact: true }).waitFor();
    assert.deepEqual(state.writes.find(item => item.path === '/api/migration/apply').body, { id: 'upload-job', applyPlan: false });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('手机迁移预览不溢出，取消确认不导入，勾选维护计划后明确提交', async () => {
  const { page, state } = await openPage('backups', 375);
  try {
    await page.getByRole('button', { name: '完整迁移', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '完整迁移', exact: true });
    await dialog.locator('input[type="file"]').setInputFiles({ name: 'migration.zip', mimeType: 'application/zip', buffer: Buffer.from('zip') });
    await dialog.getByRole('checkbox').check();
    await dialog.getByRole('button', { name: '导入为新房间', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(state.writes.some(item => item.path === '/api/migration/apply'), false);
    await dialog.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/migration-375.png', import.meta.url)) });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    await dialog.getByRole('button', { name: '导入为新房间', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await dialog.getByText(/已创建 2 个房间/).waitFor();
    assert.equal(state.writes.find(item => item.path === '/api/migration/apply').body.applyPlan, true);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('地形地图复用 DMP 配色，使用自定义 Select，可定位地标并切换分片', async () => {
  const { page, state } = await openPage('saves');
  try {
    await page.getByRole('button', { name: '地图预览', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '存档地图预览', exact: true });
    await dialog.getByRole('region', { name: '存档地形地图' }).waitFor();
    assert.equal(state.mapCalls.length, 1, '打开一次仅请求一次地图');
    assert.equal(await dialog.getByRole('combobox', { name: '地图房间' }).evaluate(el => el.tagName), 'BUTTON');
    assert.equal(await dialog.locator('svg polygon').count(), 0);
    assert.match(await dialog.getByRole('img', { name: '地形底图' }).getAttribute('href'), /^data:image\/png;base64,/);
    const pixels = await dialog.getByRole('img', { name: '地形底图' }).evaluate(async el => {
      const image = await window.createImageBitmap(await (await fetch(el.getAttribute('href'))).blob());
      const canvas = document.createElement('canvas'); canvas.width = 400; canvas.height = 320;
      const ctx = canvas.getContext('2d'); ctx.drawImage(image, 0, 0, 400, 320);
      return { grass: [...ctx.getImageData(50, 50, 1, 1).data], ocean: [...ctx.getImageData(350, 50, 1, 1).data] };
    });
    assert.ok(pixels.grass[1] > pixels.grass[0] && pixels.grass[1] > pixels.grass[2], JSON.stringify(pixels));
    assert.ok(pixels.ocean[2] > pixels.ocean[1] && pixels.ocean[1] > pixels.ocean[0], `高 ID 海洋正确呈蓝色 ${JSON.stringify(pixels)}`);
    await dialog.getByRole('list', { name: '地形图例' }).getByText('草地', { exact: true }).waitFor();
    assert.match(await dialog.locator('time').innerText(), /保存于/);
    await dialog.getByRole('button', { name: '放大地图', exact: true }).click();
    assert.equal(await dialog.getByRole('button', { name: '重置地图缩放' }).innerText(), '150%');
    await dialog.getByRole('checkbox', { name: '地标', exact: true }).uncheck();
    assert.equal(await dialog.locator('[data-landmark]').count(), 0);
    await dialog.getByRole('checkbox', { name: '地标', exact: true }).check();
    await dialog.getByRole('list', { name: '地图地标清单' }).getByRole('button', { name: /^定位猪王/ }).click();
    assert.equal(await dialog.getByRole('button', { name: '重置地图缩放' }).innerText(), '200%');
    await dialog.getByRole('button', { name: '重置地图缩放' }).click();
    await dialog.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/saved-map.png', import.meta.url)) });
    await dialog.getByRole('combobox', { name: '地图分片' }).click();
    await page.getByRole('option', { name: '洞穴（停用）', exact: true }).click();
    await dialog.getByRole('alert').getByText('此分片尚无世界保存点', { exact: true }).waitFor();
    assert.equal(await dialog.getByRole('img', { name: '地形底图' }).count(), 0, '切换分片不残留旧图');
    await dialog.getByRole('combobox', { name: '地图房间' }).click();
    await page.getByRole('option', { name: '另一房间', exact: true }).click();
    await dialog.getByRole('region', { name: '存档地形地图' }).waitFor();
    assert.deepEqual(state.mapCalls.at(-1), { room: '2', shard: 'Master' });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('手机地图不溢出，支持滚轮、拖动、键盘复位和搜索虫洞定位', async () => {
  const { page, state } = await openPage('saves', 375);
  try {
    await page.getByRole('button', { name: '地图预览', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '存档地图预览', exact: true });
    await dialog.getByRole('region', { name: '存档地形地图' }).waitFor();
    await dialog.getByRole('button', { name: '放大地图', exact: true }).click();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    const svg = dialog.getByRole('group', { name: '地图地形与地标', exact: true });
    const before = await svg.getAttribute('viewBox');
    const box = await svg.boundingBox();
    await page.mouse.move(box.x + box.width * 0.8, box.y + box.height * 0.3);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width * 0.8 + 35, box.y + box.height * 0.3 + 20, { steps: 5 });
    await page.mouse.up();
    assert.notEqual(await svg.getAttribute('viewBox'), before);
    await page.mouse.wheel(0, -100);
    await page.waitForFunction(() => document.querySelector('button[aria-label="重置地图缩放"]').textContent.trim() === '180%');
    await dialog.getByRole('region', { name: '存档地形地图' }).focus();
    await page.keyboard.press('Home');
    assert.equal(await dialog.getByRole('button', { name: '重置地图缩放' }).innerText(), '100%');
    await dialog.getByRole('textbox', { name: '搜索地标', exact: true }).fill('虫洞');
    const list = dialog.getByRole('list', { name: '地图地标清单' });
    assert.equal(await list.getByRole('button').count(), 2);
    await list.getByRole('button').first().click();
    await dialog.locator('line[aria-label="虫洞连接"]').waitFor();
    await dialog.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/saved-map-375.png', import.meta.url)) });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});
