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
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
});
after(async () => {
  await browser?.close();
  await vite?.close();
});

const mods = [
  { id: 'workshop-1185229307', name: 'workshop-1185229307', enabled: true, installed: false },
  { id: 'workshop-1207269058', name: '本地已下载名称', version: '1.2.3-beta', enabled: false, installed: true },
  { id: 'workshop-1271089343', name: 'workshop-1271089343', enabled: false, installed: false },
  { id: 'local-test', name: '本地测试模组', enabled: false, installed: true },
];
const details = [
  { id: '1185229307', title: '工坊模组 <测试>', previewUrl: 'https://fixtures.invalid/cover.svg' },
  { id: '1207269058', title: '已下载的工坊标题', previewUrl: 'https://fixtures.invalid/broken.png' },
];

async function openMods({
  failDetails = false, running = false, delayDetails = false, delayMods = false, width = 1280,
  schema = [], config = {}, ugcDir = '', serverLocked = false,
  modItems = mods, brokenIcons = [],
  shardItems = [{ name: 'Master', worldType: 'forest', isMaster: true }],
  shardMods = null, statusState = null,
  delayToggle = false,
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
  let releaseToggle;
  const waitForDetails = delayDetails ? new Promise(resolve => { releaseDetails = resolve; }) : Promise.resolve();
  const waitForMods = delayMods ? new Promise(resolve => { releaseMods = resolve; }) : Promise.resolve();
  const waitForToggle = delayToggle ? new Promise(resolve => { releaseToggle = resolve; }) : Promise.resolve();
  const requests = [];
  const state = {
    mods: globalThis.structuredClone(modItems), toggleError: '', configError: '', searchError: '',
    workshopResults: details, searchPages: {}, startError: '', statusError: '',
    listDelay: Promise.resolve(),
    shardMods: globalThis.structuredClone(shardMods), shardDelays: {}, updateDelays: {},
    updates: { checked: 1, updates: [], missing: [], notInstalled: [] },
  };
  state.mods[1].config = config;
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
    requests.push({ path: url.pathname, params: Object.fromEntries(url.searchParams), method: route.request().method(),
      body: route.request().headers()['content-type']?.includes('application/json') ? route.request().postDataJSON() : null });
    const ok = data => route.fulfill({
      contentType: 'application/json', body: JSON.stringify({ code: 200, data }),
    });
    const fail = message => route.fulfill({ json: { code: 500, message } });
    if (url.pathname === '/api/mods/icon') {
      if (brokenIcons.includes(url.searchParams.get('id'))) return route.fulfill({ status: 404, body: '' });
      return route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="40"><rect width="80" height="40" fill="#397461"/></svg>' });
    }
    if (url.pathname === '/api/rooms') return ok([{
      id: 2, name: '测试房间', key: 'Cluster_2', gameMode: 'survival', maxPlayers: 4,
      shards: shardItems,
    }]);
    if (url.pathname === '/api/status') return ok([{ shard: 'Master', state: statusState || (running ? 'running' : 'stopped') }]);
    if (url.pathname === '/api/install') return ok({ installed: true });
    if (url.pathname === '/api/server') return ok({
      installed: true, candidates: [], ugcDir, locked: serverLocked, archOptions: ['auto', '64', '32'],
    });
    if (url.pathname === '/api/mods') {
      const shard = url.searchParams.get('shard') || 'Master';
      const list = globalThis.structuredClone(state.shardMods?.[shard] || state.mods);
      const delay = state.shardDelays[shard] || state.listDelay;
      await waitForMods;
      await delay;
      return ok(list);
    }
    if (url.pathname === '/api/mods/delete') {
      state.mods = state.mods.filter(mod => mod.id !== url.searchParams.get('id'));
      if (state.shardMods) for (const shard in state.shardMods) state.shardMods[shard] = state.shardMods[shard].filter(mod => mod.id !== url.searchParams.get('id'));
      return ok({});
    }
    if (url.pathname === '/api/mods/toggle') {
      await waitForToggle;
      if (state.toggleError) return fail(state.toggleError);
      const list = state.shardMods?.[url.searchParams.get('shard')] || state.mods;
      const mod = list.find(mod => mod.id === url.searchParams.get('id'));
      if (mod) mod.enabled = url.searchParams.get('enabled') === 'true';
      return ok({});
    }
    if (url.pathname === '/api/mods/config') {
      if (state.configError) return fail(state.configError);
      const list = state.shardMods?.[url.searchParams.get('shard')] || state.mods;
      const mod = list.find(mod => mod.id === url.searchParams.get('id'));
      if (!mod.config) mod.enabled = true;
      (mod.config ||= {})[url.searchParams.get('key')] = JSON.parse(url.searchParams.get('value'));
      return ok({});
    }
    if (url.pathname === '/api/mods/updates') {
      const result = globalThis.structuredClone(state.updates);
      await state.updateDelays[url.searchParams.get('shard')];
      return ok(result);
    }
    if (url.pathname === '/api/mods/schema') return ok(schema);
    if (url.pathname === '/api/workshop/details') {
      await waitForDetails;
      if (failDetails) return route.fulfill({
        contentType: 'application/json', body: JSON.stringify({ code: 500, message: 'Steam 暂时不可达' }),
      });
      return ok(url.searchParams.get('input').includes(',') ? details : state.workshopResults);
    }
    if (url.pathname === '/api/workshop/search') {
      if (state.searchError) return fail(state.searchError);
      return ok(state.searchPages[url.searchParams.get('page')] || state.workshopResults);
    }
    if (url.pathname === '/api/workshop/key') { state.searchError = ''; return ok({}); }
    if (url.pathname === '/api/mods/update') {
      if (route.request().method() === 'POST') return state.startError ? fail(state.startError) : ok({ running: true, output: ['开始'] });
      if (route.request().method() === 'DELETE') return ok({});
      if (state.statusError) return fail(state.statusError);
      return ok(updateStatuses[Math.min(updateStatusIndex++, updateStatuses.length - 1)]);
    }
    if (url.pathname === '/api/logs/stream') return route.fulfill({ contentType: 'text/event-stream', body: '' });
    return ok([]);
  });
  await page.goto(origin);
  await page.locator(delayMods ? '.mod-skeleton' : '.mod-entry-head').first().waitFor();
  return { page, requests, releaseDetails, releaseMods, releaseToggle, state };
}

async function waitForTitle(page) {
  await page.getByRole('link', { name: '工坊模组 <测试>' }).waitFor();
}

async function chooseSelect(page, label, option) {
  await page.getByRole('combobox', { name: label, exact: true }).click();
  await page.getByRole('option', { name: option }).click();
}

const shardItems = [
  { name: 'Surface', worldType: 'forest', isMaster: true },
  { name: 'Below', worldType: 'cave' },
  { name: 'Dormant', worldType: 'forest', disabled: true },
];
const shardMods = {
  Surface: [{ id: 'local-shared', name: '独立参数测试', installed: true, enabled: true, config: { amount: 1 } }],
  Below: [{ id: 'local-shared', name: '独立参数测试', installed: true, enabled: false, config: { amount: 9 } }],
  Dormant: [{ id: 'local-shared', name: '独立参数测试', installed: true, enabled: false, config: { amount: 7 } }],
};

test('运行中禁用的模组操作仍可悬停和聚焦查看提示，回车不触发删除', async () => {
  const { page, requests } = await openMods({ running: true });
  try {
    const item = page.getByRole('article', { name: '本地测试模组', exact: true });
    const remove = item.getByRole('button', { name: '删除', exact: true });
    assert.equal(await remove.isDisabled(), true);
    const trigger = remove.locator('..');
    await trigger.hover();
    await page.getByRole('tooltip', { name: '删除模组', exact: true }).waitFor();
    await page.keyboard.press('Escape');
    await page.getByRole('tooltip').waitFor({ state: 'hidden' });
    await page.mouse.move(0, 0);
    await page.keyboard.press('Tab');
    await trigger.focus();
    await page.getByRole('tooltip', { name: '删除模组', exact: true }).waitFor();
    await page.keyboard.press('Enter');
    assert.equal(requests.filter(request => request.path === '/api/mods/delete').length, 0);
    assert.equal(await page.getByRole('alertdialog').count(), 0);
    assert.equal(await page.locator('[title]').count(), 0);
  } finally { await page.close(); }
});

test('分片独立启用和保存配置，抽屉锁定分片，停用世界仍可编辑', async () => {
  const { page, requests, state } = await openMods({ shardItems, shardMods,
    schema: [{ name: 'amount', label: '分片数量', type: 'number', default: 1 }],
  });
  try {
    const select = page.locator('[data-slot="select-trigger"][aria-label="模组分片"]');
    assert.match(await select.innerText(), /森林 · Surface/);
    await select.click();
    await page.getByRole('option', { name: /洞穴 · Below/ }).waitFor();
    await page.keyboard.press('Escape');
    await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === '/api/mods' && new URL(response.url()).searchParams.get('shard') === 'Below'),
      chooseSelect(page, '模组分片', /Below/),
    ]);
    const entry = page.getByRole('article', { name: '独立参数测试' });
    await entry.getByRole('switch').click();
    await page.waitForFunction(() => document.querySelector('.mod [role="switch"]')?.getAttribute('aria-checked') === 'true');
    await entry.getByRole('button', { name: '配置', exact: true }).click();
    assert.equal(await select.isDisabled(), true);
    const input = page.getByLabel('分片数量', { exact: true });
    assert.equal(await input.inputValue(), '9');
    await input.fill('4');
    await page.getByRole('button', { name: '保存配置', exact: true }).click();
    await page.getByText('模组配置已保存，重启后生效', { exact: true }).waitFor();
    await page.getByRole('button', { name: '关闭配置', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await chooseSelect(page, '模组分片', /Surface/);
    await entry.getByRole('button', { name: '配置', exact: true }).click();
    assert.equal(await input.inputValue(), '1');
    await input.fill('2');
    await page.getByRole('button', { name: '关闭配置', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '继续编辑', exact: true }).click();
    assert.equal(await select.isDisabled(), true);
    assert.equal(await input.inputValue(), '2');
    await page.getByRole('button', { name: '关闭配置', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '放弃修改', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await chooseSelect(page, '模组分片', /Dormant/);
    await entry.getByRole('switch').click();
    await page.waitForFunction(() => document.querySelector('.mod [role="switch"]')?.getAttribute('aria-checked') === 'true');
    assert.equal(state.shardMods.Surface[0].config.amount, 1);
    assert.equal(state.shardMods.Below[0].config.amount, 4);
    assert.equal(state.shardMods.Dormant[0].enabled, true);
    assert.deepEqual(requests.filter(request => request.path === '/api/mods/config').map(request => request.params.shard), ['Below']);
    await page.reload();
    await entry.waitFor();
    await chooseSelect(page, '模组分片', /Below/);
    await entry.getByRole('button', { name: '配置', exact: true }).click();
    assert.equal(await input.inputValue(), '4');
  } finally { await page.close(); }
});

test('分片慢列表和更新响应不会覆盖新选择', async () => {
  const { page, state } = await openMods({ shardItems, shardMods });
  let releaseList;
  let releaseUpdate;
  try {
    state.shardDelays.Below = new Promise(resolve => { releaseList = resolve; });
    await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === '/api/mods' && new URL(request.url()).searchParams.get('shard') === 'Below'),
      chooseSelect(page, '模组分片', /Below/),
    ]);
    await chooseSelect(page, '模组分片', /Surface/);
    await page.getByRole('article', { name: '独立参数测试' }).waitFor();
    const oldResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/mods' && new URL(response.url()).searchParams.get('shard') === 'Below');
    releaseList();
    await oldResponse;
    assert.equal(await page.locator('.mod').getByRole('switch').getAttribute('aria-checked'), 'true');
    state.updateDelays.Surface = new Promise(resolve => { releaseUpdate = resolve; });
    state.updates = { checked: 1, updates: [{ id: 'local-shared', localTime: 1, remoteTime: 2 }], missing: [], notInstalled: [] };
    await Promise.all([
      page.waitForRequest(request => new URL(request.url()).pathname === '/api/mods/updates'),
      page.getByRole('button', { name: '检查更新', exact: true }).click(),
    ]);
    await chooseSelect(page, '模组分片', /Below/);
    await page.getByRole('article', { name: '独立参数测试' }).waitFor();
    const updateResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/mods/updates');
    releaseUpdate();
    await updateResponse;
    assert.equal(await page.getByText('有新版本', { exact: true }).count(), 0);
    assert.equal(await page.locator('.mod').getByRole('switch').getAttribute('aria-checked'), 'false');
  } finally { releaseList?.(); releaseUpdate?.(); await page.close(); }
});

test('停止中的房间冻结各分片启用和配置', async () => {
  const { page } = await openMods({ shardItems, shardMods, statusState: 'stopping', width: 375,
    schema: [{ name: 'amount', label: '分片数量', type: 'number', default: 1 }],
  });
  try {
    await page.getByText('服务器运行中，模组修改已冻结。', { exact: true }).waitFor();
    await chooseSelect(page, '模组分片', /Below/);
    const entry = page.getByRole('article', { name: '独立参数测试' });
    assert.equal(await entry.getByRole('switch').isDisabled(), true);
    await entry.getByRole('button', { name: '配置', exact: true }).click();
    assert.equal(await page.getByLabel('分片数量', { exact: true }).isDisabled(), true);
    assert.equal(await page.getByRole('button', { name: '保存配置', exact: true }).isDisabled(), true);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
  } finally { await page.close(); }
});

test('工坊添加后的启用操作只写所选分片', async () => {
  const { page, state, requests } = await openMods({ shardItems,
    shardMods: { Surface: globalThis.structuredClone(mods), Below: globalThis.structuredClone(mods) },
  });
  try {
    await chooseSelect(page, '模组分片', /Below/);
    await page.locator('.mod').nth(1).waitFor();
    await page.getByRole('button', { name: '添加模组', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '添加模组', exact: true });
    await dialog.getByRole('textbox', { name: '搜索创意工坊' }).fill('1207269058');
    await dialog.getByRole('button', { name: '查询', exact: true }).click();
    const item = dialog.getByRole('article').filter({ hasText: '已下载的工坊标题' });
    await Promise.all([
      page.waitForResponse(response => new URL(response.url()).pathname === '/api/mods/toggle'),
      item.getByRole('button', { name: '启用', exact: true }).click(),
    ]);
    assert.equal(state.shardMods.Surface[1].enabled, false);
    assert.equal(state.shardMods.Below[1].enabled, true);
    assert.equal(requests.find(request => request.path === '/api/mods/toggle').params.shard, 'Below');
  } finally { await page.close(); }
});

test('保存配置后条目离开禁用筛选时释放分片选择锁', async () => {
  const { page } = await openMods({ shardItems,
    shardMods: { Surface: [{ id: 'local-new', name: '未配置模组', enabled: false, installed: true }] },
    schema: [{ name: 'amount', label: '分片数量', type: 'number', default: 1 }],
  });
  try {
    await chooseSelect(page, '模组状态', /已禁用/);
    await page.getByRole('button', { name: '配置', exact: true }).click();
    await page.getByRole('button', { name: '保存配置', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    assert.equal(await page.getByRole('combobox', { name: '模组分片', exact: true }).isDisabled(), false);
  } finally { await page.close(); }
});

test('未知默认值保持未设置，保留已有配置和显式空值，手动编辑后才提交', async () => {
  const { page, requests } = await openMods({
    schema: [
      { name: 'font', label: '字体', default: '', defaultUnresolved: true },
      { name: 'existing', label: '已有配置', default: '', defaultUnresolved: true },
      { name: 'empty', label: '空默认值', default: '' },
    ],
    config: { existing: false },
  });
  try {
    await page.locator('.mod').nth(1).getByRole('button', { name: '配置', exact: true }).click();
    const font = page.getByLabel('字体', { exact: true });
    await font.waitFor();
    assert.equal(await font.inputValue(), '');
    assert.equal(await page.getByLabel('已有配置', { exact: true }).inputValue(), 'false');
    const save = page.getByRole('button', { name: '保存配置', exact: true });
    await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/api/mods/config' && new URL(r.url()).searchParams.get('key') === 'empty'),
      save.click(),
    ]);
    const writes = requests.filter(r => r.path === '/api/mods/config');
    assert.deepEqual(writes.map(r => [r.params.key, JSON.parse(r.params.value)]), [['empty', '']]);
    await font.fill('custom-font');
    await Promise.all([
      page.waitForResponse(r => new URL(r.url()).pathname === '/api/mods/config' && new URL(r.url()).searchParams.get('key') === 'font'),
      save.click(),
    ]);
    assert.equal(JSON.parse(requests.find(r => r.path === '/api/mods/config' && r.params.key === 'font').params.value), 'custom-font');
  } finally {
    await page.close();
  }
});

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
    assert.match(await page.locator('.mod').nth(1).innerText(), /版本 1\.2\.3-beta/);
    assert.match(await page.locator('.mod').nth(3).innerText(), /版本未知/);
    assert.doesNotMatch(await page.locator('.mod').first().innerText(), /版本未知/);
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

test('设置展示实际 Steam 工坊路径，下载期间锁定路径切换且手机不溢出', async () => {
  const ugcDir = 'D:\\很长的 Steam 游戏安装目录\\steamapps\\workshop';
  const { page, requests } = await openMods({ ugcDir, serverLocked: true });
  try {
    await page.locator('#app-navigation').getByRole('button', { name: '设置', exact: true }).click();
    await page.getByText(ugcDir, { exact: true }).waitFor();
    await page.getByText('更改服务器文件位置', { exact: true }).click();
    assert.equal(await page.locator('#srv_dir').isDisabled(), true);
    await page.setViewportSize({ width: 375, height: 900 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    assert.equal(requests.some(r => r.path === '/api/mods/update' && r.method === 'POST'), false);
  } finally { await page.close(); }
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
    assert.equal(await broken.getByRole('button', { name: '重新下载', exact: true }).isDisabled(), true);
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
        // 入场平移会产生浮点舍入误差，保留千分之一像素后再比较。
        return { height: Number(rects[0].height.toFixed(3)), gap: Number((rects[1].top - rects[0].bottom).toFixed(3)) };
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

test('下载留在后台，日志按需打开，关闭后继续轮询，可重开并中止任务', async () => {
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
    await page.getByRole('button', { name: '查看进度', exact: true }).waitFor();
    assert.equal(await dialog.count(), 0, '下载不抢占当前操作');
    assert.equal(await page.locator('.mod').nth(1).getByRole('button', { name: '删除', exact: true }).isDisabled(), true, '共享文件更新期间禁用删除');
    await page.getByRole('button', { name: '查看进度', exact: true }).click();
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

test('本地搜索和状态筛选即时响应开关，刷新后仍保留筛选和已保存的状态', async () => {
  const { page, requests } = await openMods();
  try {
    await waitForTitle(page);
    await chooseSelect(page, '模组状态', /已启用/);
    assert.equal(await page.locator('.mod').count(), 1);
    await page.locator('.mod').getByRole('switch').click();
    await page.getByText('没有匹配的模组', { exact: true }).waitFor();
    await chooseSelect(page, '模组状态', /已禁用/);
    assert.equal(await page.locator('.mod').count(), 4);
    const search = page.getByRole('textbox', { name: '搜索模组', exact: true });
    await search.fill('本地已下载名称');
    await page.locator('.mod').getByRole('link', { name: '已下载的工坊标题', exact: true }).waitFor();
    assert.equal(await page.locator('.mod').count(), 1);
    await search.fill('local-test');
    assert.equal(await page.locator('.mod').count(), 1);
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await page.locator('.mod').getByText('本地测试模组', { exact: true }).waitFor();
    assert.equal(await search.inputValue(), 'local-test');
    await search.fill('不存在的模组');
    await page.getByRole('button', { name: '清除筛选', exact: true }).click();
    assert.equal(await page.locator('.mod').count(), 4);
    assert.equal(requests.filter(r => r.path === '/api/mods/toggle').length, 1);
  } finally { await page.close(); }
});

test('开关提交中禁止重复点击，失败恢复原状态，重试后正确保存', async () => {
  const { page, requests, state, releaseToggle } = await openMods({ delayToggle: true });
  try {
    await waitForTitle(page);
    state.toggleError = '无法保存启用状态';
    const toggle = page.locator('.mod').first().getByRole('switch');
    await toggle.click();
    assert.equal(await toggle.isDisabled(), true);
    await toggle.evaluate(el => el.click());
    releaseToggle();
    await page.getByText('无法保存启用状态', { exact: true }).waitFor();
    assert.equal(await toggle.getAttribute('aria-checked'), 'true');
    assert.equal(requests.filter(r => r.path === '/api/mods/toggle').length, 1);
    state.toggleError = '';
    await toggle.click();
    await page.waitForFunction(() => document.querySelector('.mod [role=switch]').getAttribute('aria-checked') === 'false');
    assert.equal(state.mods[0].enabled, false);
  } finally { releaseToggle(); await page.close(); }
});

test('刷新慢响应晚于开关保存返回时，不会覆盖刚保存的启用状态', async () => {
  const { page, state } = await openMods();
  let release;
  try {
    await waitForTitle(page);
    state.listDelay = new Promise(resolve => { release = resolve; });
    const read = page.waitForRequest(request => new URL(request.url()).pathname === '/api/mods');
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await read;
    state.listDelay = Promise.resolve();
    const fresh = page.waitForResponse(response => new URL(response.url()).pathname === '/api/mods');
    const toggle = page.locator('.mod').first().getByRole('switch');
    await toggle.click();
    await fresh;
    const old = page.waitForResponse(response => new URL(response.url()).pathname === '/api/mods');
    release();
    await old;
    assert.equal(await toggle.getAttribute('aria-checked'), 'false');
    assert.equal(state.mods[0].enabled, false);
  } finally { release?.(); await page.close(); }
});

test('配置侧栏保护未保存修改，撤销还原，保存失败保留草稿，成功后重新打开仍为新值', async () => {
  const { page, state, requests } = await openMods({
    schema: [{ name: 'text', label: '测试配置', default: '初始值' }, { name: 'count', label: '数量', type: 'number', default: 1, min: 1, max: 5 }],
  });
  try {
    await waitForTitle(page);
    const configure = page.locator('.mod').nth(1).getByRole('button', { name: '配置', exact: true });
    await configure.click();
    const field = page.getByRole('textbox', { name: '测试配置', exact: true });
    await field.fill('保留草稿');
    await page.getByRole('button', { name: '关闭配置', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '继续编辑', exact: true }).click();
    assert.equal(await field.inputValue(), '保留草稿');
    await page.getByRole('button', { name: '撤销修改', exact: true }).click();
    assert.equal(await field.inputValue(), '初始值');
    await field.fill('新的配置');
    const count = page.getByRole('spinbutton', { name: '数量', exact: true });
    await count.fill('99');
    await page.getByRole('button', { name: '保存配置', exact: true }).click();
    assert.equal(requests.filter(r => r.path === '/api/mods/config').length, 0, '无效范围不能提交');
    await count.fill('2');
    state.configError = '配置写入失败';
    await page.getByRole('button', { name: '保存配置', exact: true }).click();
    await page.getByText('配置写入失败', { exact: true }).waitFor();
    assert.equal(await field.inputValue(), '新的配置');
    state.configError = '';
    await page.getByRole('button', { name: '保存配置', exact: true }).click();
    await page.getByText('未保存', { exact: true }).waitFor({ state: 'hidden' });
    await page.getByRole('button', { name: '关闭配置', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await configure.click();
    await field.waitFor();
    assert.equal(await field.inputValue(), '新的配置');
    assert.equal(await count.inputValue(), '2');
    await field.fill('放弃这次编辑');
    await page.getByRole('button', { name: '关闭配置', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '放弃修改', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
  } finally { await page.close(); }
});

test('添加窗口按 ID 下载不抢焦点或重复排队，下载后可直接启用，关闭再打开保留查询', async () => {
  const { page, state, requests } = await openMods({ updateStatuses: [
    { running: false, output: [] }, { running: true, progress: '20%', output: ['开始下载'] },
    { running: false, output: ['开始', '下载完成', '结束'] },
  ] });
  try {
    state.workshopResults = [{ id: '55555555', title: '新测试模组', installed: false }];
    await page.getByRole('button', { name: '添加模组', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '添加模组', exact: true });
    const input = dialog.getByRole('textbox', { name: '搜索创意工坊' });
    await input.fill('55555555');
    await input.press('Enter');
    await dialog.getByRole('link', { name: '新测试模组', exact: true }).waitFor();
    await dialog.getByRole('button', { name: '下载', exact: true }).click();
    const queued = dialog.getByRole('button', { name: '已加入队列', exact: true });
    await queued.waitFor();
    assert.equal(await queued.isDisabled(), true);
    assert.equal(await page.getByRole('dialog', { name: '模组更新', exact: true }).count(), 0);
    assert.equal(requests.filter(r => r.path === '/api/mods/update' && r.method === 'POST').length, 1);
    state.mods.push({ id: 'workshop-55555555', name: '新测试模组', enabled: false, installed: true });
    await dialog.getByRole('button', { name: '启用', exact: true }).click();
    await dialog.getByRole('button', { name: '已启用', exact: true }).waitFor();
    assert.equal(state.mods.find(mod => mod.id === 'workshop-55555555').enabled, true);
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    await page.waitForFunction(() => document.activeElement?.textContent.trim() === '添加模组');
    await page.getByRole('button', { name: '添加模组', exact: true }).click();
    await dialog.getByRole('link', { name: '新测试模组', exact: true }).waitFor();
    assert.equal(await input.inputValue(), '55555555');
  } finally { await page.close(); }
});

test('关键词缺少 Key 时就地配置并重试，结果可翻页且翻页不提交尚未搜索的新关键词', async () => {
  const { page, state, requests } = await openMods();
  try {
    state.searchError = '关键词搜索需要 Steam Web API Key';
    state.searchPages = {
      1: Array.from({ length: 20 }, (_, i) => ({ id: String(90000000 + i), title: `搜索结果 ${i + 1}` })),
      2: [{ id: '90000020', title: '第二页结果' }],
    };
    await page.getByRole('button', { name: '添加模组', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '添加模组', exact: true });
    const input = dialog.getByRole('textbox', { name: '搜索创意工坊' });
    await input.fill('背包');
    await input.press('Enter');
    const key = dialog.getByLabel('Steam Web API Key', { exact: true });
    await key.waitFor();
    assert.equal(await key.getAttribute('type'), 'password');
    assert.equal(requests.some(r => r.path === '/api/workshop/key'), false, '不把现有密钥读到页面');
    await key.fill('0123456789abcdef0123456789abcdef');
    await dialog.getByRole('button', { name: '保存', exact: true }).click();
    await dialog.getByRole('link', { name: '搜索结果 1', exact: true }).waitFor();
    await dialog.getByRole('button', { name: '下一页', exact: true }).click();
    await dialog.getByRole('link', { name: '第二页结果', exact: true }).waitFor();
    await input.fill('还没提交的新关键词');
    await dialog.getByRole('button', { name: '上一页', exact: true }).click();
    await dialog.getByRole('link', { name: '搜索结果 1', exact: true }).waitFor();
    const searches = requests.filter(r => r.path === '/api/workshop/search');
    assert.deepEqual(searches.map(r => r.params.page), ['1', '1', '2', '1']);
    assert.equal(searches.every(r => r.params.q === '背包'), true);
  } finally { await page.close(); }
});

test('检查更新只报告结果，用户确认更新后才下载，提交失败可以重试', async () => {
  const { page, state, requests } = await openMods({ updateStatuses: [
    { running: false, output: [] }, { running: true, progress: '10%', output: ['开始'] },
  ] });
  try {
    state.updates.updates = [{ id: mods[1].id, name: '已下载的工坊标题', localTime: 1, remoteTime: 2 }];
    await page.getByRole('button', { name: '检查更新', exact: true }).click();
    await page.getByText('1 个模组有新版本', { exact: true }).waitFor();
    assert.equal(requests.some(r => r.path === '/api/mods/update' && r.method === 'POST'), false);
    await page.getByRole('button', { name: '更新全部', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '取消', exact: true }).click();
    assert.equal(requests.some(r => r.path === '/api/mods/update' && r.method === 'POST'), false);
    state.startError = '下载服务暂不可用';
    const item = page.locator('.mod').first();
    await item.getByRole('button', { name: '下载', exact: true }).click();
    await page.getByText('下载服务暂不可用', { exact: true }).waitFor();
    assert.equal(await item.getByRole('button', { name: '下载', exact: true }).isEnabled(), true);
    state.startError = '';
    await page.getByRole('button', { name: '更新全部', exact: true }).click();
    await page.getByRole('alertdialog').getByRole('button', { name: '开始更新', exact: true }).click();
    await page.getByRole('button', { name: '查看进度', exact: true }).waitFor();
    assert.equal(requests.filter(r => r.path === '/api/mods/update' && r.method === 'POST').at(-1).params.id, mods[1].id);
    assert.equal(await page.getByRole('dialog', { name: '模组更新', exact: true }).count(), 0);
  } finally { await page.close(); }
});

test('手机上的添加与配置窗口不溢出，运行中的配置可查看且只读', async () => {
  const { page } = await openMods({ width: 375, running: true,
    schema: Array.from({ length: 12 }, (_, i) => ({ name: `option_${i}`, label: `选项 ${i}`, default: 'value' })),
  });
  try {
    await waitForTitle(page);
    await page.locator('.mod').nth(1).getByRole('button', { name: '配置', exact: true }).click();
    await page.getByLabel('选项 0', { exact: true }).waitFor();
    assert.equal(await page.getByLabel('选项 0', { exact: true }).isDisabled(), true);
    const save = page.getByRole('button', { name: '保存配置', exact: true });
    assert.equal(await save.isDisabled(), true);
    const box = await save.boundingBox();
    assert.ok(box.y >= 0 && box.y + box.height <= 900, '保存栏始终可见');
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    await page.getByRole('button', { name: '关闭配置', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    await page.getByRole('button', { name: '添加模组', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '添加模组', exact: true });
    await dialog.getByRole('textbox', { name: '搜索创意工坊' }).fill('1185229307');
    await dialog.getByRole('button', { name: '查询', exact: true }).click();
    await dialog.getByRole('link').first().waitFor();
    assert.equal(await dialog.getByRole('button', { name: '下载', exact: true }).isDisabled(), true);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    const bounds = await dialog.boundingBox();
    assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= 375 && bounds.y >= 0 && bounds.y + bounds.height <= 900);
  } finally { await page.close(); }
});

test('进度请求失败显示状态未知，重试恢复真实进度，关闭日志后焦点返回入口', async () => {
  const { page, state } = await openMods({ updateStatuses: [
    { running: false, output: [] }, { running: true, progress: '50%', output: ['恢复后的下载输出'] },
  ] });
  try {
    await page.waitForLoadState('networkidle');
    state.statusError = '进度接口暂不可用';
    await page.locator('.mod').first().getByRole('button', { name: '下载', exact: true }).click();
    await page.getByText('无法读取更新进度：进度接口暂不可用', { exact: true }).waitFor();
    await page.getByRole('button', { name: '更新日志', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '模组更新', exact: true });
    await dialog.getByText('进度暂不可用', { exact: true }).waitFor();
    state.statusError = '';
    await dialog.getByRole('button', { name: '重试', exact: true }).click();
    await dialog.getByText('50%', { exact: true }).waitFor();
    assert.match(await dialog.locator('pre').innerText(), /恢复后的下载输出/);
    await dialog.getByRole('button', { name: '关闭', exact: true }).click();
    await page.waitForFunction(() => document.activeElement?.textContent.trim() === '更新日志');
  } finally { await page.close(); }
});

test('Steam 不可达时已安装模组直接显示本地图标，无需访问工坊详情', async () => {
  const modItems = mods.map((mod, i) => ({ ...mod, name: `本地模组 ${i}`, installed: true, iconUrl: `/api/mods/icon?room=2&id=${mod.id}` }));
  const { page, requests } = await openMods({ failDetails: true, modItems });
  try {
    await page.waitForFunction(() => {
      const images = [...document.querySelectorAll('.mod-cover img')];
      return images.length === 4 && images.every(img => img.complete && img.naturalWidth > 0);
    });
    const images = page.locator('.mod-cover img');
    assert.equal(await images.count(), 4);
    for (const image of await images.all()) {
      assert.match(await image.getAttribute('src'), /^\/api\/mods\/icon\?/);
      assert.equal(await image.evaluate(el => el.naturalWidth / el.naturalHeight), 2);
    }
    assert.equal(requests.filter(r => r.path === '/api/mods/icon').length, 4);
    assert.equal(requests.some(r => r.path === '/api/workshop/details'), false);
  } finally { await page.close(); }
});

test('本地图标不可用时回退到工坊封面，两者都失败时显示占位且不重复请求本地图标', async () => {
  const modItems = mods.map(mod => ({ ...mod, installed: true, iconUrl: `/api/mods/icon?room=2&id=${mod.id}` }));
  const brokenIcons = [mods[0].id, mods[1].id];
  const { page, requests, state } = await openMods({ modItems, brokenIcons });
  try {
    await waitForTitle(page);
    await page.waitForFunction(() => {
      const first = document.querySelector('.mod .mod-cover img');
      const second = document.querySelectorAll('.mod')[1].querySelector('.mod-cover img');
      return first?.getAttribute('src') === 'https://fixtures.invalid/cover.svg' && first.complete && first.naturalWidth > 0 && !second;
    });
    assert.equal(await page.locator('.mod').nth(1).locator('.mod-cover svg').count(), 1);
    assert.equal(requests.filter(r => r.path === '/api/mods/icon' && r.params.id === mods[0].id).length, 1);
    assert.match(await page.locator('.mod').nth(2).locator('.mod-cover img').getAttribute('src'), /^\/api\/mods\/icon\?/);
    brokenIcons.length = 0;
    state.mods[0].iconUrl += '&v=updated';
    await page.getByRole('button', { name: '刷新', exact: true }).click();
    await page.waitForFunction(() => {
      const image = document.querySelector('.mod .mod-cover img');
      return image?.getAttribute('src')?.endsWith('&v=updated') && image.complete && image.naturalWidth > 0;
    });
    assert.equal(requests.filter(r => r.path === '/api/mods/icon' && r.params.id === mods[0].id).length, 2);
  } finally { await page.close(); }
});
