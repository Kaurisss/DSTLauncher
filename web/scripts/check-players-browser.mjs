// All game/player APIs are intercepted; these tests never operate on real players.
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

const nickname = '阿尔法<img src=x onerror=alert(1)>';
const fixture = [
  { userid: 'KU_A', name: nickname, shard: 'Master', prefab: 'wilson', days: 12, inWorld: true, ghost: false },
  { userid: 'KU_B', name: '贝塔', shard: 'Master', prefab: 'willow', days: 8, inWorld: true, ghost: false, admin: true },
  { userid: 'KU_C', name: '洞穴幽灵', shard: 'Caves', prefab: 'wendy', days: 3, inWorld: true, ghost: true },
  { userid: 'KU_S', name: '等待选角', shard: 'Master', prefab: '', days: null, inWorld: false, ghost: false },
];

async function openPanel({
  width = 1280, partial = false, failure = false, delayList = false, delayAction = false, actionError = '',
  statuses = [{ shard: 'Master', state: 'running', playersReady: true }, { shard: 'Caves', state: 'running', playersReady: true }], delayStatus = false,
} = {}) {
  const page = await browser.newPage({ viewport: { width, height: 1000 } });
  page.setDefaultTimeout(10000);
  await page.addInitScript(() => {
    localStorage.setItem('dst-tab', 'overview');
    localStorage.setItem('dst-room', '2');
  });
  const requests = [];
  const state = { players: fixture.map(player => ({ ...player })), failure, statuses };
  let releaseList;
  let releaseAction;
  let releaseStatus;
  const statusWait = delayStatus ? new Promise(resolve => { releaseStatus = resolve; }) : Promise.resolve();
  const listWait = delayList ? new Promise(resolve => { releaseList = resolve; }) : Promise.resolve();
  const actionWait = delayAction ? new Promise(resolve => { releaseAction = resolve; }) : Promise.resolve();
  await page.route('**/api/**', async route => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const body = request.postData() ? request.postDataJSON() : null;
    requests.push({ path, room: url.searchParams.get('room'), method: request.method(), body });
    const ok = data => route.fulfill({ json: { code: 200, data } });
    if (path === '/api/auth/status') return ok({ enabled: false, authenticated: true });
    if (path === '/api/rooms') return ok([2, 3].map(id => ({
      id, name: id === 2 ? '玩家管理测试房间' : '另一房间', gameMode: 'survival', maxSnapshots: 10,
      shards: [{ name: 'Master', isMaster: true }, { name: 'Caves' }],
    })));
    if (path === '/api/rooms/meta') return ok({});
    if (path === '/api/status') { await statusWait; return ok(state.statuses); }
    if (path === '/api/server') return ok({ installed: true, candidates: [] });
    if (path === '/api/install') return ok({ installed: true });
    if (path === '/api/host') return ok({ memory: null, disk: null });
    if (path === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    if (path === '/api/players') {
      if (url.searchParams.get('room') === '3') return ok({ players: [{ ...fixture[1], name: '另一房间玩家', userid: 'KU_D' }], errors: {} });
      await listWait;
      if (state.failure) return route.fulfill({ json: { code: 500, message: '玩家查询暂时失败' } });
      return ok({ players: state.players.filter(player => !partial || player.shard !== 'Caves'), errors: partial ? { Caves: '世界尚未加载完成，请稍后刷新' } : {} });
    }
    if (path === '/api/players/action') {
      await actionWait;
      if (actionError) return route.fulfill({ json: { code: 500, message: actionError } });
      if (body.action === 'kick' || body.action === 'ban') state.players = state.players.filter(player => player.userid !== body.userid);
      if (body.action === 'revive') state.players.find(player => player.userid === body.userid).ghost = false;
      return ok({ executed: true });
    }
    if (path === '/api/admins') {
      state.players.find(player => player.userid === body.kleiId).adminSaved = true;
      return ok({ admins: [body.kleiId], needRestart: true });
    }
    return ok([]);
  });
  const firstRequest = !delayStatus && statuses.some(status => status.state === 'running' && status.playersReady)
    ? page.waitForRequest(request => new URL(request.url()).pathname === '/api/players') : null;
  await page.goto(origin);
  await firstRequest;
  const section = page.getByRole('region', { name: '在线玩家', exact: true });
  await section.getByRole('button', { name: /刷新玩家|查询中/ }).waitFor();
  return { page, section, requests, state, releaseList, releaseAction, releaseStatus };
}

function row(section, userid) {
  return section.getByRole('article').filter({ hasText: userid });
}
const writes = requests => requests.filter(request => request.path === '/api/players/action');

async function changeRoom(page) {
  await page.locator('#roomSelect').click();
  await page.getByRole('option', { name: '另一房间', exact: true }).click();
}

test('状态未知、未启动、正在启动及尚无玩家生成日志时均不查询，有玩家后才查询', async () => {
  const { page, section, requests, state, releaseStatus } = await openPanel({ statuses: [], delayStatus: true });
  const queries = () => requests.filter(request => request.path === '/api/players');
  try {
    await section.getByText('等待分片状态', { exact: true }).waitFor();
    assert.equal(queries().length, 0);
    assert.equal(await section.getByRole('button', { name: '刷新玩家', exact: true }).isDisabled(), true);
    releaseStatus();
    await section.getByText('房间未运行', { exact: true }).waitFor();
    assert.equal(queries().length, 0);
    state.statuses = [{ shard: 'Master', state: 'starting' }];
    await section.getByText('等待分片启动', { exact: true }).waitFor();
    assert.equal(queries().length, 0);
    state.statuses = [{ shard: 'Master', state: 'running', playersReady: false }];
    await section.getByText('等待玩家进入世界', { exact: true }).waitFor();
    assert.equal(queries().length, 0);
    assert.equal(await section.getByRole('button', { name: '刷新玩家', exact: true }).isDisabled(), true);
    state.statuses = [{ shard: 'Master', state: 'running', playersReady: true }];
    await row(section, 'KU_A').waitFor();
    assert.equal(queries().length, 1);
  } finally { releaseStatus(); await page.close(); }
});

test('停服后清空列表、忽略迟到回复并停止轮询，再启动时恢复查询', async () => {
  const { page, section, requests, state, releaseList } = await openPanel({ delayList: true });
  const queries = () => requests.filter(request => request.path === '/api/players');
  try {
    state.statuses = [];
    await section.getByText('房间未运行', { exact: true }).waitFor();
    releaseList();
    await page.waitForLoadState('networkidle');
    assert.equal(await section.getByRole('article').count(), 0);
    assert.equal(await section.getByRole('button', { name: '刷新玩家', exact: true }).isDisabled(), true);
    const stoppedCount = queries().length;
    await page.waitForTimeout(16000);
    assert.equal(queries().length, stoppedCount);
    state.statuses = [{ shard: 'Master', state: 'running', playersReady: false }];
    await section.getByText('等待玩家进入世界', { exact: true }).waitFor();
    assert.equal(queries().length, stoppedCount);
    state.statuses = [{ shard: 'Master', state: 'running', playersReady: true }];
    await row(section, 'KU_A').waitFor();
    assert.equal(queries().length, stoppedCount + 1);
  } finally { releaseList(); await page.close(); }
});

test('已记忆房间确认运行后首次加载查询玩家，角色/天数/选角正常，昵称安全且手机不溢出', async () => {
  for (const width of [1280, 375]) {
    const { page, section, requests } = await openPanel({ width });
    try {
      await row(section, 'KU_A').waitFor();
      assert.match(await row(section, 'KU_A').innerText(), /威尔逊[\s\S]*12 天/);
      assert.match(await row(section, 'KU_S').innerText(), /选角中[\s\S]*天数未知/);
      assert.match(await row(section, 'KU_C').innerText(), /洞穴[\s\S]*温蒂[\s\S]*幽灵/);
      assert.equal(await section.locator('article img').count(), 0);
      assert.equal(await row(section, 'KU_A').getByRole('button', { name: '复活', exact: true }).isDisabled(), true);
      assert.equal(await row(section, 'KU_C').getByRole('button', { name: '复活', exact: true }).isEnabled(), true);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
      assert.equal(requests.filter(request => request.path === '/api/players').length, 1);
      assert.equal(requests.find(request => request.path === '/api/players').room, '2');
    } finally { await page.close(); }
  }
});

test('封禁先确认，取消不请求；等待回执时防重复，成功后刷新列表', async () => {
  const { page, section, requests, releaseAction } = await openPanel({ delayAction: true });
  try {
    const player = row(section, 'KU_A');
    await player.getByRole('button', { name: '封禁', exact: true }).click();
    const dialog = page.getByRole('alertdialog');
    assert.match(await dialog.innerText(), /无法重新加入/);
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(writes(requests).length, 0);
    await player.getByRole('button', { name: '封禁', exact: true }).click();
    await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === '/api/players/action'),
      dialog.getByRole('button', { name: '确定', exact: true }).click(),
    ]);
    assert.equal(await player.getByRole('button', { name: '封禁', exact: true }).isDisabled(), true);
    assert.deepEqual(writes(requests), [{ path: '/api/players/action', room: '2', method: 'POST', body: { shard: 'Master', userid: 'KU_A', action: 'ban' } }]);
    releaseAction();
    await player.waitFor({ state: 'hidden' });
    assert.equal(writes(requests).length, 1);
  } finally { releaseAction(); await page.close(); }
});

test('复活请求使用真实分片，游戏拒绝时保留列表并显示错误', async () => {
  const { page, section, requests } = await openPanel({ actionError: '该玩家不是幽灵，无需复活' });
  try {
    await row(section, 'KU_C').getByRole('button', { name: '复活', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '确定', exact: true }).click();
    await section.getByRole('alert').filter({ hasText: '该玩家不是幽灵，无需复活' }).waitFor();
    assert.deepEqual(writes(requests)[0].body, { shard: 'Caves', userid: 'KU_C', action: 'revive' });
    assert.equal(await row(section, 'KU_C').count(), 1);
    assert.equal(await page.getByText('复活指令已执行', { exact: true }).count(), 0);
  } finally { await page.close(); }
});

test('传送只选同分片已入场目标，管理员复用名单接口并明确重启生效', async () => {
  const { page, section, requests } = await openPanel();
  try {
    const player = row(section, 'KU_A');
    await player.getByRole('button', { name: '传送', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: /^传送/ });
    assert.equal(await dialog.getByRole('button', { name: '确认传送' }).isDisabled(), true);
    await dialog.getByRole('combobox', { name: '传送目标' }).click();
    await page.getByRole('option', { name: '贝塔 (KU_B)', exact: true }).waitFor();
    assert.deepEqual(await page.getByRole('option').allTextContents(), ['贝塔 (KU_B)']);
    await page.getByRole('option', { name: '贝塔 (KU_B)', exact: true }).click();
    await dialog.getByRole('button', { name: '确认传送' }).click();
    await dialog.waitFor({ state: 'hidden' });
    assert.deepEqual(writes(requests)[0].body, { shard: 'Master', userid: 'KU_A', action: 'teleport', target: 'KU_B' });
    await player.getByRole('button', { name: '设为管理员', exact: true }).click();
    const confirm = page.getByRole('alertdialog');
    assert.match(await confirm.innerText(), /下次重启房间后生效/);
    await confirm.getByRole('button', { name: '确定', exact: true }).click();
    await player.getByText('管理员待重启生效', { exact: true }).waitFor();
    assert.deepEqual(requests.find(request => request.path === '/api/admins').body, { kleiId: 'KU_A' });
    assert.equal(await player.getByRole('button', { name: '设为管理员', exact: true }).isDisabled(), true);
  } finally { await page.close(); }
});

test('部分分片失败单独显示，查询失败不显示假空列表，手动刷新可恢复', async () => {
  const { page, section, state } = await openPanel({ partial: true });
  try {
    await row(section, 'KU_A').waitFor();
    await section.getByText('洞穴：世界尚未加载完成，请稍后刷新', { exact: true }).waitFor();
    await page.getByRole('tab', { name: /洞穴 Caves/ }).click();
    await page.getByRole('tabpanel', { name: /洞穴 Caves/ }).waitFor();
    state.failure = true;
    await section.getByRole('button', { name: '刷新玩家', exact: true }).click();
    await section.getByRole('alert').filter({ hasText: '玩家查询暂时失败' }).waitFor();
    assert.equal(await section.getByRole('article').count(), 0);
    assert.equal(await section.getByText('暂无在线玩家', { exact: true }).count(), 0);
    state.failure = false;
    await section.getByRole('button', { name: '刷新玩家', exact: true }).click();
    await row(section, 'KU_A').waitFor();
    assert.equal(await section.getByRole('alert').count(), 0);
  } finally { await page.close(); }
});

test('切房后迟到的旧玩家列表不会串入新房间', async () => {
  const { page, requests, releaseList } = await openPanel({ delayList: true });
  try {
    await changeRoom(page);
    const section = page.getByRole('region', { name: '在线玩家', exact: true });
    await row(section, 'KU_D').waitFor();
    releaseList();
    await page.waitForLoadState('networkidle');
    assert.equal(await row(section, 'KU_A').count(), 0);
    assert.equal(writes(requests).length, 0);
  } finally { releaseList(); await page.close(); }
});
