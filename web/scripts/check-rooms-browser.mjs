// Room navigation/import regressions: every API and ZIP is an isolated fixture.
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

function room(id, name) {
  return {
    id, name, key: `Cluster_${id}`, gameMode: 'endless', maxPlayers: 12, tickRate: 30, language: 'zh',
    maxSnapshots: 10, description: '', password: '', token: 'fixture-token', needToken: false,
    pvp: false, pauseEmpty: true, voteEnabled: true, voteKick: false, lanOnly: false, offline: false,
    masterIp: '127.0.0.1', masterPort: 16000 + id, clusterKey: `fixture-key-${id}`,
    statuses: [{ shard: 'Master', state: 'stopped' }],
    shards: [
      { name: 'Master', worldType: 'forest', isMaster: true, gameId: 1, disabled: false, serverPort: 11000 + id, masterServerPort: 27000 + id, authenticationPort: 8700 + id },
      { name: 'Caves', worldType: 'cave', isMaster: false, gameId: 2, disabled: true, serverPort: 12000 + id, masterServerPort: 28000 + id, authenticationPort: 8800 + id },
    ],
  };
}
const defaults = { ...room(0, '新的饥荒房间'), pvp: true, voteKick: true };
for (const shard of defaults.shards) { shard.serverPort = 0; shard.masterServerPort = 0; shard.authenticationPort = 0; }
const zip = { name: 'fixture-save.zip', mimeType: 'application/zip', buffer: Buffer.from('PK isolated fixture') };

async function openRooms({ width = 1280, height = 900, delaySave = false, delayUpload = false, running = false, empty = false, dark = false } = {}) {
  const page = await browser.newPage({ viewport: { width, height } });
  page.setDefaultTimeout(10000);
  await page.addInitScript(dark => {
    localStorage.setItem('dst-tab', 'rooms');
    localStorage.setItem('dst-room', '2');
    localStorage.setItem('dst-theme', dark ? 'dark' : 'light');
  }, dark);
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const state = {
    rooms: empty ? [] : [room(2, '测试房间甲'), room(3, '测试房间乙')],
    saveError: '', uploadError: '', requests: [], admins: { 2: [], 3: [] },
  };
  if (running) state.rooms[1].statuses[0].state = 'running';
  let releaseSave;
  let releaseUpload;
  const saveWait = delaySave ? new Promise(resolve => { releaseSave = resolve; }) : Promise.resolve();
  const uploadWait = delayUpload ? new Promise(resolve => { releaseUpload = resolve; }) : Promise.resolve();
  await page.route('**/api/**', async route => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();
    const body = request.headers()['content-type']?.includes('application/json') ? request.postDataJSON() : request.postData();
    state.requests.push({ path, method, query: Object.fromEntries(url.searchParams), body });
    const ok = data => route.fulfill({ json: { code: 200, data } });
    const fail = message => route.fulfill({ json: { code: 500, message } });
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms/meta') return ok({ gameModes: ['endless', 'survival'], tickRates: [15, 30, 60], languages: ['zh', 'en'], defaults });
    if (path === '/api/rooms') {
      if (method === 'GET') return ok(state.rooms);
      if (method === 'POST' || method === 'PUT') {
        await saveWait;
        if (state.saveError) return fail(state.saveError);
        const id = method === 'POST' ? 4 : body.id;
        const saved = { ...room(id, body.name), ...body, id, key: `Cluster_${id}` };
        state.rooms = [...state.rooms.filter(item => item.id !== id), saved];
        return ok(saved);
      }
      if (method === 'DELETE') {
        state.rooms = state.rooms.filter(item => String(item.id) !== url.searchParams.get('room'));
        return ok({ message: '房间已删除' });
      }
    }
    if (path === '/api/status') return ok(state.rooms.find(item => String(item.id) === url.searchParams.get('room'))?.statuses || []);
    if (path === '/api/admins') {
      const id = Number(url.searchParams.get('room'));
      if (method === 'POST') state.admins[id] = [...(state.admins[id] || []), body.kleiId];
      if (method === 'DELETE') state.admins[id] = (state.admins[id] || []).filter(item => item !== url.searchParams.get('id'));
      return ok({ admins: state.admins[id] || [], file: `/fixture/Cluster_${id}/adminlist.txt`, needRestart: true });
    }
    if (path === '/api/saves/upload') {
      await uploadWait;
      if (state.uploadError) return fail(state.uploadError);
      const mode = /name="mode"\r\n\r\n(new|overwrite)\r\n/.exec(body)?.[1];
      const id = mode === 'new' ? 4 : Number(/name="room"\r\n\r\n(\d+)\r\n/.exec(body)?.[1]);
      const saved = room(id, mode === 'new' ? '导入的新房间' : '导入后的乙房间');
      state.rooms = [...state.rooms.filter(item => item.id !== id), saved];
      return ok({ room: saved, summary: {
        roomName: saved.name, newRoom: mode === 'new', shards: ['Master', 'Caves'], hasSave: true,
        modsTotal: 2, modsMissing: ['123'], tokenSource: '保留原有令牌',
        worldFiles: ['Master/leveldataoverride.lua'], listFiles: ['adminlist.txt'],
        notes: ['端口保持不变'], warnings: Array.from({ length: 16 }, (_, index) => `测试警告 ${index + 1}：请确认已安装对应模组`),
      } });
    }
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    return ok([]);
  });
  await page.goto(origin);
  await page.getByRole('button', { name: '新建房间', exact: true }).waitFor();
  if (!empty) await page.getByRole('button', { name: '选择房间 测试房间乙', exact: true }).waitFor();
  const close = async () => {
    releaseSave?.(); releaseUpload?.();
    await page.close();
    assert.deepEqual(errors, [], '页面不应出现未处理的异常');
  };
  return { page, state, releaseSave, releaseUpload, close };
}

function card(page, name = '测试房间乙') {
  return page.getByRole('article').filter({ has: page.getByRole('heading', { name, exact: true }) });
}
const writes = (state, path, method) => state.requests.filter(request => request.path === path && (!method || request.method === method));
const uploadButton = page => page.getByRole('button', { name: '上传存档', exact: true }).first();
const uploadDialog = page => page.getByRole('dialog', { name: '上传存档', exact: true });
async function pickZip(dialog, file = zip) { await dialog.locator('input[type=file]').setInputFiles(file); }

test('分片编辑的禁用输入与选择框可查看提示，新分片选择类型仍可操作', async () => {
  const { page, close } = await openRooms();
  try {
    await card(page).getByRole('button', { name: '编辑', exact: true }).click();
    const master = page.locator('.shard-card').first();
    const input = master.getByRole('textbox', { name: '世界名', exact: true });
    assert.equal(await input.isDisabled(), true);
    await input.locator('..').hover();
    await page.getByRole('tooltip', { name: '存档目录名，保存后不可修改', exact: true }).waitFor();
    await page.keyboard.press('Escape');
    await page.getByRole('tooltip').waitFor({ state: 'hidden' });
    await page.mouse.move(0, 0);
    await master.getByRole('combobox', { name: '世界类型', exact: true }).locator('..').hover();
    await page.getByRole('tooltip', { name: '存档类型', exact: true }).waitFor();
    await page.waitForFunction(() => [...document.querySelectorAll('[data-slot="tooltip-content"]')].some(element => {
      const rect = element.getBoundingClientRect();
      return element.textContent.includes('存档类型') && rect.top >= 0 && rect.bottom <= window.innerHeight;
    }));
    await page.keyboard.press('Escape');
    await page.getByRole('button', { name: '添加世界', exact: true }).click();
    const added = page.locator('.shard-card').last();
    await added.getByRole('textbox', { name: '世界名', exact: true }).fill('Another');
    await added.getByRole('combobox', { name: '世界类型', exact: true }).click();
    await page.getByRole('option', { name: '洞穴型', exact: true }).click();
    assert.match(await added.getByRole('combobox').innerText(), /洞穴型/);
    assert.equal(await page.locator('[title]').count(), 0);
  } finally { await close(); }
});

test('新建进入独立二级页，返回不保存；保存中禁止返回，成功后回到列表并选择新房间', async () => {
  const { page, state, releaseSave, close } = await openRooms({ delaySave: true });
  try {
    assert.equal(await page.getByRole('heading', { name: '房间管理员', exact: true }).count(), 0);
    await page.getByRole('button', { name: '新建房间', exact: true }).click();
    await page.locator('#f_name').fill('不应保存的草稿');
    assert.equal(await page.locator('#new-room-button').count(), 0);
    assert.equal(await page.getByRole('dialog').count(), 0);
    await page.getByRole('button', { name: '返回房间列表', exact: true }).click();
    await page.getByRole('button', { name: '新建房间', exact: true }).waitFor();
    assert.equal(writes(state, '/api/rooms', 'POST').length, 0);
    await page.getByRole('button', { name: '新建房间', exact: true }).click();
    assert.equal(await page.locator('#f_name').inputValue(), defaults.name);
    await page.locator('#f_name').fill('  新建测试房间  ');
    await page.getByRole('switch', { name: 'PVP', exact: true }).click();
    await page.getByRole('switch', { name: '投票踢人', exact: true }).click();
    await Promise.all([
      page.waitForRequest(request => request.method() === 'POST' && new URL(request.url()).pathname === '/api/rooms'),
      page.getByRole('button', { name: '保存', exact: true }).click(),
    ]);
    assert.equal(await page.getByRole('button', { name: '返回房间列表', exact: true }).isDisabled(), true);
    assert.equal(await page.getByRole('button', { name: '取消', exact: true }).isDisabled(), true);
    const payload = writes(state, '/api/rooms', 'POST')[0].body;
    assert.equal(payload.name, '新建测试房间');
    assert.equal(payload.shards.length, 2);
    assert.equal(payload.shards[1].disabled, true);
    assert.equal(payload.pvp, false);
    assert.equal(payload.voteKick, false);
    releaseSave();
    await page.getByRole('button', { name: '新建房间', exact: true }).waitFor();
    await card(page, '新建测试房间').waitFor();
    assert.equal(await page.evaluate(() => localStorage.getItem('dst-room')), '4');
  } finally { await close(); }
});

test('编辑和管理员绑定所编辑房间，保存失败保留草稿，重试仍保留原有端口', async () => {
  const { page, state, close } = await openRooms();
  try {
    await card(page).getByRole('button', { name: '编辑', exact: true }).click();
    await page.getByRole('heading', { name: '编辑房间 · 测试房间乙', exact: true }).waitFor();
    assert.equal(await page.evaluate(() => localStorage.getItem('dst-room')), '2');
    await page.getByPlaceholder('输入 Klei ID（KU_ 开头）').fill('KU_SCOPED3');
    await page.getByRole('button', { name: '添加', exact: true }).click();
    await page.getByText('KU_SCOPED3', { exact: true }).waitFor();
    assert.equal(writes(state, '/api/admins', 'POST')[0].query.room, '3');
    await page.locator('#f_name').fill('编辑后的乙房间');
    state.saveError = '测试端口冲突';
    await page.getByRole('button', { name: '保存', exact: true }).click();
    await page.getByRole('alert').filter({ hasText: '测试端口冲突' }).waitFor();
    assert.equal(await page.locator('#f_name').inputValue(), '编辑后的乙房间');
    assert.equal(await page.locator('#new-room-button').count(), 0);
    state.saveError = '';
    await page.getByRole('button', { name: '保存', exact: true }).click();
    await card(page, '编辑后的乙房间').waitFor();
    const payload = writes(state, '/api/rooms', 'PUT').at(-1).body;
    assert.equal(payload.id, 3);
    assert.equal(payload.masterPort, 16003);
    assert.equal(payload.clusterKey, 'fixture-key-3');
    assert.equal(payload.shards[0].serverPort, 11003);
    assert.equal(payload.shards[1].disabled, true);
    assert.equal(writes(state, '/api/rooms', 'POST').length, 0);
  } finally { await close(); }
});

test('全局上传打开对话框且默认新建，文件校验和失败重试均在框内完成', async () => {
  const { page, state, close } = await openRooms();
  try {
    await uploadButton(page).click();
    const dialog = uploadDialog(page);
    await dialog.waitFor();
    assert.equal(await dialog.getAttribute('data-slot'), 'dialog-content');
    assert.equal(await dialog.locator('[data-slot="dialog-header"]').count(), 1);
    assert.equal(await dialog.locator('[data-slot="dialog-footer"]').count(), 1);
    assert.equal(await dialog.getByRole('radio', { name: '新建房间', exact: true }).isChecked(), true);
    assert.equal(await dialog.getByRole('button', { name: '选择文件', exact: true }).count(), 0);
    assert.equal(await dialog.getByRole('button', { name: '重新选择', exact: true }).count(), 0);
    const [chooser] = await Promise.all([
      page.waitForEvent('filechooser'),
      dialog.getByText('点击选择或将 .zip 存档拖到这里', { exact: true }).click(),
    ]);
    await chooser.setFiles({ ...zip, name: 'wrong.txt' });
    await dialog.getByRole('alert').filter({ hasText: '请选择 .zip 压缩包' }).waitFor();
    assert.equal(await dialog.getByRole('button', { name: '上传并新建房间', exact: true }).isDisabled(), true);
    await pickZip(dialog);
    state.uploadError = '测试 ZIP 内容无效';
    await dialog.getByRole('button', { name: '上传并新建房间', exact: true }).click();
    await dialog.getByRole('alert').filter({ hasText: '测试 ZIP 内容无效' }).waitFor();
    await dialog.getByText(zip.name, { exact: true }).waitFor();
    await dialog.getByRole('button', { name: '重新选择', exact: true }).waitFor();
    await dialog.getByText('点击选择或将 .zip 存档拖到这里', { exact: true }).waitFor({ state: 'hidden' });
    state.uploadError = '';
    await dialog.getByRole('button', { name: '上传并新建房间', exact: true }).click();
    await dialog.getByRole('region', { name: '导入结果', exact: true }).waitFor();
    assert.equal(writes(state, '/api/saves/upload').length, 2);
    for (const request of writes(state, '/api/saves/upload')) {
      assert.match(request.body, /name="mode"\r\n\r\nnew\r\n/);
      assert.doesNotMatch(request.body, /name="room"\r\n/);
    }
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    await card(page, '导入的新房间').waitFor();
    assert.equal(await page.evaluate(() => localStorage.getItem('dst-room')), '4');
  } finally { await close(); }
});

test('卡片上传预选该房间，处理期间禁止取消、Escape 和外部点击，完成后恢复焦点', async () => {
  const { page, state, releaseUpload, close } = await openRooms({ delayUpload: true });
  try {
    const trigger = card(page).getByRole('button', { name: '上传存档', exact: true });
    const opener = await trigger.elementHandle();
    await trigger.click();
    const dialog = uploadDialog(page);
    await dialog.getByRole('radio', { name: '覆盖已有房间', exact: true }).waitFor();
    assert.equal(await dialog.getByRole('radio', { name: '覆盖已有房间', exact: true }).isChecked(), true);
    assert.match(await dialog.locator('#s_target').innerText(), /测试房间乙/);
    // 一次性读取两处坐标：弹窗入场动画期间分两次取会读到不同缩放。
    const aligned = await dialog.evaluate(element => {
      const label = element.querySelector('label[for="s_target"]').getBoundingClientRect();
      const trigger = element.querySelector('#s_target').getBoundingClientRect();
      return Math.abs(label.left - trigger.left) <= 2;
    });
    assert.ok(aligned, '目标房间标签应与下拉框左对齐');
    await pickZip(dialog);
    await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === '/api/saves/upload'),
      dialog.getByRole('button', { name: '覆盖「测试房间乙」', exact: true }).click(),
    ]);
    assert.equal(await dialog.getByRole('button', { name: '取消', exact: true }).isDisabled(), true);
    assert.equal(await dialog.getByRole('button', { name: '关闭', exact: true }).isDisabled(), true);
    await page.keyboard.press('Escape');
    await page.mouse.click(4, 4);
    assert.equal(await dialog.isVisible(), true);
    const request = writes(state, '/api/saves/upload')[0];
    assert.match(request.body, /name="mode"\r\n\r\noverwrite\r\n/);
    assert.match(request.body, /name="room"\r\n\r\n3\r\n/);
    releaseUpload();
    const result = dialog.getByRole('region', { name: '导入结果', exact: true });
    await result.waitFor();
    assert.match(await result.innerText(), /导入后的乙房间/);
    assert.match(await result.innerText(), /端口保持不变/);
    assert.match(await result.innerText(), /对应模组/);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    await card(page, '导入后的乙房间').waitFor();
    assert.equal(await opener.evaluate(element => element === document.activeElement), true);
    assert.equal(writes(state, '/api/saves/upload').length, 1);
  } finally { await close(); }
});

test('运行中的目标不能覆盖，改为新建不受其它房间运行状态干扰', async () => {
  const { page, state, close } = await openRooms({ running: true });
  try {
    await card(page).getByRole('button', { name: '上传存档', exact: true }).click();
    const dialog = uploadDialog(page);
    await pickZip(dialog);
    await dialog.getByRole('alert').filter({ hasText: '房间正在运行' }).waitFor();
    assert.equal(await dialog.getByRole('button', { name: '覆盖「测试房间乙」', exact: true }).isDisabled(), true);
    assert.equal(writes(state, '/api/saves/upload').length, 0);
    await dialog.getByRole('radio', { name: '新建房间', exact: true }).click();
    assert.equal(await dialog.getByRole('button', { name: '上传并新建房间', exact: true }).isEnabled(), true);
    assert.equal(await dialog.getByRole('alert').count(), 0);
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
  } finally { await close(); }
});

test('手机二级页和上传框不横向溢出，长文件名和结果可滚动，底部操作始终可见', async () => {
  const { page, close } = await openRooms({ width: 375, height: 740 });
  try {
    await page.getByRole('button', { name: '新建房间', exact: true }).click();
    await page.locator('#f_name').waitFor();
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.equal(await page.getByPlaceholder('自动分配').first().inputValue(), '');
    await page.locator('[data-slot="page-scroll"]').evaluate(element => { element.scrollTop = element.scrollHeight; });
    const saveBounds = await page.getByRole('button', { name: '保存', exact: true }).boundingBox();
    assert.ok(saveBounds.y >= 56 && saveBounds.y + saveBounds.height <= 740, '滚到表单末尾仍能保存');
    await page.getByRole('button', { name: '返回房间列表', exact: true }).click();
    await uploadButton(page).click();
    const dialog = uploadDialog(page);
    await pickZip(dialog, { ...zip, name: `${'很长的存档文件名'.repeat(14)}.zip` });
    const bounds = await dialog.boundingBox();
    assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= 375);
    assert.ok(bounds.y >= 0 && bounds.y + bounds.height <= 740);
    assert.equal(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth), true);
    await dialog.getByRole('button', { name: '上传并新建房间', exact: true }).click();
    await dialog.getByRole('region', { name: '导入结果', exact: true }).waitFor();
    assert.equal(await dialog.evaluate(element => Array.from(element.querySelectorAll('*')).some(child =>
      window.getComputedStyle(child).overflowY === 'auto' && child.scrollHeight > child.clientHeight)), true);
    const done = dialog.getByRole('button', { name: '完成', exact: true });
    const buttonBounds = await done.boundingBox();
    assert.ok(buttonBounds.y >= 0 && buttonBounds.y + buttonBounds.height <= 740);
    assert.equal(await dialog.evaluate(element => element.scrollWidth <= element.clientWidth), true);
    await done.click();
    await dialog.waitFor({ state: 'hidden' });
  } finally { await close(); }
});

test('没有房间时可从对话框导入新房间，覆盖选项不可用', async () => {
  const { page, state, close } = await openRooms({ empty: true });
  try {
    await uploadButton(page).click();
    const dialog = uploadDialog(page);
    assert.equal(await dialog.getByRole('radio', { name: '新建房间', exact: true }).isChecked(), true);
    assert.equal(await dialog.getByRole('radio', { name: '覆盖已有房间', exact: true }).isDisabled(), true);
    await pickZip(dialog);
    await dialog.getByRole('button', { name: '上传并新建房间', exact: true }).click();
    await dialog.getByRole('region', { name: '导入结果', exact: true }).waitFor();
    assert.match(writes(state, '/api/saves/upload')[0].body, /name="mode"\r\n\r\nnew\r\n/);
  } finally { await close(); }
});

test('编辑期间房间被移除时不把编辑表单变为新建表单', async () => {
  const { page, state, close } = await openRooms();
  try {
    await card(page).getByRole('button', { name: '编辑', exact: true }).click();
    await page.locator('#f_name').waitFor();
    state.rooms = state.rooms.filter(item => item.id !== 3);
    await page.getByRole('heading', { name: '房间已不存在', exact: true }).waitFor({ timeout: 20000 });
    assert.equal(await page.locator('#f_name').count(), 0);
    assert.equal(writes(state, '/api/rooms', 'POST').length, 0);
    await page.getByRole('button', { name: '返回房间列表', exact: true }).click();
    await page.getByRole('button', { name: '新建房间', exact: true }).waitFor();
  } finally { await close(); }
});

test('房间表单吸顶合并返回与动作，垂直标签和12/4列网格对齐，分片紧凑双列', async () => {
  for (const [width, dark] of [[1280, false], [1440, true], [375, false]]) {
    const { page, close } = await openRooms({ width, dark });
    try {
      await page.emulateMedia({ reducedMotion: 'reduce' });
      await page.getByRole('button', { name: '新建房间', exact: true }).click();
      const editor = page.locator('.room-editor');
      const toolbar = page.getByRole('region', { name: '房间编辑工具栏', exact: true });
      await toolbar.waitFor();
      assert.equal(await page.getByRole('navigation', { name: '房间导航', exact: true }).count(), 0);
      assert.equal(await page.getByRole('heading', { name: '世界（分片）', exact: true }).count(), 1);
      const cancel = await toolbar.getByRole('button', { name: '取消', exact: true }).boundingBox();
      const save = await toolbar.getByRole('button', { name: '保存', exact: true }).boundingBox();
      assert.ok(cancel.x < save.x && Math.abs(cancel.y - save.y) < 2);
      const separator = await toolbar.locator('[data-slot="separator"]').boundingBox();
      const back = await toolbar.getByRole('button', { name: '返回房间列表', exact: true }).boundingBox();
      const title = await toolbar.getByRole('heading').boundingBox();
      assert.ok(Math.abs(separator.y + separator.height / 2 - (back.y + back.height / 2)) < 1, '分隔线与返回按钮居中对齐');
      assert.ok(Math.abs(separator.y + separator.height / 2 - (title.y + title.height / 2)) < 1, '分隔线与标题居中对齐');
      const token = editor.locator('#f_token');
      assert.equal(await token.evaluate(el => el.tagName), 'INPUT');
      assert.equal(await token.getAttribute('type'), 'password');
      assert.ok((await token.boundingBox()).height <= 36);
      const link = editor.getByRole('link', { name: '前往 Klei 获取令牌（新标签页）', exact: true });
      assert.equal(await link.getAttribute('href'), 'https://accounts.klei.com/account/game/servers?game=DontStarveTogether');
      assert.equal(await link.getAttribute('target'), '_blank');
      assert.match(await link.getAttribute('rel'), /noopener/);
      assert.equal(await editor.locator('#f_tickRate').isVisible(), false, 'tick不占用基本信息网格');
      const layout = await editor.evaluate(el => {
        const core = el.querySelector('.room-core-grid'), coreStyle = window.getComputedStyle(core);
        const grid = selector => window.getComputedStyle(el.querySelector(selector)).gridTemplateColumns.split(' ').length;
        return {
          coreWidth: core.getBoundingClientRect().width, gap: parseFloat(coreStyle.columnGap),
          coreFields: [...core.children].map(child => child.getBoundingClientRect().width),
          ruleColumns: grid('.room-rules-grid'), switchColumns: grid('.room-switch-grid'), shardColumns: grid('.shard-editor-grid'),
          fieldsAligned: [...el.querySelectorAll('.field-block')].filter(field => field.getBoundingClientRect().height).every(field => {
            const label = field.querySelector('.field-label'), control = field.querySelector('input,[role=combobox]');
            if (!label || !control) return true;
            const a = label.getBoundingClientRect(), b = control.getBoundingClientRect();
            return a.bottom <= b.top && Math.abs(a.left - b.left) < 2;
          }),
          ports: [...el.querySelectorAll('.shard-card')].map(card => [...card.querySelectorAll('input[type=number]')].map(input => input.getBoundingClientRect().width)),
        };
      });
      assert.equal(layout.fieldsAligned, true, '标签在控件上方，且左边缘对齐');
      if (width >= 1280) {
        const unit = (layout.coreWidth + layout.gap) / 12;
        for (const [i, span] of [4, 5, 3].entries()) assert.ok(Math.abs(layout.coreFields[i] - (span * unit - layout.gap)) < 2);
        assert.equal(layout.ruleColumns, 4);
        assert.equal(layout.switchColumns, 6);
        assert.equal(layout.shardColumns, 2);
      } else assert.equal(layout.shardColumns, 1);
      for (const ports of layout.ports) assert.ok(Math.max(...ports) - Math.min(...ports) < 2, '三个端口输入等宽');
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
      await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/room-editor-${width}-${dark ? 'dark' : 'light'}.png`, import.meta.url)) });
      await editor.getByText('高级设置', { exact: true }).click();
      await editor.locator('#f_tickRate').waitFor();
      await page.setViewportSize({ width, height: 560 });
      await page.locator('[data-slot="page-scroll"]').evaluate(el => { el.scrollTop = el.scrollHeight; });
      const position = await page.evaluate(() => ({
        toolbar: document.querySelector('.room-editor .ui-toolbar').getBoundingClientRect().top,
        card: document.querySelector('.room-editor').getBoundingClientRect().top,
        list: document.querySelector('[data-slot="page-scroll"]').getBoundingClientRect().top,
        bottom: document.querySelector('.room-editor .ui-toolbar').getBoundingClientRect().bottom,
      }));
      assert.ok(Math.abs(position.toolbar - position.card) < 0.1, 'Header紧贴固定卡片顶部：' + JSON.stringify(position));
      assert.ok(Math.abs(position.bottom - position.list) < 0.1, 'Header下方直接衔接滚动区');
      assert.ok(position.bottom < 560);
      assert.equal(await toolbar.getByRole('button', { name: '保存', exact: true }).isVisible(), true);
      await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/room-editor-sticky-${width}-${dark ? 'dark' : 'light'}.png`, import.meta.url)) });
    } finally { await close(); }
  }
});

test('编辑卡片及Header在连续滚动中保持原位，四边描边始终可见且名单可达', async () => {
  for (const [width, dark] of [[1440, false], [1440, true], [375, false]]) {
    const { page, close } = await openRooms({ width, dark });
    try {
      await page.emulateMedia({ reducedMotion: 'reduce' });
      await card(page).getByRole('button', { name: '编辑', exact: true }).click();
      await page.getByRole('heading', { name: '房间管理员', exact: true }).waitFor();
      const editor = page.locator('.room-editor');
      const toolbar = page.getByRole('region', { name: '房间编辑工具栏', exact: true });
      const scroll = page.getByRole('region', { name: '房间编辑内容', exact: true });
      const before = { card: await editor.boundingBox(), header: await toolbar.boundingBox() };
      const viewport = await page.locator('.page-view').boundingBox();
      assert.ok(before.card.y >= viewport.y + 1 && before.card.y + before.card.height <= viewport.y + viewport.height - 1, '卡片上下描边位于裁剪范围内');
      assert.ok(before.card.x >= viewport.x + 1 && before.card.x + before.card.width <= viewport.x + viewport.width - 1, '卡片左右描边位于裁剪范围内');
      assert.equal(await editor.evaluate(el => window.getComputedStyle(el).boxShadow !== 'none'), true, '外框有实际描边');
      const maximum = await scroll.evaluate(el => el.scrollHeight - el.clientHeight);
      assert.ok(maximum > 0);
      for (const top of [1, 2, 8, maximum / 2, maximum, 0]) {
        await scroll.evaluate((el, top) => new Promise(resolve => {
          el.scrollTop = top;
          requestAnimationFrame(() => resolve());
        }), top);
        assert.deepEqual(await editor.boundingBox(), before.card, '滚动时卡片四边和圆角不移动');
        assert.deepEqual(await toolbar.boundingBox(), before.header, 'Header不经历吸顶切换或一像素抖动');
      }
      await scroll.hover();
      await page.mouse.wheel(0, 300);
      await page.waitForFunction(() => document.querySelector('[data-slot="page-scroll"]').scrollTop > 0);
      assert.deepEqual(await toolbar.boundingBox(), before.header, '真实滚轮滚动时Header保持固定');
      await scroll.evaluate(el => { el.scrollTop = el.scrollHeight; });
      const last = scroll.locator('.stack > [data-slot="card"]').last();
      const bounds = await last.boundingBox(), clip = await scroll.boundingBox();
      assert.ok(bounds.y + bounds.height <= clip.y + clip.height - 1, '最后一张名单卡片及底部描边可达');
      assert.equal(await page.locator('#page-content').evaluate(el => el.scrollTop), 0);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
      await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/room-editor-fixed-${width}-${dark ? 'dark' : 'light'}.png`, import.meta.url)) });
    } finally { await close(); }
  }
});

test('单行令牌从真实剪贴板粘贴，整张开关卡片可点击，保存保留高级参数和分片语义', async () => {
  const { page, state, close } = await openRooms();
  try {
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.getByRole('button', { name: '新建房间', exact: true }).click();
    await page.locator('#f_name').fill('  紧凑表单测试  ');
    await page.evaluate(() => window.navigator.clipboard.writeText('  pds-fixture-token  '));
    await page.getByRole('button', { name: '粘贴', exact: true }).click();
    await page.waitForFunction(() => document.querySelector('#f_token').value === 'pds-fixture-token');
    for (const name of ['PVP', '无人时暂停', '投票', '投票踢人', '仅局域网', '离线模式']) {
      const toggle = page.getByRole('switch', { name, exact: true });
      const previous = await toggle.getAttribute('aria-checked');
      const card = toggle.locator('..');
      await card.click({ position: { x: 5, y: 5 } });
      assert.equal(await toggle.getAttribute('aria-checked'), previous === 'true' ? 'false' : 'true');
    }
    await page.locator('#f_maxSnapshots').fill('20');
    await page.getByText('高级设置', { exact: true }).click();
    await page.getByRole('combobox', { name: 'tick 频率', exact: true }).click();
    await page.getByRole('option', { name: /^60/ }).click();
    await page.getByRole('button', { name: '保存', exact: true }).click();
    await page.getByRole('button', { name: '新建房间', exact: true }).waitFor();
    const payload = writes(state, '/api/rooms', 'POST').at(-1).body;
    assert.equal(payload.name, '紧凑表单测试');
    assert.equal(payload.token, 'pds-fixture-token');
    assert.equal(payload.tickRate, 60);
    assert.equal(payload.maxSnapshots, 20);
    assert.deepEqual([payload.pvp, payload.pauseEmpty, payload.voteEnabled, payload.voteKick, payload.lanOnly, payload.offline], [false, false, false, false, true, true]);
    assert.deepEqual(payload.shards.map(shard => [shard.name, shard.gameId, shard.disabled]), [['Master', 1, false], ['Caves', 2, true]]);
  } finally { await close(); }
});

test('分片移除操作保留主世界约束，取消保留草稿，确认后只移除目标分片', async () => {
  const { page, close } = await openRooms();
  try {
    await card(page).getByRole('button', { name: '编辑', exact: true }).click();
    const shards = page.locator('.shard-card');
    assert.equal(await shards.first().getByRole('switch', { name: '启用', exact: true }).isDisabled(), true);
    assert.equal(await shards.first().getByRole('button', { name: '从面板移除（存档目录保留）', exact: true }).count(), 0);
    const remove = shards.nth(1).getByRole('button', { name: '从面板移除（存档目录保留）', exact: true });
    await remove.click();
    const confirmation = page.getByRole('alertdialog');
    await confirmation.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(await shards.count(), 2);
    await remove.click();
    await confirmation.getByRole('button', { name: '确定', exact: true }).click();
    assert.equal(await shards.count(), 1);
    assert.equal(await shards.first().getByRole('textbox', { name: '世界名', exact: true }).inputValue(), 'Master');
  } finally { await close(); }
});
