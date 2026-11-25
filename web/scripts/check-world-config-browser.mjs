// 世界配置页的海难/火山适配回归。
//
// 在真实浏览器里跑：SSR 不会执行 onMounted，视图的加载流程只有真挂载才走得到。
// 接口全部被下面的 fetch 替身拦住，不读写任何真实存档。
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
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
  browser = await chromium.launch({ executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH });
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

async function openWorldPage(data, options = {}) {
  data = globalThis.structuredClone(data);
  const byShard = data.Master ? data : null;
  const shardOf = name => (byShard ? byShard[name] || byShard.Master : data);
  const page = await browser.newPage({ viewport: { width: options.width || 1280, height: 800 } });
  await page.addInitScript(dark => { localStorage.setItem('dst-theme', dark ? 'dark' : 'light'); }, !!options.dark);
  await page.emulateMedia({ reducedMotion: 'reduce' });
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
    if (url.pathname === '/api/status') return ok([{ shard: 'Master', state: options.running ? 'running' : 'stopped' }]);
    if (url.pathname === '/api/worldconfig') {
      if (route.request().method() === 'PUT') {
        const payload = route.request().postDataJSON();
        const target = shardOf(payload.shard);
        // 普通文件整表替换；游戏 leveldata 文件只合并规则增量。
        target.config = {
          ...payload,
          overrides: target.levelDataInUse
            ? { ...target.config.overrides, ...payload.overrides }
            : payload.overrides,
        };
        return ok({ needRestart: false });
      }
      return ok(shardOf(url.searchParams.get('shard')));
    }
    if (url.pathname === '/api/logs/stream') {
      return route.fulfill({ status: 200, contentType: 'text/event-stream', body: '' });
    }
    return ok([]);
  });
  await page.goto(`http://127.0.0.1:${port}/`);
  await page.evaluate(() => localStorage.setItem('dst-tab', 'world'));
  await page.reload();
  // 等类别计数和配置项，避免在接口尚未返回时开始操作。
  await page.locator('.world-options-card .seg-count').nth(1).waitFor({ timeout: 15000 });
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
  assert.doesNotMatch(text, /leveldataoverride\.lua/, '路径不常驻占用页面高度');
  await page.getByRole('button', { name: '路径与说明', exact: true }).click();
  const details = page.getByRole('dialog', { name: '路径与说明', exact: true });
  assert.match(await details.innerText(), /leveldataoverride\.lua/);
  await page.keyboard.press('Escape');
  await details.waitFor({ state: 'hidden' });

  // 默认停在有内容的类别：世界规则只有 1 项，这里应直接展示它
  assert.match(text, /温和季/, '默认应展示有内容的世界规则');

  // 用类别切换按钮的精确文案点击；getByRole 会同时命中导航与区域标题。
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  await page.waitForFunction(() => !document.querySelector('.world-category-skeleton'), null, { timeout: 5000 });
  await page.waitForFunction(
    () => document.querySelectorAll('.opt-row').length > 0, null, { timeout: 5000 },
  );
  assert.match(await page.locator('#page-content').innerText(), /竹子/);
  assert.equal(await page.getByText('地图已生成，仅供查看', { exact: true }).isVisible(), true);
  // 只读时下拉、步进和恢复默认都必须不可操作。
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
  assert.equal(await row.getByRole('combobox').count(), 0);
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
    else await page.getByRole('button', { name: '温和季 恢复默认', exact: true }).click();
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
  assert.equal(await page.locator('.world-count').innerText(), '已改 1 项');
  assert.deepEqual(await page.locator('.seg-count').allInnerTexts(), ['1', '2']);
  assert.equal(await page.locator('.opt-row', { hasText: '岛屿规则' }).count(), 0);
  const search = page.getByPlaceholder('搜索选项（中英文均可）');
  await search.fill('岛屿规则');
  assert.equal(await page.locator('.opt-row').count(), 0);
  await search.fill('');
  await page.locator('.seg-btn', { hasText: '世界生成' }).click();
  await page.getByRole('button', { name: '火山岛 下一项', exact: true }).click();

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
  assert.deepEqual(await page.locator('.seg-count').allInnerTexts(), ['1', '2']);
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

test('图标按选项名正确落到模组目录，且切换分类后首帧即已就绪不闪烁', async () => {
  const { page } = await openWorldPage(SHIPWRECKED);
  await page.locator('.opt-row img').first().waitFor();
  const srcs = await page.locator('.opt-row img').evaluateAll(els => els.map(e => ({
    src: e.getAttribute('src'),
    loading: e.getAttribute('loading'),
  })));
  assert.ok(srcs.every(item => item.src.startsWith('/world-icons/')), JSON.stringify(srcs));
  assert.ok(srcs.every(item => item.loading !== 'lazy'), '图标不应使用 loading="lazy" 避免切标签闪烁');
  // 等空闲预热解码完成后切换分类，校验首帧所有新图标立即完整绘制
  await page.waitForTimeout(150);
  const firstFrameReady = await page.evaluate(async () => {
    document.querySelectorAll('.seg-btn')[1].click();
    await Promise.resolve();
    await Promise.resolve();
    const imgs = [...document.querySelectorAll('.opt-row img')];
    return imgs.length > 0 && imgs.every(img => img.complete && img.naturalWidth > 0);
  });
  assert.equal(firstFrameReady, true, '切到世界生成后首帧图标应直接命中解码缓存');
  await page.close();
});

test('改动计数与重置保存位于选项卡头右侧，切分片保留并一并保存未提交改动', async () => {
  const caves = globalThis.structuredClone(SHIPWRECKED);
  caves.shard = 'Caves';
  caves.location = 'volcanoworld';
  caves.file = '/data/dst_save/DST/Cluster_2/Caves/leveldataoverride.lua';
  caves.config.overrides = { dragoons: 'default' };
  caves.groups = [{
    id: 'monsters', label: '怪物', category: 'settings', order: 1,
    options: [{ name: 'dragoons', label: '火龙', default: 'default', values: [{ data: 'default', label: '默认' }, { data: 'often', label: '较多' }] }],
  }];

  const { page, requests } = await openWorldPage({ Master: SHIPWRECKED, Caves: caves });
  const header = page.locator('.world-options-card .world-control-header');
  await header.locator('.world-count').waitFor();
  await header.getByRole('button', { name: '重置', exact: true }).waitFor();
  await header.getByRole('button', { name: '保存', exact: true }).waitFor();

  // 在海难分片修改世界规则，切换到火山分片后再切回，未保存改动不丢失
  await page.getByRole('button', { name: '温和季 下一项', exact: true }).click();
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '短');

  await page.locator('.shard-tab', { hasText: '火山' }).click();
  await page.locator('.opt-row', { hasText: '火龙' }).waitFor();
  assert.equal(await header.getByRole('button', { name: '保存', exact: true }).isEnabled(), true);

  await page.locator('.shard-tab', { hasText: '海难' }).click();
  await page.locator('.opt-row', { hasText: '温和季' }).waitFor();
  assert.equal(await page.locator('.opt-row-value').first().innerText(), '短');

  // 切到火山再改一项并保存，两个分片的改动都会落盘
  await page.locator('.shard-tab', { hasText: '火山' }).click();
  await page.getByRole('button', { name: '火龙 下一项', exact: true }).click();
  await saveAndRefresh(page, requests);

  const puts = requests.filter(r => r.path === '/api/worldconfig' && r.method === 'PUT').map(r => JSON.parse(r.body));
  assert.equal(puts.length, 2);
  assert.deepEqual(puts.map(p => [p.shard, p.overrides]), [
    ['Master', { mild: 'shortseason' }],
    ['Caves', { dragoons: 'often' }],
  ]);

  // 桌面端工具栏单行排列，窄屏下顶部导航栏不溢出且工具栏分为上下两行
  await page.setViewportSize({ width: 1280, height: 800 });
  const desktopToolbar = await page.evaluate(() => {
    const seg = document.querySelector('.world-toolbar .seg').getBoundingClientRect();
    const search = document.querySelector('.world-toolbar .world-search').getBoundingClientRect();
    const sw = document.querySelector('.world-toolbar label').getBoundingClientRect();
    return { segTop: seg.top, searchTop: search.top, swTop: sw.top };
  });
  assert.ok(Math.abs(desktopToolbar.segTop - desktopToolbar.searchTop) < 8, '桌面端分类与搜索应同行');
  assert.ok(Math.abs(desktopToolbar.searchTop - desktopToolbar.swTop) < 8, '桌面端搜索与只看已改动应同行');

  await page.setViewportSize({ width: 375, height: 812 });
  await page.waitForFunction(() => document.querySelector('[data-slot="sidebar-wrapper"]').getBoundingClientRect().width === window.innerWidth);
  const mobileLayout = await page.evaluate(() => {
    const h = document.querySelector('header');
    const btn = document.querySelector('header button[aria-label="切换主题"]');
    const seg = document.querySelector('.world-toolbar .seg').getBoundingClientRect();
    const search = document.querySelector('.world-toolbar .world-search').getBoundingClientRect();
    const sw = document.querySelector('.world-toolbar label').getBoundingClientRect();
    return {
      scrollWidth: h.scrollWidth,
      clientWidth: h.clientWidth,
      btnRight: btn.getBoundingClientRect().right,
      segBottom: seg.bottom,
      searchTop: search.top,
      swTop: sw.top,
    };
  });
  assert.ok(mobileLayout.scrollWidth <= mobileLayout.clientWidth, `header 不应溢出: ${JSON.stringify(mobileLayout)}`);
  assert.ok(mobileLayout.btnRight <= 375, `主题按钮不应超出屏幕右侧: ${JSON.stringify(mobileLayout)}`);
  assert.ok(mobileLayout.searchTop >= mobileLayout.segBottom, '移动端搜索应换到分类下一行');
  assert.ok(Math.abs(mobileLayout.searchTop - mobileLayout.swTop) < 8, '移动端搜索与只看已改动应同行');
  await page.close();
});

test('选项名称与值为普通文本，步进切换未知值，单项还原同步分组徽章', async () => {
  const data = ordinaryWorld();
  data.config.overrides.mild = 'from-future-version';
  const { page, requests } = await openWorldPage(data);
  try {
    const row = page.locator('.opt-row', { hasText: '温和季' });
    const nav = page.getByRole('navigation', { name: '配置分组' });
    assert.equal(await row.locator('.opt-row-value').innerText(), 'from-future-version');
    assert.equal(await row.getByRole('combobox').count(), 0);
    await row.locator('.opt-row-value').click();
    assert.equal(await page.getByRole('listbox').count(), 0, '点击文本不展开菜单');
    await row.getByRole('button', { name: '温和季 下一项', exact: true }).click();
    assert.equal(await row.locator('.opt-row-value').innerText(), '短');
    assert.equal(await nav.locator('.world-group-changes').innerText(), '1');
    await row.hover();
    const reset = row.getByRole('button', { name: '温和季 恢复默认', exact: true });
    assert.equal(await reset.evaluate(el => window.getComputedStyle(el).opacity), '1');
    await reset.focus();
    await page.keyboard.press('Enter');
    assert.equal(await row.locator('.opt-row-value').innerText(), '默认');
    assert.equal(await nav.locator('.world-group-changes').count(), 0);
    const payload = await saveAndRefresh(page, requests);
    assert.equal(payload.overrides.mild, 'default');
  } finally { await page.close(); }
});

test('普通文件的预设与启用开关直接展示在配置栏，关服前所有编辑控件冻结', async () => {
  const { page, requests } = await openWorldPage(ordinaryWorld(true), { running: true });
  try {
    await page.getByText('服务器运行中，配置已冻结。请先在「概览」关闭服务器再修改。', { exact: true }).waitFor();
    await page.getByRole('button', { name: '前往概览', exact: true }).waitFor();
    for (const control of await page.locator('.opt-row button, .opt-row input').all()) assert.equal(await control.isDisabled(), true);
    const presets = page.getByRole('region', { name: '预设与启用', exact: true });
    assert.equal(await presets.getByRole('combobox', { name: '世界生成预设', exact: true }).isDisabled(), true);
    assert.equal(await presets.getByRole('combobox', { name: '世界规则预设', exact: true }).isDisabled(), true);
    assert.equal(await presets.getByRole('switch', { name: '启用本文件', exact: true }).isDisabled(), true);
    assert.equal(requests.filter(r => r.method === 'PUT').length, 0);
  } finally { await page.close(); }
});

test('真实选项布局紧凑，桌面两行吸顶保存搜索，浅色深色和手机均无横向溢出', async () => {
  const dataset = JSON.parse(readFileSync(new URL('../../internal/worldsettings/options.json', import.meta.url), 'utf8'));
  const data = globalThis.structuredClone(SHIPWRECKED);
  data.location = 'forest';
  data.mod = '';
  data.groups = dataset.groups.map(g => ({ ...g, options: g.options.filter(o => o.worlds.includes('forest')) })).filter(g => g.options.length);
  data.config.overrides = {};
  const largest = data.groups.filter(g => g.category !== 'worldgen').sort((a, b) => b.options.length - a.options.length)[0];
  for (const [width, dark] of [[2240, false], [1440, true], [375, false]]) {
    const { page } = await openWorldPage(data, { width, dark });
    try {
      const nav = page.getByRole('navigation', { name: '配置分组' });
      await nav.getByRole('button', { name: largest.label, exact: true }).click();
      const rows = page.locator('.opt-row');
      assert.equal(await rows.count(), largest.options.length);
      const sizes = await page.evaluate(() => {
        const row = document.querySelector('.opt-row').getBoundingClientRect();
        const icon = document.querySelector('.opt-row-icon').getBoundingClientRect();
        const grid = window.getComputedStyle(document.querySelector('.opt-group-body')).gridTemplateColumns.split(' ').length;
        return { rowHeight: row.height, iconWidth: icon.width, iconHeight: icon.height, grid };
      });
      assert.ok(sizes.rowHeight <= 80, '卡片保持紧凑');
      assert.equal(sizes.iconWidth, 56);
      assert.equal(sizes.iconHeight, 56);
      if (width === 2240) assert.equal(sizes.grid, 4);
      if (width === 1440) assert.ok(sizes.grid >= 3);
      await page.locator('[data-slot="page-scroll"]').evaluate(el => el.scrollTo({ top: 0 }));
      await page.screenshot({ path: fileURLToPath(new URL(`../../.scratch/screenshots/world-workspace-${width}-${dark ? 'dark' : 'light'}.png`, import.meta.url)) });
      await page.setViewportSize({ width, height: 360 });
      const headerTop = (await page.locator('.world-control-header').boundingBox()).y;
      await rows.last().scrollIntoViewIfNeeded();
      const position = await page.evaluate(() => {
        const header = document.querySelector('.world-control-header').getBoundingClientRect();
        const main = document.querySelector('#page-content').getBoundingClientRect();
        const root = document.querySelector('[data-slot="page-scroll"]');
        const options = document.querySelector('.world-group-content');
        return { headerTop: header.top, mainScroll: document.querySelector('#page-content').scrollTop, headerBottom: header.bottom, viewportBottom: main.bottom, scrollTop: options.scrollTop, scrollWidth: root.scrollWidth, clientWidth: root.clientWidth };
      });
      assert.ok(position.scrollTop > 0, '卡片列表真实发生滚动');
      assert.equal(position.mainScroll, 0);
      assert.ok(Math.abs(position.headerTop - headerTop) < 2, '完整两行控制栏保持不动');
      assert.ok(position.headerBottom < position.viewportBottom, '吸顶控件仍在视口内');
      assert.ok(position.scrollWidth <= position.clientWidth, '主工作区不横向溢出');
      assert.equal(await page.getByRole('button', { name: '保存', exact: true }).isVisible(), true);
      const scroller = page.getByRole('region', { name: '世界配置选项', exact: true });
      await scroller.evaluate(el => { el.scrollTop = el.scrollHeight; });
      const scrolled = await page.evaluate(() => {
        const nav = document.querySelector('.world-group-nav').getBoundingClientRect();
        const list = document.querySelector('[data-slot="page-scroll"]').getBoundingClientRect();
        const options = document.querySelector('.world-group-content');
        return {
          navTop: nav.top, listTop: list.top, optionsScroll: options.scrollTop,
          outerScroll: document.querySelector('[data-slot="page-scroll"]').scrollTop,
          pageScroll: document.querySelector('#page-content').scrollTop,
        };
      });
      assert.ok(scrolled.optionsScroll > 0, '选项容器独立滚动');
      assert.ok(Math.abs(scrolled.navTop - scrolled.listTop) < 16, '分组导航不随选项滚走');
      assert.equal(scrolled.outerScroll, 0);
      assert.equal(scrolled.pageScroll, 0);
      await page.getByRole('textbox', { name: '搜索世界配置', exact: true }).fill('活动');
      await page.locator('.opt-row-label').getByText('活动', { exact: true }).waitFor();
      assert.equal(await nav.getByRole('button', { name: '全局', exact: true }).getAttribute('aria-current'), 'true', '吸顶搜索跨组定位');
      const listBounds = await page.locator('[data-slot="page-scroll"]').boundingBox();
      const optionBounds = await page.locator('.opt-row-label').getByText('活动', { exact: true }).boundingBox();
      assert.ok(optionBounds.y >= listBounds.y && optionBounds.y + optionBounds.height <= listBounds.y + listBounds.height, JSON.stringify({ width, listBounds, optionBounds }));
    } finally { await page.close(); }
  }
});

test('支持清空搜索、筛选空态恢复、按组恢复默认、分组按钮等高与搜索图标不重叠', async () => {
  const data = ordinaryWorld();
  data.groups[0].options.push({
    name: 'autumn', label: '秋季', default: 'default',
    values: [{ data: 'default', label: '默认' }, { data: 'shortseason', label: '短' }],
  });
  const { page } = await openWorldPage(data);
  try {
    const navBtn = page.getByRole('navigation', { name: '配置分组' }).getByRole('button').first();
    const groupHead = page.locator('.opt-group-head');
    const heightWithoutBadge = (await navBtn.boundingBox()).height;
    const headHeightWithoutReset = (await groupHead.boundingBox()).height;

    // 搜索框左侧内边距必须大于放大镜图标右边界，避免文本重叠
    const searchMetrics = await page.evaluate(() => {
      const icon = document.querySelector('.world-search-icon').getBoundingClientRect();
      const input = document.querySelector('.world-search');
      const rect = input.getBoundingClientRect();
      const padLeft = parseFloat(window.getComputedStyle(input).paddingLeft);
      return { iconRightOffset: icon.right - rect.left, padLeft };
    });
    assert.ok(searchMetrics.padLeft >= searchMetrics.iconRightOffset + 4, JSON.stringify(searchMetrics));

    // 键盘左右方向键步进 + 改动后分组按钮与分组头高度不跳动 + 按组恢复默认
    const mildNext = page.getByRole('button', { name: '温和季 下一项', exact: true });
    await mildNext.focus();
    await page.keyboard.press('ArrowRight');
    assert.equal(await page.locator('.opt-row', { hasText: '温和季' }).locator('.opt-row-value').innerText(), '短');
    assert.equal(await page.locator('.opt-changed-dot').count(), 0, '不渲染装饰性圆点');
    assert.equal(await page.locator('.shard-tab[data-state="active"] .shard-tab-dirty').count(), 1, '有未保存改动时显示分片草稿标记');
    const heightWithBadge = (await navBtn.boundingBox()).height;
    const headHeightWithReset = (await groupHead.boundingBox()).height;
    assert.equal(heightWithBadge, heightWithoutBadge, '分组按钮有/无徽章高度必须完全一致');
    assert.equal(headHeightWithReset, headHeightWithoutReset, '分组标题栏有/无恢复按钮高度必须完全一致');
    await page.screenshot({ path: fileURLToPath(new URL('../../.scratch/screenshots/world-changed-light.png', import.meta.url)) });

    await page.getByRole('button', { name: '恢复本组默认', exact: true }).click();
    assert.equal(await page.locator('.opt-row', { hasText: '温和季' }).locator('.opt-row-value').innerText(), '默认');

    // 搜索无结果时可一键清空搜索
    const search = page.getByRole('textbox', { name: '搜索世界配置', exact: true });
    await search.fill('完全不存在的选项名称');
    await page.getByText('没有找到匹配的配置项', { exact: true }).waitFor();
    await page.getByRole('button', { name: '清空搜索', exact: true }).first().click();
    assert.equal(await search.inputValue(), '');
    assert.ok(await page.locator('.opt-row').count() >= 2);
  } finally { await page.close(); }
});
