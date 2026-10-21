// 世界配置页的海难/火山适配回归。
//
// 在真实浏览器里跑：SSR 不会执行 onMounted，视图的加载流程只有真挂载才走得到。
// 接口全部被下面的 fetch 替身拦住，不读写任何真实存档。
import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { createServer } from 'vite';
import { chromium } from 'playwright';

const port = 16812;
let vite;
let browser;

before(async () => {
  vite = await createServer({
    root: new URL('..', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1'),
    server: { port, strictPort: true, host: '127.0.0.1' },
  });
  await vite.listen();
  browser = await chromium.launch();
});

after(async () => {
  await browser?.close();
  await vite?.close();
});

/** 后端 /api/worldconfig 的真实响应形态：location 来自 leveldataoverride.lua。 */
const SHIPWRECKED = {
  shard: 'Master', file: '/data/dst_save/DST/Cluster_2/Master/leveldataoverride.lua',
  exists: true, hasSave: true, location: 'shipwrecked', mod: 'island-adventures',
  levelDataInUse: true,
  config: {
    overrideEnabled: true, worldgenPreset: '', settingsPreset: '',
    overrides: { bamboo: 'often', mild: 'default', bamboo_watermelon: 'default' },
  },
  presets: {
    settings: [{ data: 'SHIPWRECKED_ENDLESS', label: '无尽' }],
    worldgen: [{ data: 'SHIPWRECKED_ENDLESS', label: '无尽' }],
  },
  groups: [
    {
      id: 'global', label: '全局', category: 'settings', order: 1,
      options: [{ name: 'mild', label: '温和季', default: 'default', values: [{ data: 'default', label: '默认' }, { data: 'shortseason', label: '短' }] }],
    },
    {
      id: 'resources', label: '资源', category: 'worldgen', order: 3,
      options: [{ name: 'bamboo', label: '竹子', default: 'default', values: [{ data: 'default', label: '默认' }, { data: 'often', label: '较多' }] }],
    },
  ],
  gameVersion: '756039', generatedAt: '2026-10-05T00:01:41+08:00',
};

async function openWorldPage(data) {
  data = globalThis.structuredClone(data);
  const page = await browser.newPage();
  const requests = [];
  await page.route('**/api/**', async route => {
    const url = new URL(route.request().url());
    const ok = payload => route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ code: 200, message: 'success', data: payload }),
    });
    requests.push({ path: url.pathname, query: Object.fromEntries(url.searchParams), method: route.request().method(), body: route.request().postData() });
    if (url.pathname === '/api/rooms') {
      return ok([{
        id: 2, name: '存档二', key: 'Cluster_2', gameMode: 'endless', maxPlayers: 4, needToken: false,
        shards: [{ name: 'Master', worldType: 'forest', isMaster: true }, { name: 'Caves', worldType: 'cave' }],
        // 存档里真实的位置：Master 是海难，Caves 是火山。
        // 目录名与房间配置都指向 forest/cave，必须凭它才能标对 tab。
        locations: { Master: 'shipwrecked', Caves: 'volcanoworld' },
      }]);
    }
    if (url.pathname === '/api/rooms/meta') return ok({ gameModes: [], tickRates: [] });
    if (url.pathname === '/api/status') return ok([{ shard: 'Master', state: 'stopped' }]);
    if (url.pathname === '/api/worldconfig') {
      if (route.request().method() === 'PUT') {
        const payload = route.request().postDataJSON();
        // 普通文件整表替换；游戏 leveldata 文件只合并规则增量。
        data.config = {
          ...payload,
          overrides: data.levelDataInUse
            ? { ...data.config.overrides, ...payload.overrides }
            : payload.overrides,
        };
        return ok({ needRestart: false });
      }
      return ok(data);
    }
    if (url.pathname === '/api/logs/stream') {
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' });
    }
    return ok([]);
  });
  await page.goto(`http://127.0.0.1:${port}/`);
  await page.evaluate(() => localStorage.setItem('dst-tab', 'world'));
  await page.reload();
  await page.getByRole('button', { name: '世界配置' }).first().waitFor();
  // 以“选项卡片出现且两个类别计数已渲染”作为加载完成信号：
  // 仅等按钮出现时，接口响应与显存渲染还没完成，后续操作会读到骨架屏。
  await page.locator('.world-options-card .seg-count').nth(1).waitFor({ timeout: 15000 });
  // 加载完成还会触发一次两帧的骨架屏，必须等它消失再操作，
  // 否则后续点击看到的仍是骨架屏占位。
  await page.waitForFunction(() => !document.querySelector('.world-category-skeleton'), null, { timeout: 15000 });
  await page.waitForFunction(() => document.querySelectorAll('.opt-row').length > 0, null, { timeout: 15000 });
  return { page, requests };
}

test('海难分片按存档 location 显示模组名，不再显示地表型', async () => {
  const { page } = await openWorldPage(SHIPWRECKED);
  const text = await page.locator('#page-content').innerText();
  assert.match(text, /岛屿冒险/, '应显示岛屿冒险');
  assert.doesNotMatch(text, /地表型/, '不应再按 rooms.json 的旧类型显示地表型');
  await page.close();
});

test('分片标签按 location 显示：Master=海难、Caves=火山', async () => {
  const { page } = await openWorldPage(SHIPWRECKED);
  // 目录名 Master / Caves 与真实世界无关，标签必须是存档里的 location。
  const tabs = await page.locator('.shard-tab').allInnerTexts();
  assert.deepEqual(tabs, ['海难', '火山'], `分片标签应为海难/火山，实际为 ${tabs.join('/')}`);
  await page.close();
});

test('存档已生成时隐藏预设与启用开关，世界生成只读', async () => {
  const { page } = await openWorldPage(SHIPWRECKED);
  const text = await page.locator('#page-content').innerText();
  assert.doesNotMatch(text, /世界生成预设/, '不应出现预设选择');
  assert.doesNotMatch(text, /启用本文件/, '不应出现启用开关');
  assert.match(text, /leveldataoverride\.lua/);

  // 默认停在有内容的类别：世界规则只有 1 项，这里应直接展示它
  assert.match(text, /温和季/, '默认应展示有内容的世界规则');

  // 用类别切换按钮的精确文案点击；getByRole 会同时命中导航与区域标题。
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  await page.waitForFunction(() => !document.querySelector('.world-category-skeleton'), null, { timeout: 5000 });
  await page.waitForFunction(
    () => document.querySelectorAll('.opt-row').length > 0, null, { timeout: 5000 },
  );
  assert.match(await page.locator('#page-content').innerText(), /竹子/);
  // 游戏内是两侧箭头翻值：只读时左右箭头与输入框都必须不可操作
  const controls = page.locator('.opt-row button, .opt-row input');
  const count = await controls.count();
  assert.ok(count > 0, '世界生成应有可渲染的控件');
  for (let i = 0; i < count; i++) {
    assert.equal(await controls.nth(i).isDisabled(), true, '世界生成类选项必须只读');
  }
  await page.close();
});

test('世界规则可改，保存只提交世界规则', async () => {
  const { page, requests } = await openWorldPage(SHIPWRECKED);
  const row = page.locator('.opt-row').first();
  await row.waitFor();
  // 游戏内布局：左右箭头翻值，不再用下拉框
  assert.equal(await row.locator('.opt-arrow').first().isDisabled(), false, '世界规则必须可编辑');

  await row.locator('.opt-arrow').last().click();
  await page.getByRole('button', { name: '保存' }).click();
  await page.waitForFunction(() => document.body.innerText.includes('已保存')
    || document.querySelector('[data-sonner-toast]') !== null, null, { timeout: 5000 });

  const put = requests.find(r => r.method === 'PUT' && r.path === '/api/worldconfig');
  assert.ok(put, '应发出保存请求');
  const payload = JSON.parse(put.body);
  assert.equal(payload.overrides.mild, 'shortseason', '右箭头应把 default 翻到下一项');
  assert.equal(payload.overrides.bamboo, undefined, '世界生成类项不得写入');
  await page.close();
});

async function saveAndRefresh(page, requests) {
  const loaded = page.waitForResponse(r => r.url().includes('/api/worldconfig') && r.request().method() === 'GET');
  await page.getByRole('button', { name: '保存', exact: true }).click();
  await loaded;
  await page.reload();
  await page.locator('.opt-row').first().waitFor();
  return JSON.parse(requests.filter(r => r.path === '/api/worldconfig' && r.method === 'PUT').at(-1).body);
}

function ordinaryWorld(hasSave = false) {
  const data = globalThis.structuredClone(SHIPWRECKED);
  data.levelDataInUse = false;
  data.hasSave = hasSave;
  data.file = '/fixture/worldgenoverride.lua';
  data.config.overrides.unknown_mod_key = 'custom';
  return data;
}

test('普通文件保存保留未知模组项，不展示也不计数', async () => {
  const { page, requests } = await openWorldPage(ordinaryWorld());
  assert.equal(await page.locator('.world-count').innerText(), '已改 1 项');
  assert.doesNotMatch(await page.locator('.world-options-card').innerText(), /unknown_mod_key/);
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  const payload = await saveAndRefresh(page, requests);
  assert.equal(payload.overrides.unknown_mod_key, 'custom');
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '短');
  assert.equal(await page.locator('.world-count').innerText(), '已改 2 项');
  await page.close();
});

test('普通已有存档保存规则时保留只读生成原值', async () => {
  const { page, requests } = await openWorldPage(ordinaryWorld(true));
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  const payload = await saveAndRefresh(page, requests);
  assert.equal(payload.overrides.bamboo, 'often');
  assert.equal(payload.overrides.unknown_mod_key, 'custom');
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  const bamboo = page.locator('.opt-row', { hasText: '竹子' });
  await bamboo.waitFor();
  assert.equal(await bamboo.locator('.opt-row-value').innerText(), '较多');
  assert.equal(await bamboo.locator('.opt-arrow').first().isDisabled(), true);
  await page.close();
});

test('leveldata 规则恢复 default 后发送显式增量且刷新持久化', async () => {
  const data = globalThis.structuredClone(SHIPWRECKED);
  data.config.overrides.mild = 'shortseason';
  const { page, requests } = await openWorldPage(data);
  await page.getByRole('button', { name: '温和季 上一项', exact: true }).click();
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '默认');
  assert.equal(await page.getByRole('button', { name: '保存', exact: true }).isEnabled(), true);
  const payload = await saveAndRefresh(page, requests);
  assert.deepEqual(payload.overrides, { mild: 'default' });
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '默认');
  assert.equal(await page.locator('.world-count').innerText(), '已改 1 项', '只读竹子仍偏离默认，载入的默认规则不计数');
  await page.close();
});

test('leveldata 重置持久化规则默认值且不提交只读生成项或无关默认项', async () => {
  const data = globalThis.structuredClone(SHIPWRECKED);
  data.config.overrides.mild = 'shortseason';
  data.groups[0].options.push({ name: 'untouched', label: '未改规则', default: 'default', values: [{ data: 'default', label: '默认' }] });
  data.config.overrides.untouched = 'default';
  const { page, requests } = await openWorldPage(data);
  await page.getByRole('button', { name: '重置', exact: true }).click();
  assert.equal(await page.getByRole('button', { name: '保存', exact: true }).isEnabled(), true);
  const payload = await saveAndRefresh(page, requests);
  assert.deepEqual(payload.overrides, { mild: 'default' });
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '默认');
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  const bamboo = page.locator('.opt-row', { hasText: '竹子' });
  await bamboo.waitFor();
  assert.equal(await bamboo.locator('.opt-row-value').innerText(), '较多');
  await page.close();
});

for (const action of ['恢复默认', '重置']) {
  test(`普通已有存档${action}持久化，同时保留未知项和只读生成值`, async () => {
    const data = ordinaryWorld(true);
    data.config.overrides.mild = 'shortseason';
    const { page, requests } = await openWorldPage(data);
    if (action === '重置') await page.getByRole('button', { name: '重置', exact: true }).click();
    else await page.getByRole('button', { name: '温和季 上一项', exact: true }).click();
    const payload = await saveAndRefresh(page, requests);
    assert.equal(payload.overrides.mild, 'default');
    assert.equal(payload.overrides.bamboo, 'often');
    assert.equal(payload.overrides.unknown_mod_key, 'custom');
    assert.equal(await page.locator('.opt-row-value').first().innerText(), '默认');
    await page.close();
  });
}

test('leveldata 默认项不计改动，未编辑的非默认规则不进 patch，改回载入值撤销 patch', async () => {
  const data = globalThis.structuredClone(SHIPWRECKED);
  data.groups[0].options.push({ name: 'other', label: '其他规则', default: 'default', values: [{ data: 'default', label: '默认' }, { data: 'often', label: '较多' }] });
  data.config.overrides.other = 'often';
  for (let i = 0; i < 100; i++) {
    data.groups[0].options.push({ name: `rule${i}`, label: `规则${i}`, default: 'default', values: [{ data: 'default', label: '默认' }] });
    data.config.overrides[`rule${i}`] = 'default';
  }
  const { page, requests } = await openWorldPage(data);
  assert.equal(await page.locator('.world-count').innerText(), '已改 2 项');
  assert.equal(await page.getByRole('button', { name: '保存', exact: true }).isDisabled(), true);
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  await page.getByRole('button', { name: '温和季 上一项', exact: true }).click();
  assert.equal(await page.getByRole('button', { name: '保存', exact: true }).isDisabled(), true);
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  const payload = await saveAndRefresh(page, requests);
  assert.deepEqual(payload.overrides, { mild: 'shortseason' });
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '短');
  await page.close();
});

test('保存 URL 的 room/shard 查询参数与 body 一致', async () => {
  const { page, requests } = await openWorldPage(SHIPWRECKED);
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  const payload = await saveAndRefresh(page, requests);
  const put = requests.find(r => r.method === 'PUT');
  assert.deepEqual(put.query, { room: '2', shard: 'Master' });
  assert.equal(payload.room, 2);
  assert.equal(payload.shard, 'Master');
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '短');
  await page.close();
});

function islandWorld(hasSave = false) {
  const data = ordinaryWorld(hasSave);
  data.volcanoIslandOnly = ['island_rule', 'island_resource'];
  data.config.overrides.island_rule = 'often';
  data.config.overrides.island_resource = 'often';
  data.groups[0].options.push({ name: 'island_rule', label: '岛屿规则', default: 'default', values: [{ data: 'default', label: '默认' }, { data: 'often', label: '较多' }] });
  data.groups[1].options.push(
    { name: 'volcanoisland', label: '火山岛', default: 'none', values: [{ data: 'none', label: '无' }, { data: 'default', label: '默认' }] },
    { name: 'island_resource', label: '岛屿资源', default: 'default', values: [{ data: 'default', label: '默认' }, { data: 'often', label: '较多' }] },
  );
  return data;
}

test('火山岛缺省 none 隐藏候选项，开关即时更新显隐计数搜索，隐藏值保存后保留', async () => {
  const { page, requests } = await openWorldPage(islandWorld());
  assert.match(await page.locator('.world-options-card').innerText(), /共 3 项/);
  assert.equal(await page.locator('.world-count').innerText(), '已改 1 项');
  assert.deepEqual(await page.locator('.seg-count').allInnerTexts(), ['1', '2']);
  assert.equal(await page.locator('.opt-row', { hasText: '岛屿规则' }).count(), 0);
  const search = page.getByPlaceholder('搜索选项（中英文均可）');
  await search.fill('岛屿规则');
  assert.equal(await page.locator('.opt-row').count(), 0);
  await search.fill('');
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  await page.getByRole('button', { name: '火山岛 下一项', exact: true }).click();
  assert.match(await page.locator('.world-options-card').innerText(), /共 5 项/);
  assert.deepEqual(await page.locator('.seg-count').allInnerTexts(), ['2', '3']);
  assert.equal(await page.locator('.opt-row', { hasText: '岛屿资源' }).locator('.opt-row-value').innerText(), '较多');
  await page.locator('.seg-btn', { hasText: '世界规则' }).click();
  await search.fill('岛屿规则');
  await page.locator('.opt-row', { hasText: '岛屿规则' }).waitFor();
  assert.equal(await page.locator('.opt-row').count(), 1);
  await search.fill('');
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  await page.getByRole('button', { name: '火山岛 上一项', exact: true }).click();
  assert.equal(await page.locator('.opt-row', { hasText: '岛屿资源' }).count(), 0);
  await page.getByRole('button', { name: '火山岛 下一项', exact: true }).click();
  assert.equal(await page.locator('.opt-row', { hasText: '岛屿资源' }).locator('.opt-row-value').innerText(), '较多');
  await page.getByRole('button', { name: '火山岛 上一项', exact: true }).click();
  await page.locator('.seg-btn', { hasText: '世界规则' }).click();
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  const payload = await saveAndRefresh(page, requests);
  assert.equal(payload.overrides.island_rule, 'often');
  assert.equal(payload.overrides.island_resource, 'often');
  assert.match(await page.locator('.world-options-card').innerText(), /共 3 项/);
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  await page.getByRole('button', { name: '火山岛 下一项', exact: true }).click();
  assert.equal(await page.locator('.opt-row', { hasText: '岛屿资源' }).locator('.opt-row-value').innerText(), '较多');
  await page.close();
});

test('已有存档火山岛开关保持只读，隐藏候选值也不被重置清除', async () => {
  const { page, requests } = await openWorldPage(islandWorld(true));
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  await page.getByRole('button', { name: '重置', exact: true }).click();
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  const payload = await saveAndRefresh(page, requests);
  assert.equal(payload.overrides.island_rule, 'often');
  assert.equal(payload.overrides.island_resource, 'often');
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  assert.equal(await page.getByRole('button', { name: '火山岛 下一项', exact: true }).isDisabled(), true);
  assert.equal(await page.locator('.opt-row', { hasText: '岛屿资源' }).count(), 0);
  await page.close();
});

test('图标按选项名正确落到模组目录', async () => {
  const { page } = await openWorldPage(SHIPWRECKED);
  await page.locator('.opt-row img').first().waitFor();
  const srcs = await page.locator('.opt-row img').evaluateAll(els => els.map(e => e.getAttribute('src')));
  assert.ok(srcs.every(src => src.startsWith('/world-icons/')), srcs.join(','));
  await page.close();
});
