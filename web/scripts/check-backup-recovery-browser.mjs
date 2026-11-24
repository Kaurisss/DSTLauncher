import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let vite, browser, origin;
before(async () => {
  vite = await createServer({ root: fileURLToPath(new URL('..', import.meta.url)), server: { host: '127.0.0.1', port: 0 } });
  await vite.listen(); origin = vite.resolvedUrls.local[0];
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => { await browser?.close(); await vite?.close(); });

const archive = 'Cluster_1_20261107_120000.000000001.zip';
async function openPage(tab, { width = 1280, badCheck = false, running = false } = {}) {
  const page = await browser.newPage({ viewport: { width, height: 1000 } });
  page.setDefaultTimeout(10000);
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.addInitScript(tab => { localStorage.setItem('dst-tab', tab); localStorage.setItem('dst-room', '1'); }, tab);
  const state = { writes: [], errors: [], badCheck, testFailure: '', config: { enabled: true, kind: 'directory', directory: 'D:\\Backups', url: '', username: '', passwordSet: false, keep: 3, results: [{ id: 'old', room: 1, name: archive, state: 'failed', time: '2026-11-07T12:00:00Z', error: 'WebDAV PUT 返回 HTTP 503' }] }, check: null };
  page.on('pageerror', error => state.errors.push(error.message));
  const tiles = Buffer.alloc(20 * 20 * 2);
  for (let i = 0; i < 400; i++) tiles.writeUInt16LE(i % 20 < 12 ? 7 : 201, 2 * i);
  await page.route('**/api/**', async route => {
    const request = route.request(), url = new URL(request.url()), path = url.pathname, method = request.method();
    const ok = data => route.fulfill({ json: { code: 200, data } });
    const body = method === 'POST' || method === 'PUT' ? request.postDataJSON() : null;
    if (method !== 'GET') state.writes.push({ path, method, body });
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok([{ id: 1, name: '恢复测试房间', key: 'Cluster_1', running, shards: [{ name: 'Master', isMaster: true }, { name: 'Caves', disabled: true }], statuses: [] }]);
    if (path === '/api/rooms/meta') return ok({});
    if (path === '/api/backups') return ok([{ name: archive, size: 2048, createdAt: '2026-11-07T12:00:00Z' }]);
    if (path === '/api/backup-remote') {
      if (method === 'PUT') { state.config = { ...state.config, ...body, passwordSet: body.password === '' ? false : !!body.password || state.config.passwordSet }; delete state.config.password; }
      return ok(state.config);
    }
    if (path === '/api/backup-remote/test') return state.testFailure ? route.fulfill({ json: { code: 500, message: state.testFailure } }) : ok(null);
    if (path === '/api/backup-remote/sync') { state.config.results = [{ ...state.config.results[0], id: 'retry', state: 'synced', error: '' }]; return ok({ id: 'retry' }); }
    if (path === '/api/backups/check') {
      if (method === 'POST') {
        state.check = state.badCheck ? { id: 'check', state: 'failed', error: '备份 ZIP CRC 校验失败' } : { id: 'check', state: 'ready', room: 1, name: archive, report: { name: archive, sha256: 'a'.repeat(64), canRestore: true, createdAt: '2026-11-07T12:00:00Z', files: 12, bytes: 2048, warnings: ['Caves 没有世界保存点，可能尚未生成或未包含在备份中'], shards: [{ name: 'Master', snapshots: 3, playerDirectories: 2, playerFiles: 4, worldConfig: true, modsConfig: true, state: { day: 22, season: 'autumn', savedAt: '2026-11-07T11:59:00Z' } }, { name: 'Caves', snapshots: 0, playerDirectories: 0, playerFiles: 0, worldConfig: true, modsConfig: true }] } };
        return ok({ id: 'check' });
      }
      if (method === 'DELETE') return ok(null);
      return ok(state.check);
    }
    if (path === '/api/backups/check/map') return ok({ width: 20, height: 20, kind: 'terrain', terrain: { tiles: tiles.toString('base64'), specs: [{ id: 7, name: 'GRASS' }, { id: 201, name: 'OCEAN_COASTAL' }] }, landmarks: [{ id: 'pigking:0', name: '猪王', kind: 'pigking', x: 8, y: 10 }] });
    if (path === '/api/backups/restore') { state.check = { ...state.check, state: 'restored', previousDirectory: 'isolated/Cluster_1.before-restore' }; return ok({ id: 'check' }); }
    if (path === '/api/automation') return ok({ config: { backupHours: 0, keep: 10, restartAt: '', updateMinutes: 0, updateMode: 'notify' }, jobs: {} });
    if (path === '/api/host') return ok({ memory: { totalBytes: 16e9, availableBytes: 12e9 }, disk: { totalBytes: 100e9, availableBytes: 80e9 } });
    if (path === '/api/server' || path === '/api/install') return ok({ installed: true, candidates: [] });
    if (path === '/api/workshop/key') return ok({ steamApiKey: '' });
    return ok([]);
  });
  await page.goto(origin);
  return { page, state };
}

test('异地备份用自定义 Select 切换 WebDAV，保存凭据不回填密码，连接失败可见', async () => {
  const { page, state } = await openPage('settings');
  try {
    await page.getByRole('navigation', { name: '设置导航' }).getByRole('button', { name: '异地备份', exact: true }).click();
    const target = page.getByRole('combobox', { name: '备份目标', exact: true });
    assert.equal(await target.evaluate(el => el.tagName), 'BUTTON');
    await target.click(); await page.getByRole('option', { name: 'WebDAV', exact: true }).click();
    await page.getByRole('textbox', { name: 'WebDAV 目录地址', exact: true }).fill('https://example.com/dav/backups/');
    await page.getByRole('textbox', { name: '用户名', exact: true }).fill('backup-user');
    await page.getByLabel('密码或应用密码', { exact: true }).fill('test-only-secret');
    await page.getByRole('spinbutton', { name: '每个房间保留份数' }).fill('6');
    await page.getByRole('button', { name: '保存异地备份设置', exact: true }).click();
    await page.waitForFunction(() => document.querySelector('#remote-password').placeholder.includes('已保存'));
    const saved = state.writes.find(item => item.path === '/api/backup-remote');
    assert.equal(saved.body.password, 'test-only-secret'); assert.equal(saved.body.keep, 6); assert.equal(saved.body.kind, 'webdav');
    assert.equal(await page.getByLabel('密码或应用密码', { exact: true }).inputValue(), '');
    state.testFailure = 'WebDAV MKCOL 返回 HTTP 403';
    await page.getByRole('button', { name: '测试已保存的备份目标', exact: true }).click();
    await page.getByText('WebDAV MKCOL 返回 HTTP 403', { exact: true }).waitFor();
    await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/backup-remote-settings.png', import.meta.url)) });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('恢复前展示分片、玩家保存目录、天数和地图，取消不恢复，确认提交检查摘要', async () => {
  const { page, state } = await openPage('backups');
  try {
    await page.getByRole('button', { name: '回滚', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '备份恢复检查', exact: true });
    await dialog.getByText('第 22 天，秋季', { exact: true }).waitFor();
    await dialog.getByText('3 个世界保存点，2 个玩家保存目录', { exact: true }).waitFor();
    await dialog.getByRole('img', { name: '地形底图', exact: true }).waitFor();
    assert.equal(await dialog.getByRole('combobox', { name: '备份地图分片' }).evaluate(el => el.tagName), 'BUTTON');
    await dialog.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/backup-inspection.png', import.meta.url)) });
    await dialog.getByRole('button', { name: '确认恢复此备份', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(state.writes.some(item => item.path === '/api/backups/restore'), false);
    await dialog.getByRole('button', { name: '确认恢复此备份', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await dialog.getByText(/恢复完成。恢复前的存档保留于/).waitFor();
    assert.deepEqual(state.writes.find(item => item.path === '/api/backups/restore').body, { id: 'check', sha256: 'a'.repeat(64) });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('坏包不能进入恢复，重新检查后运行中房间仍禁止恢复', async () => {
  const { page, state } = await openPage('backups', { badCheck: true, running: true });
  try {
    await page.getByRole('button', { name: '回滚', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '备份恢复检查', exact: true });
    await dialog.getByRole('alert').getByText('备份 ZIP CRC 校验失败', { exact: true }).waitFor();
    assert.equal(await dialog.getByRole('button', { name: '确认恢复此备份', exact: true }).count(), 0);
    state.badCheck = false;
    await dialog.getByRole('button', { name: '重新检查', exact: true }).click();
    const restore = dialog.getByRole('button', { name: '确认恢复此备份', exact: true }); await restore.waitFor();
    assert.equal(await restore.isDisabled(), true);
    assert.equal(state.writes.some(item => item.path === '/api/backups/restore'), false);
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});

test('窄屏备份页可重试同步，检查对话框没有横向溢出', async () => {
  const { page, state } = await openPage('backups', { width: 375 });
  try {
    await page.getByRole('button', { name: '重试同步', exact: true }).click();
    await page.getByText('已同步', { exact: true }).waitFor();
    assert.deepEqual(state.writes.find(item => item.path === '/api/backup-remote/sync').body, { name: archive });
    await page.getByRole('button', { name: '回滚', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '备份恢复检查', exact: true });
    await dialog.getByRole('img', { name: '地形底图', exact: true }).waitFor();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    await dialog.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/backup-inspection-375.png', import.meta.url)) });
    assert.deepEqual(state.errors, []);
  } finally { await page.close(); }
});
