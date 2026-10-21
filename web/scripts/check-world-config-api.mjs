// 用真实存档副本验证接口；只读原存档，写入落在临时目录。
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { cpSync, mkdtempSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { after, test } from 'node:test';

const ORIGIN = join(fileURLToPath(new URL('../..', import.meta.url)));
const SAVE = join(ORIGIN, 'data', 'dst_save');
const EXE = join(ORIGIN, 'dist-test', 'dst-panel-test.exe');
const PORT = 16899;

const work = mkdtempSync(join(tmpdir(), 'dst-panel-'));
const saveDir = join(work, 'dst_save');
cpSync(join(SAVE, 'DST'), join(saveDir, 'DST'), { recursive: true });
cpSync(join(SAVE, 'rooms.json'), join(saveDir, 'rooms.json'));

const panel = spawn(EXE, ['-port', String(PORT), '-data-dir', work], { stdio: 'ignore' });
after(() => {
  panel.kill();
  rmSync(work, { recursive: true, force: true });
});

async function waitReady() {
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${PORT}/healthz`);
      if (r.ok) return;
    } catch (_) { /* 还没起来 */ }
    await new Promise(r => setTimeout(r, 250));
  }
  throw new Error('面板没能在 15 秒内启动');
}

async function api(path, init) {
  const r = await fetch(`http://127.0.0.1:${PORT}${path}`, init);
  const json = await r.json();
  assert.equal(json.code, 200, `${path}: ${json.message}`);
  return json.data;
}

test('真实存档：四个分片都识别出正确世界类型', async () => {
  await waitReady();
  const rooms = await api('/api/rooms');
  const room = rooms.find(r => r.key === 'Cluster_2');
  assert.ok(room, '应有 Cluster_2');

  const expect = { Master: 'shipwrecked', Caves: 'volcanoworld', Forest: 'forest', Cave: 'cave' };
  for (const [shard, location] of Object.entries(expect)) {
    const v = await api(`/api/worldconfig?room=${room.id}&shard=${shard}`);
    assert.equal(v.location, location, `${shard} 世界类型`);
    assert.equal(v.levelDataInUse, true, `${shard} 应识别为 leveldataoverride`);
    assert.ok(v.groups.length > 0, `${shard} 应有选项`);
    if (location === 'shipwrecked' || location === 'volcanoworld') {
      assert.equal(v.mod, 'island-adventures', `${shard} 应标记岛屿冒险`);
    }
  }
});

test('海难与火山返回各自的模组选项与预设', async () => {
  const rooms = await api('/api/rooms');
  const room = rooms.find(r => r.key === 'Cluster_2');
  const shipwrecked = await api(`/api/worldconfig?room=${room.id}&shard=Master`);
  const names = new Set(shipwrecked.groups.flatMap(g => g.options.map(o => o.name)));
  for (const n of ['bamboo', 'dragoonegg', 'ia_boats', 'ia_drowning', 'poison', 'grass', 'world_size']) {
    assert.ok(names.has(n), `海难应包含 ${n}`);
  }
  assert.ok(!names.has('deerclops'), '海难不应原样套用全部森林选项');
  assert.ok(shipwrecked.presets.worldgen.some(p => p.data === 'SHIPWRECKED_ENDLESS'));

  const volcano = await api(`/api/worldconfig?room=${room.id}&shard=Caves`);
  const vnames = new Set(volcano.groups.flatMap(g => g.options.map(o => o.name)));
  for (const n of ['dragoonegg', 'coffeebush', 'magma_rocks']) {
    assert.ok(vnames.has(n), `火山应包含 ${n}`);
  }
  assert.ok(!vnames.has('bamboo'), '火山不应包含只在海难出现的项');
  assert.ok(volcano.presets.worldgen.some(p => p.data === 'SURVIVAL_VOLCANO_CLASSIC'));
});

test('保存只改世界规则，其他内容逐字保留', async () => {
  const rooms = await api('/api/rooms');
  const room = rooms.find(r => r.key === 'Cluster_2');
  const file = join(saveDir, 'DST', 'Cluster_2', 'Caves', 'leveldataoverride.lua');
  const before = readFileSync(file, 'utf8');

  const view = await api(`/api/worldconfig?room=${room.id}&shard=Caves`);
  const settingsGroup = view.groups.find(g => g.category === 'settings');
  const option = settingsGroup.options.find(o => o.name === 'dragoonegg');
  const next = option.default === 'never' ? 'often' : 'never';

  await api('/api/worldconfig', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      room: room.id, shard: 'Caves', overrideEnabled: true,
      worldgenPreset: '', settingsPreset: '', overrides: { dragoonegg: next },
    }),
  });

  const after = readFileSync(file, 'utf8');
  assert.ok(after.includes(`dragoonegg="${next}"`), '世界规则应被写入');
  // 其余逐字保留：只允许这一行变化
  const beforeLines = before.split('\n');
  const afterLines = after.split('\n');
  assert.equal(afterLines.length, beforeLines.length, '行数不应变化');
  const changed = beforeLines.filter((l, i) => l !== afterLines[i]);
  assert.equal(changed.length, 1, `只应改动一行，实际改动 ${changed.length} 行`);
  for (const key of ['required_prefabs', 'substitutes', 'version=', 'has_ocean', 'location=']) {
    assert.ok(after.includes(key), `应保留 ${key}`);
  }
});

test('世界生成类设置拒绝写入', async () => {
  const rooms = await api('/api/rooms');
  const room = rooms.find(r => r.key === 'Cluster_2');
  const file = join(saveDir, 'DST', 'Cluster_2', 'Master', 'leveldataoverride.lua');
  const before = readFileSync(file, 'utf8');
  const r = await fetch(`http://127.0.0.1:${PORT}/api/worldconfig`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      room: room.id, shard: 'Master', overrideEnabled: true,
      worldgenPreset: '', settingsPreset: '', overrides: { bamboo: 'often' },
    }),
  });
  const json = await r.json();
  assert.notEqual(json.code, 200, '世界生成类应被拒绝');
  assert.match(json.message, /世界生成/);
  assert.equal(readFileSync(file, 'utf8'), before, '拒绝时不得改动文件');
});
