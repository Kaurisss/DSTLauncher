// 图标映射与静态素材回归；无需启动面板或游戏。
import assert from 'node:assert/strict';
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { WORLD_ICONS, iconOf } from '../src/lib/worldIcons.js';

const root = new URL('../public/world-icons/', import.meta.url);
const source = new URL('../../tools/world-icon-sources/island-adventures/', import.meta.url);
const folders = ['common', 'dst', 'island-adventures', 'island-adventures-core'];
assert.deepEqual(readdirSync(root).sort(), folders.sort(), '图标根目录只保留分类目录');

for (const [name, file] of Object.entries(WORLD_ICONS)) {
  assert.match(file, /^(common|dst|island-adventures|island-adventures-core)\/[a-z0-9_]+\.png$/);
  assert.ok(existsSync(new URL(file, root)), `${name} 引用的图标不存在：${file}`);
  assert.equal(iconOf(name), `/world-icons/${file}`);
}

const dataset = JSON.parse(readFileSync(new URL('../../internal/worldsettings/options.json', import.meta.url)));
for (const group of dataset.groups) {
  for (const option of group.options) assert.ok(iconOf(option.name), `原有选项丢失图标：${option.name}`);
}

for (const name of ['missing_icon', 'constructor', '__proto__', 'toString']) assert.equal(iconOf(name), '');
assert.equal(iconOf('grass'), '/world-icons/common/grass.png');
assert.equal(iconOf('beequeen'), '/world-icons/dst/beequeen.png');
assert.equal(iconOf('cave_ponds'), iconOf('ponds'), '相同素材只保留一份');
assert.equal(iconOf('bamboo'), '/world-icons/island-adventures/bamboo.png');
assert.equal(iconOf('dragoonegg'), '/world-icons/island-adventures/dragooneggs.png');
assert.equal(iconOf('ia_boats'), '/world-icons/island-adventures-core/smallboats.png');
assert.equal(iconOf('poison'), '/world-icons/island-adventures-core/poison.png');
assert.notEqual(iconOf('primeape_setting'), iconOf('primeape'), '猿猴数量与原版小窝图不能混用');
assert.equal(iconOf('shipwrecked_trees'), iconOf('palmtree_regrowth'));
assert.equal(iconOf('rock_obsidian'), iconOf('rock_obsidian_regrowth'));
assert.notEqual(iconOf('trees'), iconOf('shipwrecked_trees'), '同名图集变体不得覆盖原版');
assert.notEqual(iconOf('mosquitos'), iconOf('mosquito'));
assert.ok(existsSync(new URL('customization_shipwrecked.png', source)));
const xml = readFileSync(new URL('customization_shipwrecked.xml', source), 'utf8');
const slices = [...xml.matchAll(/<Element name="([a-zA-Z0-9_]+)\.tex"/g)].map(m => `${m[1]}.png`);
assert.equal(slices.length, 89);
const iaFiles = readdirSync(new URL('island-adventures/', root), { withFileTypes: true })
  .filter(entry => entry.isFile())
  .map(entry => entry.name)
  .sort();
assert.deepEqual(iaFiles, slices.sort(), '完整保留图集切片，包括尚未使用的变体');

const coreSource = new URL('../../tools/world-icon-sources/island-adventures-core/', import.meta.url);
assert.ok(existsSync(new URL('customization_core.png', coreSource)));
const coreXml = readFileSync(new URL('customization_core.xml', coreSource), 'utf8');
const coreSlices = [...coreXml.matchAll(/<Element name="([a-zA-Z0-9_]+)\.tex"/g)].map(m => `${m[1]}.png`);
assert.equal(coreSlices.length, 7);
assert.deepEqual(readdirSync(new URL('island-adventures-core/', root)).sort(), coreSlices.sort());

// 原版目录不留失去引用的文件；模组目录保留完整切片以便后续适配。
const referenced = new Set(Object.values(WORLD_ICONS));
for (const folder of ['common', 'dst']) {
  for (const entry of readdirSync(new URL(`${folder}/`, root), { withFileTypes: true })) {
    if (!entry.isFile()) continue;
    assert.ok(referenced.has(`${folder}/${entry.name}`), `未使用的原版图标：${folder}/${entry.name}`);
  }
}

// 地图成对彩色图标：虫洞（22 组）与电光三角（24 组）
const wormholeFiles = readdirSync(new URL('dst/wormhole/', root)).sort();
const expectedWormholes = Array.from({ length: 22 }, (_, i) => `mark_${i + 1}.png`).sort();
assert.deepEqual(wormholeFiles, expectedWormholes, '虫洞彩色标记应完整保留 mark_1..22.png');

const triangleFiles = readdirSync(new URL('island-adventures/electric isosceles/', root)).sort();
assert.equal(triangleFiles.length, 24, '电光三角彩色图标应完整保留 24 张');
console.log(`图标检查通过（${Object.keys(WORLD_ICONS).length} 项映射，${slices.length} 张岛屿冒险切片，${wormholeFiles.length} 张虫洞图标，${triangleFiles.length} 张电光三角图标）`);