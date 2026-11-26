// 带完整模拟数据的前端 UI 预览脚本（零 Go / DST 服务端依赖，支持 Vite 热更新）。
//
// 用法：
//   pnpm --dir web run dev:mock
//   pnpm --dir web run dev:mock -- --port 5174 --delay 400 --open
//   pnpm --dir web run dev:mock -- --auth   # 预览登录页（默认账号 admin / admin）

import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { createLocalGameAssets, encodePng } from './mock-game-assets.mjs';

/* ---------- CLI 参数解析 ---------- */

function parseArgs(argv) {
  const opts = { port: 5174, host: '127.0.0.1', delay: 0, auth: false, open: false, backend: '', gameDir: '', help: false };
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === '-h' || arg === '--help') opts.help = true;
    else if (arg === '--auth') opts.auth = true;
    else if (arg === '--open') opts.open = true;
    else if (arg === '--port' && argv[i + 1]) opts.port = Number(argv[++i]) || 5174;
    else if (arg.startsWith('--port=')) opts.port = Number(arg.slice('--port='.length)) || 5174;
    else if (arg === '--host' && argv[i + 1]) opts.host = argv[++i];
    else if (arg.startsWith('--host=')) opts.host = arg.slice('--host='.length);
    else if (arg === '--delay' && argv[i + 1]) opts.delay = Math.max(0, Number(argv[++i]) || 0);
    else if (arg.startsWith('--delay=')) opts.delay = Math.max(0, Number(arg.slice('--delay='.length)) || 0);
    else if (arg === '--backend' && argv[i + 1]) opts.backend = argv[++i].replace(/\/+$/, '');
    else if (arg.startsWith('--backend=')) opts.backend = arg.slice('--backend='.length).replace(/\/+$/, '');
    else if (arg === '--game-dir' && argv[i + 1]) opts.gameDir = argv[++i];
    else if (arg.startsWith('--game-dir=')) opts.gameDir = arg.slice('--game-dir='.length);
  }
  return opts;
}

const cli = parseArgs(process.argv.slice(2));
if (cli.help) {
  console.log(`用法：pnpm --dir web run dev:mock -- [选项]

选项：
  --port <端口>      监听端口（默认 5174）
  --host <地址>      监听地址（默认 127.0.0.1）
  --delay <毫秒>     模拟接口延迟（默认 0；如设 600 可直观检查各页骨架屏）
  --game-dir <目录>  指定本地 DST 安装目录以直接解码真实游戏小地图贴图与图标（默认自动探测 Steam 库）
  --backend <地址>   可选，透传 /api/map/texture 与 /api/map/icon 到本地面板（如 http://127.0.0.1:16800）
  --auth             开启登录态预览（默认用户名 admin，密码 admin）
  --open             启动后自动打开浏览器
  -h, --help         显示帮助
`);
  process.exit(0);
}

const localGameAssets = createLocalGameAssets(cli.gameDir);

/* ---------- 工具函数 ---------- */

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const nowIso = (offsetMinutes = 0) => new Date(Date.now() - offsetMinutes * 60_000).toISOString();
const clone = value => JSON.parse(JSON.stringify(value));

function sendJson(res, data, code = 200, message = 'ok') {
  res.statusCode = code === 401 ? 401 : 200;
  res.setHeader('Content-Type', 'application/json; charset=utf-8');
  res.setHeader('Cache-Control', 'no-store');
  res.end(JSON.stringify({ code, message, data }));
}

function readJsonBody(req) {
  return new Promise(resolve => {
    const chunks = [];
    req.on('data', chunk => chunks.push(chunk));
    req.on('end', () => {
      const text = Buffer.concat(chunks).toString('utf8').trim();
      if (!text) { resolve({}); return; }
      try { resolve(JSON.parse(text)); }
      catch { resolve({}); }
    });
    req.on('error', () => resolve({}));
  });
}

/* ---------- 程序羊皮纸兜底（仅在本机无 DST 游戏文件时使用） ---------- */

function clampByte(v) {
  return v < 0 ? 0 : v > 255 ? 255 : v | 0;
}

function texHash(x, y, seed) {
  let h = Math.imul((x & 63) + seed * 31, 374761393) + Math.imul((y & 63) + seed * 17, 668265263);
  h = Math.imul(h ^ (h >>> 13), 1274126177);
  return ((h ^ (h >>> 16)) >>> 0) / 4294967295;
}

const MOCK_TILE_BASE = {
  GRASS: [132, 166, 106],
  FOREST: [86, 128, 92],
  SAVANNA: [204, 188, 114],
  DECIDUOUS: [174, 134, 86],
  ROCKY: [170, 166, 152],
  DESERT_DIRT: [218, 194, 140],
  MARSH: [116, 100, 126],
  ROAD: [182, 162, 140],
  CAVE: [104, 114, 126],
  FUNGUS: [96, 146, 178],
  FUNGUSRED: [176, 102, 98],
  FUNGUSGREEN: [104, 156, 110],
  MUD: [136, 114, 92],
  UNDERROCK: [118, 118, 128],
  BRICK: [128, 120, 152],
  SINKHOLE: [116, 102, 88],
  BEACH: [224, 204, 152],
  JUNGLE: [88, 134, 96],
  MEADOW: [152, 178, 104],
  MAGMAFIELD: [134, 100, 92],
  TIDALMARSH: [124, 138, 106],
  VOLCANO: [126, 98, 88],
  VOLCANO_ROCK: [118, 110, 108],
  VOLCANO_LAVA: [232, 126, 68],
  ASH: [148, 138, 132],
};

const texturePngCache = new Map();

function getMockTexturePng(key) {
  if (texturePngCache.has(key)) return texturePngCache.get(key);
  const size = 64;
  const rgba = Buffer.alloc(size * size * 4);
  const seed = [...key].reduce((acc, ch) => acc * 31 + ch.charCodeAt(0), 7) & 0xffff;

  if (key === '__MAP_EDGE__') {
    for (let i = 0; i < size * size; i++) {
      rgba[i * 4] = 225;
      rgba[i * 4 + 1] = 188;
      rgba[i * 4 + 2] = 115;
      rgba[i * 4 + 3] = 255;
    }
  } else if (key === '__PAPER__') {
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        const fiber = Math.sin((x / size) * Math.PI * 4) * 6 + Math.cos((y / size) * Math.PI * 6) * 5;
        const grain = (texHash(x, y, 11) - 0.5) * 26 + (texHash(x >> 1, y >> 1, 19) - 0.5) * 18;
        const luma = clampByte(192 + fiber + grain);
        const o = (y * size + x) * 4;
        rgba[o] = luma; rgba[o + 1] = luma; rgba[o + 2] = luma; rgba[o + 3] = 255;
      }
    }
  } else if (key === '__OCEAN_NOISE__') {
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        const wave = Math.sin((x / size) * Math.PI * 4 + Math.sin((y / size) * Math.PI * 2) * 1.6) * 24;
        const ripple = Math.cos((y / size) * Math.PI * 8 + (x / size) * Math.PI * 2) * 14;
        const grain = (texHash(x, y, 29) - 0.5) * 18;
        const luma = clampByte(178 + wave + ripple + grain);
        const o = (y * size + x) * 4;
        rgba[o] = luma; rgba[o + 1] = luma; rgba[o + 2] = luma; rgba[o + 3] = 255;
      }
    }
  } else {
    const base = MOCK_TILE_BASE[key] || [140, 152, 128];
    const tintR = 225 / 255;
    const tintG = 188 / 255;
    const tintB = 115 / 255;
    // 反算羊皮纸着色，使 renderGameTextured 乘以 map_edge 后还原出自然温润的手绘群系色
    const preR = base[0] / tintR;
    const preG = base[1] / tintG;
    const preB = base[2] / tintB;
    for (let y = 0; y < size; y++) {
      for (let x = 0; x < size; x++) {
        const patch = Math.sin((x / size) * Math.PI * 2 + seed) * Math.cos((y / size) * Math.PI * 2) * 0.08;
        const stroke = (texHash(x, y, seed) - 0.5) * 0.14 + (texHash(x >> 2, y >> 2, seed + 5) - 0.5) * 0.12;
        const speck = texHash(x, y, seed + 97) > 0.94 ? -0.14 : 0;
        const mod = 1 + patch + stroke + speck;
        const o = (y * size + x) * 4;
        rgba[o] = clampByte(preR * mod);
        rgba[o + 1] = clampByte(preG * mod);
        rgba[o + 2] = clampByte(preB * mod);
        rgba[o + 3] = 255;
      }
    }
  }

  const png = encodePng(size, size, rgba);
  texturePngCache.set(key, png);
  return png;
}

const LANDMARK_GLYPH = {
  multiplayer_portal: { bg: '#2c6e63', fg: '#f4e8c8', path: 'M10 24V11a6 6 0 0 1 12 0v13M13 24V13a3 3 0 0 1 6 0v11M8 24h16' },
  pigking: { bg: '#9b6b1e', fg: '#fff1c4', path: 'M8 21l2-9 6 4 6-4 2 9zM10 24h12' },
  wormhole: { bg: '#6c4287', fg: '#f3d9ff', path: 'M16 16m-7 0a7 7 0 1 0 14 0a7 7 0 1 0 -14 0M16 16m-3.5 0a3.5 3.5 0 1 0 7 0a3.5 3.5 0 1 0 -7 0' },
  cave_entrance: { bg: '#5a4637', fg: '#f0dfbe', path: 'M7 23l9-13 9 13zM13 23v-5a3 3 0 0 1 6 0v5' },
  cave_exit: { bg: '#5a4637', fg: '#f0dfbe', path: 'M7 23l9-13 9 13zM16 14v9' },
  resurrectionstone: { bg: '#675c52', fg: '#f7efd8', path: 'M16 7l7 9-7 9-7-9zM16 12v8' },
  moonbase: { bg: '#3f5a74', fg: '#e3f1ff', path: 'M19 9a7 7 0 1 0 4 12a6 6 0 1 1-4-12z' },
  oasislake: { bg: '#2f7a85', fg: '#d9fbff', path: 'M16 8c4 5 6 8 6 11a6 6 0 0 1-12 0c0-3 2-6 6-11z' },
  beequeenhivegrown: { bg: '#b07b1c', fg: '#fff4c8', path: 'M16 8l6 4v8l-6 4-6-4v-8zM13 15h6M14 19h4' },
  hermithouse: { bg: '#456a75', fg: '#f2e8cf', path: 'M8 16l8-7 8 7M11 16v8h10v-8' },
  terrarium: { bg: '#3d7448', fg: '#e8ffdf', path: 'M9 12h14v12H9zM16 12v12M12 17h8' },
  ancient_altar: { bg: '#6b4f8c', fg: '#ffe9a8', path: 'M9 22h14M11 18h10l-2-8h-6zM16 7v3' },
  atrium_gate: { bg: '#4b556b', fg: '#f2d9b1', path: 'M10 24V12a6 6 0 0 1 12 0v12M16 15v5' },
  archive_switch: { bg: '#6e6446', fg: '#f7ecc5', path: 'M11 10h10v12H11zM16 13v6M13 16h6' },
  volcano: { bg: '#9e3b24', fg: '#ffd79a', path: 'M6 23l7-12h6l7 12zM13 11l3-4 3 4' },
  volcano_exit: { bg: '#9e3b24', fg: '#ffd79a', path: 'M6 23l7-12h6l7 12zM16 14v7' },
  volcano_altar: { bg: '#8c3525', fg: '#ffe0a3', path: 'M10 22h12M12 18l4-9 4 9z' },
  obsidian_workbench: { bg: '#4b3b54', fg: '#f4c88c', path: 'M8 14h16v4H8zM11 18v5M21 18v5' },
  seaworthy: { bg: '#26687a', fg: '#e5f8ff', path: 'M10 24V11a6 6 0 0 1 12 0v13M8 20c3-2 5 2 8 0s5 2 8 0' },
  octopusking: { bg: '#9e532b', fg: '#ffebc2', path: 'M11 15a5 5 0 0 1 10 0v3H11zM9 22l2-4M14 23v-5M18 23v-5M23 22l-2-4' },
  slotmachine: { bg: '#8f6922', fg: '#fff1c1', path: 'M10 9h12v14H10zM13 14h6M13 18h6' },
  coral_brain_rock: { bg: '#3e7a6d', fg: '#e6fff5', path: 'M10 21a6 6 0 0 1 12 0zM13 16a3 3 0 0 1 6 0' },
};

function getMockIconSvg(kind) {
  const g = LANDMARK_GLYPH[kind] || { bg: '#6e5c47', fg: '#f6ebd0', path: 'M16 8l2.5 5.5L24 16l-5.5 2.5L16 24l-2.5-5.5L8 16l5.5-2.5z' };
  return `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
  <circle cx="16" cy="16" r="14" fill="#231c15" fill-opacity="0.72"/>
  <circle cx="16" cy="16" r="12.5" fill="${g.bg}" stroke="#e7cfa0" stroke-width="1.8"/>
  <path d="${g.path}" fill="none" stroke="${g.fg}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;
}

async function tryProxyBinary(backendBase, reqUrl, res) {
  if (!backendBase) return false;
  try {
    const upstream = await fetch(`${backendBase}${reqUrl}`, { signal: globalThis.AbortSignal.timeout(4000) });
    if (!upstream.ok) return false;
    const ct = upstream.headers.get('content-type') || '';
    if (!ct.toLowerCase().startsWith('image/')) return false;
    const buf = Buffer.from(await upstream.arrayBuffer());
    res.statusCode = 200;
    res.setHeader('Content-Type', ct);
    res.setHeader('Cache-Control', 'public, max-age=300');
    res.end(buf);
    return true;
  } catch {
    return false;
  }
}

/* ---------- 模拟世界地图生成 ---------- */

function buildMockMap(shardName, location) {
  const width = 128;
  const height = 128;
  const isCave = location === 'cave';
  const isShip = location === 'shipwrecked';
  const isVolcano = location === 'volcanoworld';

  const specs = isVolcano
    ? [
        { id: 1, name: 'IMPASSABLE' },
        { id: 2, name: 'VOLCANO' },
        { id: 3, name: 'VOLCANO_ROCK' },
        { id: 4, name: 'VOLCANO_LAVA' },
        { id: 5, name: 'ASH' },
      ]
    : isShip
      ? [
          { id: 1, name: 'OCEAN_DEEP' },
          { id: 2, name: 'OCEAN_MEDIUM' },
          { id: 3, name: 'OCEAN_SHALLOW' },
          { id: 4, name: 'BEACH' },
          { id: 5, name: 'JUNGLE' },
          { id: 6, name: 'MEADOW' },
          { id: 7, name: 'MAGMAFIELD' },
          { id: 8, name: 'TIDALMARSH' },
        ]
      : isCave
        ? [
            { id: 1, name: 'IMPASSABLE' },
            { id: 2, name: 'CAVE' },
            { id: 3, name: 'FUNGUS' },
            { id: 4, name: 'FUNGUSRED' },
            { id: 5, name: 'FUNGUSGREEN' },
            { id: 6, name: 'MUD' },
            { id: 7, name: 'UNDERROCK' },
            { id: 8, name: 'BRICK' },
            { id: 9, name: 'SINKHOLE' },
          ]
        : [
            { id: 1, name: 'OCEAN_ROUGH' },
            { id: 2, name: 'OCEAN_SWELL' },
            { id: 3, name: 'OCEAN_COASTAL' },
            { id: 4, name: 'OCEAN_COASTAL_SHORE' },
            { id: 5, name: 'GRASS' },
            { id: 6, name: 'FOREST' },
            { id: 7, name: 'SAVANNA' },
            { id: 8, name: 'DECIDUOUS' },
            { id: 9, name: 'ROCKY' },
            { id: 10, name: 'DESERT_DIRT' },
            { id: 11, name: 'MARSH' },
            { id: 12, name: 'ROAD' },
          ];

  const buf = Buffer.alloc(width * height * 2);
  const cx = width / 2;
  const cy = height / 2;

  for (let y = 0; y < height; y++) {
    for (let x = 0; x < width; x++) {
      const dx = (x - cx) / (width * 0.44);
      const dy = (y - cy) / (height * 0.44);
      const ang = Math.atan2(dy, dx);
      const ripple = 0.14 * Math.sin(ang * 3 + 0.7) + 0.09 * Math.cos(ang * 5 - 0.4) + 0.05 * Math.sin((x + y) * 0.14);
      const dist = Math.hypot(dx, dy) + ripple;

      let id;
      if (isVolcano) {
        if (dist > 0.88) id = 1;
        else if (dist > 0.72) id = 3;
        else if (dist < 0.24) id = 4;
        else if (Math.sin(x * 0.22) * Math.cos(y * 0.22) > 0.35) id = 5;
        else id = 2;
      } else if (isShip) {
        if (dist > 1.05) id = 1;
        else if (dist > 0.85) id = 2;
        else if (dist > 0.66) id = 3;
        else if (dist > 0.56) id = 4;
        else if (dx < -0.15 && dy < 0.1) id = 5;
        else if (dx > 0.15 && dy < -0.1) id = 7;
        else if (dy > 0.18) id = 8;
        else id = 6;
      } else if (isCave) {
        if (dist > 0.92) id = 1;
        else if (dist < 0.18) id = 9;
        else if (dx > 0.28 && dy > 0.22) id = 8;
        else if (dx < -0.25 && dy < 0) id = 3;
        else if (dx < -0.1 && dy > 0.2) id = 4;
        else if (dx > 0.18 && dy < -0.18) id = 5;
        else if (Math.sin(x * 0.16 + y * 0.12) > 0.45) id = 6;
        else if (Math.cos(x * 0.18 - y * 0.15) > 0.5) id = 7;
        else id = 2;
      } else {
        if (dist > 1.1) id = 1;
        else if (dist > 0.94) id = 2;
        else if (dist > 0.8) id = 3;
        else if (dist > 0.73) id = 4;
        else if (Math.abs((x - cx) * 0.6 - (y - cy)) < 1.3 && dist < 0.68) id = 12;
        else if (dx < -0.22 && dy < -0.12) id = 6;
        else if (dx > 0.22 && dy < -0.14) id = 8;
        else if (dx > 0.24 && dy > 0.18) id = 10;
        else if (dx < -0.24 && dy > 0.2) id = 11;
        else if (Math.hypot(dx + 0.05, dy - 0.34) < 0.22) id = 9;
        else if (Math.hypot(dx - 0.05, dy + 0.3) < 0.2) id = 7;
        else id = 5;
      }
      buf.writeUInt16LE(id, (y * width + x) * 2);
    }
  }

  const landmarks = isVolcano
    ? [
        { id: 'v1', kind: 'volcano_exit', name: '火山出口', x: 64, y: 78 },
        { id: 'v2', kind: 'volcano_altar', name: '火山祭坛', x: 64, y: 52 },
        { id: 'v3', kind: 'obsidian_workbench', name: '黑曜石工作台', x: 48, y: 60 },
      ]
    : isShip
      ? [
          { id: 's1', kind: 'seaworthy', name: '海洋之椅', x: 64, y: 64 },
          { id: 's2', kind: 'octopusking', name: '章鱼王', x: 34, y: 40 },
          { id: 's3', kind: 'slotmachine', name: '老虎机', x: 74, y: 58 },
          { id: 's4', kind: 'volcano', name: '火山', x: 96, y: 34 },
          { id: 's5', kind: 'coral_brain_rock', name: '智慧树', x: 86, y: 84 },
          { id: 'bt1', kind: 'bermudatriangle', name: '电光三角 1', x: 42, y: 56, target: 'bt2', pair: 1 },
          { id: 'bt2', kind: 'bermudatriangle', name: '电光三角 1', x: 90, y: 62, target: 'bt1', pair: 1 },
          { id: 'bt3', kind: 'bermudatriangle', name: '电光三角 2', x: 52, y: 30, target: 'bt4', pair: 2 },
          { id: 'bt4', kind: 'bermudatriangle', name: '电光三角 2', x: 62, y: 94, target: 'bt3', pair: 2 },
          { id: 'bt5', kind: 'bermudatriangle', name: '电光三角 3', x: 26, y: 74, target: 'bt6', pair: 3 },
          { id: 'bt6', kind: 'bermudatriangle', name: '电光三角 3', x: 82, y: 42, target: 'bt5', pair: 3 },
        ]
      : isCave
        ? [
            { id: 'c1', kind: 'cave_exit', name: '楼梯', x: 64, y: 64 },
            { id: 'c2', kind: 'ancient_altar', name: '远古伪科学站', x: 86, y: 82 },
            { id: 'c3', kind: 'atrium_gate', name: '远古大门', x: 94, y: 74 },
            { id: 'c4', kind: 'archive_switch', name: '华丽基座', x: 40, y: 46 },
          ]
        : [
            { id: 'm1', kind: 'multiplayer_portal', name: '绚丽之门', x: 64, y: 64 },
            { id: 'm2', kind: 'pigking', name: '猪王', x: 82, y: 48 },
            { id: 'm3', kind: 'wormhole', name: '虫洞 1', x: 48, y: 52, target: 'm4', pair: 1 },
            { id: 'm4', kind: 'wormhole', name: '虫洞 1', x: 84, y: 78, target: 'm3', pair: 1 },
            { id: 'm12', kind: 'wormhole', name: '虫洞 2', x: 40, y: 74, target: 'm13', pair: 2 },
            { id: 'm13', kind: 'wormhole', name: '虫洞 2', x: 76, y: 38, target: 'm12', pair: 2 },
            { id: 'm14', kind: 'wormhole', name: '虫洞 3', x: 54, y: 36, target: 'm15', pair: 3 },
            { id: 'm15', kind: 'wormhole', name: '虫洞 3', x: 66, y: 88, target: 'm14', pair: 3 },
            { id: 'm5', kind: 'cave_entrance', name: '被堵住的洞穴', x: 58, y: 72 },
            { id: 'm6', kind: 'moonbase', name: '月亮石', x: 44, y: 42 },
            { id: 'm7', kind: 'oasislake', name: '湖泊', x: 88, y: 74 },
            { id: 'm8', kind: 'beequeenhivegrown', name: '巨大蜂窝', x: 52, y: 82 },
            { id: 'm9', kind: 'resurrectionstone', name: '试金石', x: 70, y: 42 },
            { id: 'm10', kind: 'hermithouse', name: '隐士之家', x: 24, y: 86 },
            { id: 'm11', kind: 'terrarium', name: '盒中泰拉', x: 46, y: 66 },
          ];

  const textures = [
    '__PAPER__',
    '__OCEAN_NOISE__',
    '__MAP_EDGE__',
    ...specs.map(s => s.name).filter(name => Object.hasOwn(MOCK_TILE_BASE, name)),
  ];
  const icons = [...new Set(landmarks.map(point => point.kind))];

  return {
    width,
    height,
    savedAt: nowIso(4),
    snapshot: '0000000118',
    kind: location || 'forest',
    shard: shardName,
    regions: [],
    landmarks,
    terrain: { tiles: buf.toString('base64'), specs },
    textures,
    icons,
  };
}

/* ---------- 模拟世界配置（含真实 DST 选项名，自动命中 /world-icons/*.png） ---------- */

const STANDARD_VALUES = [
  { data: 'never', label: '无' },
  { data: 'rare', label: '较少' },
  { data: 'default', label: '默认' },
  { data: 'often', label: '较多' },
  { data: 'always', label: '大量' },
];
const SEASON_VALUES = [
  { data: 'noseason', label: '无' },
  { data: 'veryshortseason', label: '极短' },
  { data: 'shortseason', label: '短' },
  { data: 'default', label: '默认' },
  { data: 'longseason', label: '长' },
  { data: 'verylongseason', label: '极长' },
  { data: 'random', label: '随机' },
];
const ENABLE_VALUES = [
  { data: 'none', label: '禁用' },
  { data: 'default', label: '启用' },
];

function buildWorldGroups(location) {
  if (location === 'cave') {
    return [
      {
        id: 'global',
        label: '全局',
        category: 'settings',
        options: [
          { name: 'weather', label: '洞穴酸雨与天气', default: 'default', values: STANDARD_VALUES },
          { name: 'earthquakes', label: '地震', default: 'default', values: STANDARD_VALUES },
          { name: 'wormattacks', label: '洞穴蠕虫攻击', default: 'default', values: STANDARD_VALUES },
          { name: 'atriumgate', label: '远古大门再生', default: 'default', values: STANDARD_VALUES },
        ],
      },
      {
        id: 'monsters',
        label: '巨兽',
        category: 'settings',
        options: [
          { name: 'toadstool', label: '毒菌蟾蜍', default: 'default', values: STANDARD_VALUES },
          { name: 'daywalker', label: '噩梦疯猪', default: 'default', values: STANDARD_VALUES },
        ],
      },
      {
        id: 'worldgen_global',
        label: '世界',
        category: 'worldgen',
        options: [
          { name: 'world_size', label: '世界大小', default: 'default', values: [{ data: 'small', label: '小' }, { data: 'medium', label: '中' }, { data: 'default', label: '大' }, { data: 'huge', label: '巨大' }] },
          { name: 'branching', label: '分支', default: 'default', values: [{ data: 'never', label: '从不' }, { data: 'least', label: '最少' }, { data: 'default', label: '默认' }, { data: 'most', label: '最多' }] },
          { name: 'loop', label: '环形', default: 'default', values: [{ data: 'never', label: '从不' }, { data: 'default', label: '默认' }, { data: 'always', label: '总是' }] },
        ],
      },
    ];
  }
  return [
    {
      id: 'global',
      label: '全局',
      category: 'settings',
      options: [
        { name: 'specialevent', label: '活动', default: 'default', values: [{ data: 'none', label: '无' }, { data: 'default', label: '自动' }, { data: 'winters_feast', label: '冬季盛宴' }, { data: 'hallowed_nights', label: '万圣夜' }] },
        { name: 'autumn', label: '秋', default: 'default', values: SEASON_VALUES },
        { name: 'winter', label: '冬', default: 'default', values: SEASON_VALUES },
        { name: 'spring', label: '春', default: 'default', values: SEASON_VALUES },
        { name: 'summer', label: '夏', default: 'default', values: SEASON_VALUES },
        { name: 'day', label: '昼夜选项', default: 'default', values: [{ data: 'default', label: '默认' }, { data: 'longday', label: '长白天' }, { data: 'longdusk', label: '长黄昏' }, { data: 'longnight', label: '长夜晚' }] },
        { name: 'spawnmode', label: '出生模式', default: 'fixed', values: [{ data: 'fixed', label: '绚丽之门' }, { data: 'scatter', label: '随机分布' }] },
        { name: 'portalresurection', label: '在绚丽之门复活', default: 'none', values: ENABLE_VALUES },
        { name: 'ghostsanitydrain', label: '鬼魂理智值惩罚', default: 'always', values: [{ data: 'none', label: '无' }, { data: 'always', label: '启用' }] },
        { name: 'resettime', label: '死亡重置倒计时', default: 'default', values: [{ data: 'none', label: '禁用' }, { data: 'slow', label: '慢' }, { data: 'default', label: '默认' }, { data: 'fast', label: '快' }] },
      ],
    },
    {
      id: 'world',
      label: '世界',
      category: 'settings',
      options: [
        { name: 'wildfires', label: '野火', default: 'default', values: STANDARD_VALUES },
        { name: 'weather', label: '雨', default: 'default', values: STANDARD_VALUES },
        { name: 'lightning', label: '闪电', default: 'default', values: STANDARD_VALUES },
        { name: 'frograin', label: '青蛙雨', default: 'default', values: STANDARD_VALUES },
        { name: 'regrowth', label: '世界再生', default: 'default', values: STANDARD_VALUES },
        { name: 'petrification', label: '森林石化', default: 'default', values: STANDARD_VALUES },
      ],
    },
    {
      id: 'creatures',
      label: '生物与巨兽',
      category: 'settings',
      options: [
        { name: 'beefaloheat', label: '皮弗娄牛交配频率', default: 'default', values: STANDARD_VALUES },
        { name: 'krampus', label: '坎普斯', default: 'default', values: STANDARD_VALUES },
        { name: 'deerclops', label: '独眼巨鹿', default: 'default', values: STANDARD_VALUES },
        { name: 'bearger', label: '熊獾', default: 'default', values: STANDARD_VALUES },
        { name: 'dragonfly', label: '龙蝇', default: 'default', values: STANDARD_VALUES },
        { name: 'goosemoose', label: '麋鹿鹅', default: 'default', values: STANDARD_VALUES },
        { name: 'antliontribute', label: '蚁狮贡品', default: 'default', values: STANDARD_VALUES },
      ],
    },
    {
      id: 'worldgen_global',
      label: '世界',
      category: 'worldgen',
      options: [
        { name: 'world_size', label: '世界大小', default: 'default', values: [{ data: 'small', label: '小' }, { data: 'medium', label: '中' }, { data: 'default', label: '大' }, { data: 'huge', label: '巨大' }] },
        { name: 'branching', label: '分支', default: 'default', values: [{ data: 'never', label: '从不' }, { data: 'least', label: '最少' }, { data: 'default', label: '默认' }, { data: 'most', label: '最多' }] },
        { name: 'loop', label: '环形', default: 'default', values: [{ data: 'never', label: '从不' }, { data: 'default', label: '默认' }, { data: 'always', label: '总是' }] },
        { name: 'roads', label: '道路', default: 'default', values: [{ data: 'never', label: '无' }, { data: 'default', label: '默认' }] },
        { name: 'touchstone', label: '试金石', default: 'default', values: STANDARD_VALUES },
        { name: 'boons', label: '前辈', default: 'default', values: STANDARD_VALUES },
      ],
    },
    {
      id: 'worldgen_resources',
      label: '资源',
      category: 'worldgen',
      options: [
        { name: 'berrybush', label: '浆果丛', default: 'default', values: STANDARD_VALUES },
        { name: 'grass', label: '草', default: 'default', values: STANDARD_VALUES },
        { name: 'sapling', label: '树苗', default: 'default', values: STANDARD_VALUES },
        { name: 'trees', label: '树', default: 'default', values: STANDARD_VALUES },
        { name: 'rock', label: '巨石', default: 'default', values: STANDARD_VALUES },
        { name: 'flint', label: '燧石', default: 'default', values: STANDARD_VALUES },
      ],
    },
  ];
}

/* ---------- 内存状态初始化 ---------- */

function createMockState(authEnabled) {
  const rooms = [
    {
      id: 1,
      key: 'Cluster_1',
      name: '饥荒联机生存服 · 主力档',
      description: '友好联机生存，欢迎萌新与老玩家一起建家打 Boss！',
      gameMode: 'survival',
      maxPlayers: 12,
      tickRate: 30,
      language: 'zh',
      password: 'dst',
      token: 'pds-g^KU_MOCK_TOKEN_DEMO_001',
      maxSnapshots: 15,
      masterIp: '127.0.0.1',
      masterPort: 10888,
      clusterKey: 'mock_cluster_secret_1',
      pvp: false,
      pauseEmpty: true,
      voteEnabled: true,
      voteKick: true,
      lanOnly: false,
      offline: false,
      hasSave: true,
      needToken: false,
      modCount: 6,
      connectIp: '',
      connectPort: 0,
      connectPorts: {},
      locations: { Master: 'forest', Caves: 'cave' },
      shards: [
        { name: 'Master', worldType: 'forest', isMaster: true, disabled: false, gameId: 1, serverPort: 10999, masterServerPort: 27018, authenticationPort: 8768 },
        { name: 'Caves', worldType: 'cave', isMaster: false, disabled: false, gameId: 2, serverPort: 11000, masterServerPort: 27019, authenticationPort: 8769 },
      ],
    },
    {
      id: 2,
      key: 'Cluster_2',
      name: '岛屿冒险 · 四世界穿梭',
      description: '森林 + 洞穴 + 海难 + 火山四世界联动（已停止，可直接测试世界配置与模组编辑）',
      gameMode: 'endless',
      maxPlayers: 8,
      tickRate: 30,
      language: 'zh',
      password: '',
      token: 'pds-g^KU_MOCK_TOKEN_DEMO_002',
      maxSnapshots: 10,
      masterIp: '127.0.0.1',
      masterPort: 10889,
      clusterKey: 'mock_cluster_secret_2',
      pvp: false,
      pauseEmpty: true,
      voteEnabled: true,
      voteKick: false,
      lanOnly: true,
      offline: false,
      hasSave: true,
      needToken: false,
      modCount: 4,
      connectIp: 'play.example.com',
      connectPort: 11002,
      connectPorts: { Caves: 11003, Island: 11006, Volcano: 11007 },
      locations: { Master: 'forest', Caves: 'cave', Island: 'shipwrecked', Volcano: 'volcanoworld' },
      shards: [
        { name: 'Master', worldType: 'forest', isMaster: true, disabled: false, gameId: 1, serverPort: 11002, masterServerPort: 27020, authenticationPort: 8770 },
        { name: 'Caves', worldType: 'cave', isMaster: false, disabled: false, gameId: 2, serverPort: 11003, masterServerPort: 27021, authenticationPort: 8771 },
        { name: 'Island', worldType: 'shipwrecked', isMaster: false, disabled: false, gameId: 3, serverPort: 11006, masterServerPort: 27022, authenticationPort: 8772 },
        { name: 'Volcano', worldType: 'volcanoworld', isMaster: false, disabled: false, gameId: 4, serverPort: 11007, masterServerPort: 27023, authenticationPort: 8773 },
      ],
    },
    {
      id: 3,
      key: 'Cluster_3',
      name: '新档测试服（缺令牌）',
      description: '尚未配置 Klei 令牌的新房间示例',
      gameMode: 'relaxed',
      maxPlayers: 6,
      tickRate: 30,
      language: 'zh',
      password: '',
      token: '',
      maxSnapshots: 10,
      masterIp: '127.0.0.1',
      masterPort: 10890,
      clusterKey: '',
      pvp: false,
      pauseEmpty: true,
      voteEnabled: true,
      voteKick: true,
      lanOnly: false,
      offline: false,
      hasSave: false,
      needToken: true,
      modCount: 0,
      connectIp: '',
      connectPort: 0,
      connectPorts: {},
      locations: { Master: 'forest', Caves: 'cave' },
      shards: [
        { name: 'Master', worldType: 'forest', isMaster: true, disabled: false, gameId: 1, serverPort: 11010, masterServerPort: 27024, authenticationPort: 8774 },
        { name: 'Caves', worldType: 'cave', isMaster: false, disabled: true, gameId: 2, serverPort: 11011, masterServerPort: 27025, authenticationPort: 8775 },
      ],
    },
  ];

  const statusesByRoom = {
    1: [
      { shard: 'Master', state: 'running', pid: 18420, port: 10999, cpuPct: 14.6, memBytes: 1_180_000_000, uptime: '3小时35分', version: '763928', startedAt: nowIso(215), playersReady: true },
      { shard: 'Caves', state: 'running', pid: 18492, port: 11000, cpuPct: 9.2, memBytes: 860_000_000, uptime: '3小时34分', version: '763928', startedAt: nowIso(214), playersReady: true },
    ],
    2: [
      { shard: 'Master', state: 'stopped', port: 11002 },
      { shard: 'Caves', state: 'stopped', port: 11003 },
      { shard: 'Island', state: 'stopped', port: 11006 },
      { shard: 'Volcano', state: 'stopped', port: 11007 },
    ],
    3: [
      { shard: 'Master', state: 'stopped', port: 11010 },
    ],
  };

  const worldStatesByRoom = {
    1: {
      Master: { state: { day: 118, season: 'spring', remainingDays: 8, savedAt: nowIso(4) } },
      Caves: { state: { day: 118, season: 'spring', remainingDays: 8, savedAt: nowIso(4) } },
    },
    2: {
      Master: { state: { day: 46, season: 'autumn', remainingDays: 12, savedAt: nowIso(680) } },
      Caves: { state: { day: 46, season: 'autumn', remainingDays: 12, savedAt: nowIso(680) } },
      Island: { state: { day: 46, season: 'mild', remainingDays: 9, savedAt: nowIso(680) } },
      Volcano: { state: null },
    },
    3: {},
  };

  const playersByRoom = {
    1: [
      { userid: 'KU_A1B2C3D4', name: '阿明', prefab: 'wilson', shard: 'Master', days: 86, inWorld: true, ghost: false, admin: true, adminSaved: true },
      { userid: 'KU_E5F6G7H8', name: '小铃铛', prefab: 'wendy', shard: 'Master', days: 54, inWorld: true, ghost: false, admin: false, adminSaved: false },
      { userid: 'KU_J9K0L1M2', name: '铁皮罐头', prefab: 'wx78', shard: 'Master', days: 41, inWorld: true, ghost: true, admin: false, adminSaved: false },
      { userid: 'KU_N3P4Q5R6', name: '大力士老王', prefab: 'wolfgang', shard: 'Caves', days: 32, inWorld: true, ghost: false, admin: false, adminSaved: true },
    ],
    2: [],
    3: [],
  };

  const seenPlayers = {
    KU_A1B2C3D4: { userid: 'KU_A1B2C3D4', name: '阿明', prefab: 'wilson', steamid: '76561198012345678', ip: '198.51.100.12', lastSeen: nowIso(1) },
    KU_E5F6G7H8: { userid: 'KU_E5F6G7H8', name: '小铃铛', prefab: 'wendy', steamid: '76561198087654321', ip: '198.51.100.24', lastSeen: nowIso(2) },
    KU_J9K0L1M2: { userid: 'KU_J9K0L1M2', name: '铁皮罐头', prefab: 'wx78', steamid: '76561198111222333', ip: '198.51.100.37', lastSeen: nowIso(3) },
    KU_N3P4Q5R6: { userid: 'KU_N3P4Q5R6', name: '大力士老王', prefab: 'wolfgang', steamid: '76561198444555666', ip: '198.51.100.45', lastSeen: nowIso(5) },
    KU_S7T8U9V0: { userid: 'KU_S7T8U9V0', name: '路人甲', prefab: 'willow', steamid: '76561198999000111', ip: '198.51.100.88', lastSeen: nowIso(140) },
  };

  const adminsByRoom = { 1: ['KU_A1B2C3D4', 'KU_N3P4Q5R6'], 2: ['KU_A1B2C3D4'], 3: [] };
  const listsByRoom = {
    1: { blocklist: ['KU_S7T8U9V0'], whitelist: ['KU_A1B2C3D4', 'KU_E5F6G7H8'] },
    2: { blocklist: [], whitelist: [] },
    3: { blocklist: [], whitelist: [] },
  };

  const modsByRoom = {
    1: [
      { id: 'workshop-378160973', name: 'Global Positions（全球定位）', enabled: true, installed: true, version: '1.7.4', author: 'rezecib', localTime: Math.floor(Date.now() / 1000) - 86400 * 12, options: { SHOWPLAYERSOPTIONS: 2, FIREOPTIONS: 2 } },
      { id: 'workshop-1207269058', name: 'Simple Health Bar DST（简易血条）', enabled: true, installed: true, version: '2.15', author: 'Dyzzy', localTime: Math.floor(Date.now() / 1000) - 86400 * 30, options: { hb_style: 'standard' } },
      { id: 'workshop-1595631294', name: 'Smart Minisign（智能小木牌）', enabled: true, installed: true, version: '1.1.8', author: 'Breeze', localTime: Math.floor(Date.now() / 1000) - 86400 * 4, options: {} },
      { id: 'workshop-2323630362', name: 'Show Me（中文信息展示）', enabled: true, installed: true, version: '0.73', author: 'Star', localTime: Math.floor(Date.now() / 1000) - 86400 * 18, options: { food_style: 0 } },
      { id: 'workshop-466732225', name: 'No Thermal Stone Durability（暖石无限耐久）', enabled: true, installed: true, version: '1.0.3', author: 'Mobbstar', localTime: Math.floor(Date.now() / 1000) - 86400 * 90, options: {} },
      { id: 'workshop-2798599672', name: 'Extra Equip Slots（五格装备栏）', enabled: true, installed: false, version: '', author: 'Community', localTime: 0, options: {} },
      { id: 'workshop-1467214795', name: 'Island Adventures - Shipwrecked', enabled: false, installed: true, version: '2.4.1', author: 'IA Team', localTime: Math.floor(Date.now() / 1000) - 86400 * 2, options: {} },
    ],
    2: [
      { id: 'workshop-1467214795', name: 'Island Adventures - Shipwrecked', enabled: true, installed: true, version: '2.4.1', author: 'IA Team', localTime: Math.floor(Date.now() / 1000) - 86400 * 2, options: {} },
      { id: 'workshop-378160973', name: 'Global Positions（全球定位）', enabled: true, installed: true, version: '1.7.4', author: 'rezecib', localTime: Math.floor(Date.now() / 1000) - 86400 * 12, options: { SHOWPLAYERSOPTIONS: 2 } },
      { id: 'workshop-1207269058', name: 'Simple Health Bar DST（简易血条）', enabled: true, installed: true, version: '2.15', author: 'Dyzzy', localTime: Math.floor(Date.now() / 1000) - 86400 * 30, options: { hb_style: 'standard' } },
      { id: 'workshop-1595631294', name: 'Smart Minisign（智能小木牌）', enabled: true, installed: true, version: '1.1.8', author: 'Breeze', localTime: Math.floor(Date.now() / 1000) - 86400 * 4, options: {} },
    ],
    3: [],
  };

  const modSchemas = {
    'workshop-378160973': [
      { name: 'SHOWPLAYERSOPTIONS', label: '玩家图标显示', default: 2, choices: [{ data: 1, description: '仅小地图' }, { data: 2, description: '小地图与屏幕边缘' }, { data: 3, description: '隐藏' }] },
      { name: 'FIREOPTIONS', label: '篝火信号显示', default: 2, choices: [{ data: 1, description: '始终显示' }, { data: 2, description: '仅木炭信号' }, { data: 3, description: '关闭' }] },
      { name: 'SHAREMINIMAPPROGRESS', label: '共享地图探索进度', default: true, type: 'boolean' },
    ],
    'workshop-1207269058': [
      { name: 'hb_style', label: '血条样式', default: 'standard', choices: [{ data: 'standard', description: '标准样式' }, { data: 'heart', description: '红心样式' }, { data: 'simple', description: '简洁数值' }] },
      { name: 'hb_dmg', label: '显示伤害飘字', default: true, type: 'boolean' },
    ],
  };

  const chatLogsByRoom = {
    1: [
      { id: 'log-1', recordedAt: nowIso(3), time: '03:28:12', kind: 'chat', userid: 'KU_A1B2C3D4', name: '阿明', prefab: 'wilson', steamid: '76561198012345678', ip: '198.51.100.12', message: '家里的灭火器记得加燃料，夏天快到了' },
      { id: 'log-2', recordedAt: nowIso(6), time: '03:25:04', kind: 'death', userid: 'KU_J9K0L1M2', name: '铁皮罐头', prefab: 'wx78', steamid: '76561198111222333', ip: '198.51.100.37', message: '死于 猎犬。他变成了可怕的鬼魂！' },
      { id: 'log-3', recordedAt: nowIso(9), time: '03:21:55', kind: 'chat', userid: 'KU_E5F6G7H8', name: '小铃铛', prefab: 'wendy', steamid: '76561198087654321', ip: '198.51.100.24', message: '我带着救赎之心过来了，别乱飘' },
      { id: 'log-4', recordedAt: nowIso(14), time: '03:16:30', kind: 'announcement', userid: '', name: '系统公告', prefab: '', steamid: '', ip: '', message: '服务器将在 10 分钟后执行例行自动备份，期间可能有短暂卡顿。' },
      { id: 'log-5', recordedAt: nowIso(22), time: '03:08:11', kind: 'join', userid: 'KU_N3P4Q5R6', name: '大力士老王', prefab: 'wolfgang', steamid: '76561198444555666', ip: '198.51.100.45', message: '加入了游戏' },
      { id: 'log-6', recordedAt: nowIso(35), time: '02:55:40', kind: 'resurrect', userid: 'KU_E5F6G7H8', name: '小铃铛', prefab: 'wendy', steamid: '76561198087654321', ip: '198.51.100.24', message: '通过 绚丽之门 复活了' },
      { id: 'log-7', recordedAt: nowIso(49), time: '02:41:18', kind: 'whisper', userid: 'KU_A1B2C3D4', name: '阿明', prefab: 'wilson', steamid: '76561198012345678', ip: '198.51.100.12', message: '洞穴远古区清理完了吗？' },
      { id: 'log-8', recordedAt: nowIso(72), time: '02:18:05', kind: 'leave', userid: 'KU_S7T8U9V0', name: '路人甲', prefab: 'willow', steamid: '76561198999000111', ip: '198.51.100.88', message: '离开了游戏' },
      { id: 'log-9', recordedAt: nowIso(95), time: '01:55:44', kind: 'join', userid: 'KU_A1B2C3D4', name: '阿明', prefab: 'wilson', steamid: '76561198012345678', ip: '198.51.100.12', message: '加入了游戏' },
      { id: 'log-10', recordedAt: nowIso(108), time: '01:42:09', kind: 'chat', userid: 'KU_N3P4Q5R6', name: '大力士老王', prefab: 'wolfgang', steamid: '76561198444555666', ip: '198.51.100.45', message: '今晚一起把蜂后和龙蝇打了！' },
      { id: 'log-11', recordedAt: nowIso(125), time: '01:25:33', kind: 'join', userid: 'KU_J9K0L1M2', name: '铁皮罐头', prefab: 'wx78', steamid: '76561198111222333', ip: '198.51.100.37', message: '加入了游戏' },
      { id: 'log-12', recordedAt: nowIso(140), time: '01:10:02', kind: 'join', userid: 'KU_S7T8U9V0', name: '路人甲', prefab: 'willow', steamid: '76561198999000111', ip: '198.51.100.88', message: '加入了游戏' },
    ],
    2: [
      { id: 'log-201', recordedAt: nowIso(680), time: '01:12:04', kind: 'chat', userid: 'KU_A1B2C3D4', name: '阿明', prefab: 'wilson', steamid: '76561198012345678', ip: '198.51.100.12', message: '准备下海难找章鱼王换金币' },
    ],
    3: [],
  };

  const backupsByRoom = {
    1: [
      { name: 'Cluster_1_20261126_200000_auto.zip', path: 'C:\\DSTServer\\backups\\Cluster_1\\Cluster_1_20261126_200000_auto.zip', size: 18_420_112, createdAt: nowIso(75) },
      { name: 'Cluster_1_20261126_140000_manual.zip', path: 'C:\\DSTServer\\backups\\Cluster_1\\Cluster_1_20261126_140000_manual.zip', size: 18_105_420, createdAt: nowIso(435) },
      { name: 'Cluster_1_20261126_080000_auto.zip', path: 'C:\\DSTServer\\backups\\Cluster_1\\Cluster_1_20261126_080000_auto.zip', size: 17_842_910, createdAt: nowIso(795) },
      { name: 'Cluster_1_20261125_200000_auto.zip', path: 'C:\\DSTServer\\backups\\Cluster_1\\Cluster_1_20261125_200000_auto.zip', size: 17_204_188, createdAt: nowIso(1515) },
    ],
    2: [
      { name: 'Cluster_2_20261125_183000_manual.zip', path: 'C:\\DSTServer\\backups\\Cluster_2\\Cluster_2_20261125_183000_manual.zip', size: 32_914_620, createdAt: nowIso(1600) },
    ],
    3: [],
  };

  const saves = [
    { cluster: 'Cluster_1', roomExists: true, roomName: '饥荒联机生存服 · 主力档', running: true, hasSave: true, shards: ['Master', 'Caves'], locations: { Master: 'forest', Caves: 'cave' }, size: 46_280_192, modTime: nowIso(4) },
    { cluster: 'Cluster_2', roomExists: true, roomName: '岛屿冒险 · 四世界穿梭', running: false, hasSave: true, shards: ['Master', 'Caves', 'Island', 'Volcano'], locations: { Master: 'forest', Caves: 'cave', Island: 'shipwrecked', Volcano: 'volcanoworld' }, size: 84_910_420, modTime: nowIso(680) },
    { cluster: 'Cluster_3', roomExists: true, roomName: '新档测试服（缺令牌）', running: false, hasSave: false, shards: ['Master'], locations: { Master: 'forest' }, size: 48_128, modTime: nowIso(1800) },
    { cluster: 'Cluster_Old_2025', roomExists: false, roomName: '', running: false, hasSave: true, shards: ['Master', 'Caves'], locations: { Master: 'forest', Caves: 'cave' }, size: 29_512_000, modTime: nowIso(43200) },
  ];

  const worldConfigs = {};
  const logHistory = {
    '1:Master': [
      '[00:00:01]: [System] Don\'t Starve Together Dedicated Server v763928 (64-bit)',
      '[00:00:02]: [Shard] Starting Master shard on UDP port 10999',
      '[00:00:04]: [Mods] Loaded 5 enabled workshop mods cleanly',
      '[00:00:06]: [Warning] Mod workshop-2798599672 is not installed locally, skipping',
      '[00:00:09]: [World] Loading snapshot 0000000118 (Day 118, spring, 8 days left)',
      '[00:00:12]: Sim paused (waiting for players)',
      '[01:55:44]: [Join Announcement] 阿明',
      '[01:55:44]: Sim unpaused',
      '[03:08:11]: [Join Announcement] 大力士老王',
      '[03:21:55]: [Say] (KU_E5F6G7H8) 小铃铛: 我带着救赎之心过来了，别乱飘',
      '[03:25:04]: [Death Announcement] 铁皮罐头 死于 猎犬。他变成了可怕的鬼魂！',
      '[03:28:12]: [Say] (KU_A1B2C3D4) 阿明: 家里的灭火器记得加燃料，夏天快到了',
      '[03:30:00]: Serializing world: session/MOCK_SESSION_01/0000000118',
    ],
    '1:Caves': [
      '[00:00:01]: [Shard] Starting Caves shard on UDP port 11000',
      '[00:00:03]: [Shard] Connected to Master shard at 127.0.0.1:10888',
      '[00:00:08]: [World] Caves world initialized (Day 118, spring)',
      '[03:10:02]: [Shard] Player 大力士老王 migrated from Master -> Caves',
    ],
  };

  const sseClients = new Set();

  return {
    nextRoomId: 4,
    auth: {
      enabled: authEnabled,
      authenticated: !authEnabled,
      username: 'admin',
      password: 'admin',
      envManaged: false,
    },
    rooms,
    statusesByRoom,
    worldStatesByRoom,
    playersByRoom,
    seenPlayers,
    adminsByRoom,
    listsByRoom,
    modsByRoom,
    modSchemas,
    chatLogsByRoom,
    backupsByRoom,
    saves,
    worldConfigs,
    logHistory,
    sseClients,
    profiles: [
      { id: 'prof-1', name: '标准双世界生存模板', createdAt: nowIso(2880), gameMode: 'survival', shards: 2, modCount: 5 },
      { id: 'prof-2', name: '海难四世界穿梭模板', createdAt: nowIso(7200), gameMode: 'endless', shards: 4, modCount: 4 },
    ],
    automation: {
      timezone: 'Asia/Shanghai',
      running: false,
      config: { backupHours: 6, keep: 12, restartAt: '04:00', updateMinutes: 30, autoUpdate: true },
      next: { backup: new Date(Date.now() + 2 * 3600_000).toISOString(), restart: new Date(Date.now() + 7 * 3600_000).toISOString(), update: new Date(Date.now() + 18 * 60_000).toISOString() },
      last: {
        backup: { at: nowIso(75), ok: true, summary: '已备份 2 个房间' },
        restart: { at: nowIso(1020), ok: true, summary: '每日维护重启完成' },
        update: { at: nowIso(12), ok: true, summary: '服务端已是最新版本 763928' },
      },
    },
    backupRemote: {
      enabled: true,
      kind: 'directory',
      directory: 'D:\\Backups\\DST',
      url: '',
      username: '',
      passwordSet: false,
      keep: 5,
      results: [
        { room: 1, name: 'Cluster_1_20261126_200000_auto.zip', state: 'synced', at: nowIso(74), error: '' },
        { room: 1, name: 'Cluster_1_20261126_140000_manual.zip', state: 'synced', at: nowIso(434), error: '' },
        { room: 1, name: 'Cluster_1_20261126_080000_auto.zip', state: 'failed', at: nowIso(794), error: '示例错误：模拟网络抖动，可点重试按钮测试交互' },
      ],
    },
    notifications: {
      enabled: true,
      url: 'https://hooks.example.com/services/dst-panel-mock',
      secretSet: true,
      events: ['crash_limit', 'maintenance_failed', 'backup_failed', 'started'],
      rooms: [1],
      lastTestAt: nowIso(180),
      lastError: '',
    },
    serverInfo: {
      installed: true,
      buildId: '763928',
      root: 'C:\\Steam\\steamapps\\common\\Don\'t Starve Together Dedicated Server',
      ugcDir: 'C:\\DSTServer\\ugc_mods',
      configPath: 'C:\\DSTServer\\panel.json',
      arch: '64',
      archPref: 'auto',
      pinned: false,
      locked: false,
      managed: false,
      source: 'Steam 库（模拟探测）',
      candidates: [
        { root: 'C:\\Steam\\steamapps\\common\\Don\'t Starve Together Dedicated Server', source: 'Steam 库', arch: '64', has64: true, has32: true },
        { root: 'C:\\DSTServer\\steamcmd\\dst_server', source: '面板托管目录', arch: '64', has64: true, has32: false },
      ],
    },
    steamApiKey: '',
    publicIp: '203.0.113.42',
    backupChecks: new Map(),
  };
}

function getWorldConfig(state, roomId, shardName) {
  const key = `${roomId}:${shardName}`;
  if (!state.worldConfigs[key]) {
    const room = state.rooms.find(r => r.id === roomId);
    const shard = room?.shards.find(s => s.name === shardName);
    const location = room?.locations?.[shardName] || shard?.worldType || 'forest';
    const isModWorld = location === 'shipwrecked' || location === 'volcanoworld';
    const preset = location === 'cave' ? 'DST_CAVE' : 'SURVIVAL_TOGETHER';
    state.worldConfigs[key] = {
      file: `C:\\DSTServer\\dst_save\\Cluster_${roomId}\\${shardName}\\worldgenoverride.lua`,
      location,
      mod: isModWorld ? 'island_adventures' : '',
      hasSave: !!room?.hasSave,
      levelDataInUse: false,
      presets: {
        worldgen: location === 'cave'
          ? [{ data: 'DST_CAVE', label: '洞穴' }, { data: 'DST_CAVE_PLUS', label: '洞穴增强' }]
          : [{ data: 'SURVIVAL_TOGETHER', label: '生存' }, { data: 'ENDLESS', label: '无尽' }, { data: 'RELAXED', label: '轻松' }],
        settings: location === 'cave'
          ? [{ data: 'DST_CAVE', label: '洞穴' }]
          : [{ data: 'SURVIVAL_TOGETHER', label: '生存' }, { data: 'ENDLESS', label: '无尽' }, { data: 'RELAXED', label: '轻松' }],
      },
      groups: buildWorldGroups(location),
      volcanoIslandOnly: [],
      config: {
        overrideEnabled: true,
        worldgenPreset: preset,
        settingsPreset: preset,
        overrides: location === 'cave' ? { earthquakes: 'rare' } : { wildfires: 'never', portalresurection: 'default' },
      },
    };
  }
  return state.worldConfigs[key];
}

function pushLog(state, roomId, shardName, message) {
  const key = `${roomId}:${shardName}`;
  if (!state.logHistory[key]) state.logHistory[key] = [];
  const stamp = new Date().toLocaleTimeString('zh-CN', { hour12: false });
  const line = `[${stamp}]: ${message}`;
  state.logHistory[key].push(line);
  if (state.logHistory[key].length > 500) state.logHistory[key].shift();
  for (const client of state.sseClients) {
    if (client.roomId === roomId && client.shard === shardName) {
      client.res.write(`data: ${JSON.stringify({ type: 'log', message: line })}\n\n`);
    }
  }
}

function roomWithSummary(state, room) {
  const statuses = state.statusesByRoom[room.id] || [];
  const running = statuses.some(s => s.state === 'running' || s.state === 'starting');
  const mods = state.modsByRoom[room.id] || [];
  return {
    ...room,
    running,
    statuses,
    modCount: mods.filter(m => m.enabled).length,
    needToken: !room.lanOnly && !room.offline && !room.token.trim(),
  };
}

/* ---------- Mock API 中间件插件 ---------- */

function createMockApiPlugin(state, delayMs) {
  const heartbeat = setInterval(() => {
    for (const room of state.rooms) {
      const list = state.statusesByRoom[room.id] || [];
      for (const st of list) {
        if (st.state === 'running') {
          st.cpuPct = Math.max(1.2, Math.min(48, Number(((st.cpuPct || 10) + (Math.random() - 0.5) * 3.2).toFixed(1))));
        }
      }
    }
  }, 3000);
  heartbeat.unref?.();

  return {
    name: 'dst-mock-api',
    configureServer(server) {
      server.httpServer?.on('close', () => clearInterval(heartbeat));

      server.middlewares.use(async (req, res, next) => {
        if (!req.url || !req.url.startsWith('/api/')) {
          next();
          return;
        }

        const parsed = new URL(req.url, 'http://localhost');
        const pathname = parsed.pathname;
        const params = parsed.searchParams;
        const method = (req.method || 'GET').toUpperCase();

        // SSE 实时日志流
        if (pathname === '/api/logs/stream' && method === 'GET') {
          const roomId = Number(params.get('room')) || 1;
          const shard = params.get('shard') || 'Master';
          res.writeHead(200, {
            'Content-Type': 'text/event-stream; charset=utf-8',
            'Cache-Control': 'no-cache',
            Connection: 'keep-alive',
          });
          const key = `${roomId}:${shard}`;
          const lines = state.logHistory[key] || [`[00:00:00]: [Mock] 已连接到房间 #${roomId} 分片 ${shard} 日志流`];
          for (const line of lines) {
            res.write(`data: ${JSON.stringify({ type: 'log', message: line })}\n\n`);
          }
          const client = { roomId, shard, res };
          state.sseClients.add(client);
          const ping = setInterval(() => res.write(': keepalive\n\n'), 15000);
          req.on('close', () => {
            clearInterval(ping);
            state.sseClients.delete(client);
          });
          return;
        }

        if (delayMs > 0) await sleep(delayMs);

        // 鉴权端点（始终放行）
        if (pathname === '/api/auth/status') {
          sendJson(res, {
            enabled: state.auth.enabled,
            authenticated: state.auth.authenticated,
            username: state.auth.username,
            envManaged: state.auth.envManaged,
          });
          return;
        }
        if (pathname === '/api/auth/login' && method === 'POST') {
          const body = await readJsonBody(req);
          if (body.username === state.auth.username && body.password === state.auth.password) {
            state.auth.authenticated = true;
            sendJson(res, { ok: true, username: state.auth.username });
          } else {
            sendJson(res, null, 400, '用户名或密码错误（Mock 默认 admin / admin）');
          }
          return;
        }
        if (pathname === '/api/auth/logout' && method === 'POST') {
          if (state.auth.enabled) state.auth.authenticated = false;
          sendJson(res, { ok: true });
          return;
        }
        if (state.auth.enabled && !state.auth.authenticated) {
          sendJson(res, null, 401, '请先登录面板');
          return;
        }
        if (pathname === '/api/auth/password' && method === 'PUT') {
          const body = await readJsonBody(req);
          if (state.auth.enabled && body.currentPassword !== state.auth.password) {
            sendJson(res, null, 400, '当前密码不正确');
            return;
          }
          state.auth.username = (body.username || 'admin').trim() || 'admin';
          if ((body.newPassword || '').trim()) {
            state.auth.enabled = true;
            state.auth.password = body.newPassword.trim();
            state.auth.authenticated = true;
          } else {
            state.auth.enabled = false;
            state.auth.authenticated = true;
          }
          sendJson(res, { enabled: state.auth.enabled, username: state.auth.username, authenticated: true });
          return;
        }

        // 房间
        if (pathname === '/api/rooms/meta') {
          sendJson(res, {
            panelVersion: 'v0.1.2-mock',
            gameModes: ['survival', 'endless', 'wilderness', 'relaxed', 'lightsout'],
            tickRates: [15, 30, 60],
            languages: ['zh', 'en'],
            defaults: {
              name: '我的饥荒服务器',
              description: '',
              gameMode: 'endless',
              maxPlayers: 12,
              tickRate: 30,
              language: 'zh',
              maxSnapshots: 10,
              masterIp: '127.0.0.1',
              shards: [
                { name: 'Master', worldType: 'forest', isMaster: true, disabled: false, gameId: 1 },
                { name: 'Caves', worldType: 'cave', isMaster: false, disabled: false, gameId: 2 },
              ],
            },
          });
          return;
        }
        if (pathname === '/api/rooms' && method === 'GET') {
          sendJson(res, state.rooms.map(r => roomWithSummary(state, r)));
          return;
        }
        if (pathname === '/api/rooms' && method === 'POST') {
          const body = await readJsonBody(req);
          const id = state.nextRoomId++;
          const shards = (body.shards || [{ name: 'Master', worldType: 'forest', isMaster: true }]).map((s, idx) => ({
            ...s,
            gameId: s.gameId || idx + 1,
            serverPort: s.serverPort || 11020 + id * 10 + idx,
            masterServerPort: s.masterServerPort || 27030 + id * 10 + idx,
            authenticationPort: s.authenticationPort || 8780 + id * 10 + idx,
          }));
          const locations = Object.fromEntries(shards.map(s => [s.name, s.worldType || 'forest']));
          const room = {
            ...body,
            id,
            key: `Cluster_${id}`,
            hasSave: false,
            connectIp: '',
            connectPort: 0,
            connectPorts: {},
            locations,
            shards,
          };
          state.rooms.push(room);
          state.statusesByRoom[id] = shards.filter(s => !s.disabled).map(s => ({ shard: s.name, state: 'stopped', port: s.serverPort }));
          state.modsByRoom[id] = [];
          state.backupsByRoom[id] = [];
          state.chatLogsByRoom[id] = [];
          state.adminsByRoom[id] = [];
          state.listsByRoom[id] = { blocklist: [], whitelist: [] };
          sendJson(res, roomWithSummary(state, room));
          return;
        }
        if (pathname === '/api/rooms' && method === 'PUT') {
          const body = await readJsonBody(req);
          const idx = state.rooms.findIndex(r => r.id === Number(body.id));
          if (idx < 0) { sendJson(res, null, 404, '房间不存在'); return; }
          const prev = state.rooms[idx];
          const shards = (body.shards || prev.shards).map((s, i) => ({
            ...s,
            gameId: s.gameId || i + 1,
            serverPort: s.serverPort || 11020 + prev.id * 10 + i,
          }));
          const locations = Object.fromEntries(shards.map(s => [s.name, prev.locations?.[s.name] || s.worldType || 'forest']));
          state.rooms[idx] = { ...prev, ...body, id: prev.id, key: prev.key, shards, locations };
          const running = (state.statusesByRoom[prev.id] || []).some(s => s.state === 'running');
          sendJson(res, { ...roomWithSummary(state, state.rooms[idx]), hint: running ? '配置已保存，运行中的分片需重启后生效' : '' });
          return;
        }
        if (pathname === '/api/rooms/connect' && method === 'PUT') {
          const roomId = Number(params.get('room'));
          const room = state.rooms.find(r => r.id === roomId);
          if (!room) { sendJson(res, null, 404, '房间不存在'); return; }
          const body = await readJsonBody(req);
          room.connectIp = (body.connectIp || '').trim();
          room.connectPort = Number(body.connectPort) || 0;
          room.connectPorts = body.connectPorts || {};
          sendJson(res, { connectIp: room.connectIp, connectPort: room.connectPort, connectPorts: room.connectPorts });
          return;
        }
        if (pathname === '/api/rooms/copy' && method === 'POST') {
          const body = await readJsonBody(req);
          const src = state.rooms.find(r => r.id === Number(body.room));
          if (!src) { sendJson(res, null, 404, '源房间不存在'); return; }
          const id = state.nextRoomId++;
          const copy = { ...clone(src), id, key: `Cluster_${id}`, name: body.name || `${src.name}（副本）`, hasSave: false };
          state.rooms.push(copy);
          state.statusesByRoom[id] = copy.shards.filter(s => !s.disabled).map(s => ({ shard: s.name, state: 'stopped', port: s.serverPort }));
          sendJson(res, roomWithSummary(state, copy));
          return;
        }
        if (pathname === '/api/rooms' && method === 'DELETE') {
          const roomId = Number(params.get('room'));
          state.rooms = state.rooms.filter(r => r.id !== roomId);
          sendJson(res, { message: '房间已从面板移除（Mock 数据，重启脚本即恢复）' });
          return;
        }

        // 模板
        if (pathname === '/api/profiles' && method === 'GET') {
          sendJson(res, state.profiles);
          return;
        }
        if (pathname === '/api/profiles' && method === 'POST') {
          const body = await readJsonBody(req);
          const item = { id: `prof-${Date.now()}`, name: body.name || '新模板', createdAt: nowIso(), gameMode: 'survival', shards: 2, modCount: 4 };
          state.profiles.unshift(item);
          sendJson(res, item);
          return;
        }
        if (pathname === '/api/profiles' && method === 'DELETE') {
          const id = params.get('id');
          state.profiles = state.profiles.filter(p => p.id !== id);
          sendJson(res, { ok: true });
          return;
        }

        // 进程状态与启停
        if (pathname === '/api/status') {
          const roomId = Number(params.get('room'));
          sendJson(res, state.statusesByRoom[roomId] || []);
          return;
        }
        if (pathname === '/api/start') {
          const roomId = Number(params.get('room'));
          const targetShard = params.get('shard') || '';
          const room = state.rooms.find(r => r.id === roomId);
          if (!room) { sendJson(res, null, 404, '房间不存在'); return; }
          const list = state.statusesByRoom[roomId] || [];
          for (const shard of room.shards.filter(s => !s.disabled && (!targetShard || s.name === targetShard))) {
            let st = list.find(x => x.shard === shard.name);
            if (!st) { st = { shard: shard.name }; list.push(st); }
            st.state = 'running';
            st.pid = 20000 + Math.floor(Math.random() * 9000);
            st.port = shard.serverPort || 10999;
            st.cpuPct = Number((8 + Math.random() * 10).toFixed(1));
            st.memBytes = 820_000_000 + Math.floor(Math.random() * 450_000_000);
            st.uptime = '刚刚启动';
            st.version = state.serverInfo.buildId;
            st.startedAt = nowIso();
            st.playersReady = true;
            pushLog(state, roomId, shard.name, `[Mock] 分片 ${shard.name} 已启动 (PID ${st.pid}, UDP ${st.port})`);
          }
          state.statusesByRoom[roomId] = list;
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/stop') {
          const roomId = Number(params.get('room'));
          const targetShard = params.get('shard') || '';
          const list = state.statusesByRoom[roomId] || [];
          for (const st of list) {
            if (!targetShard || st.shard === targetShard) {
              st.state = 'stopped';
              delete st.pid;
              delete st.cpuPct;
              delete st.memBytes;
              delete st.uptime;
              st.playersReady = false;
              pushLog(state, roomId, st.shard, `[Mock] 分片 ${st.shard} 已保存并优雅停止`);
            }
          }
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/restart') {
          const roomId = Number(params.get('room'));
          const targetShard = params.get('shard') || '';
          const list = state.statusesByRoom[roomId] || [];
          for (const st of list) {
            if (!targetShard || st.shard === targetShard) {
              st.state = 'running';
              st.pid = 20000 + Math.floor(Math.random() * 9000);
              st.startedAt = nowIso();
              st.playersReady = true;
              pushLog(state, roomId, st.shard, `[Mock] 分片 ${st.shard} 已重启完成 (PID ${st.pid})`);
            }
          }
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/cmd') {
          const roomId = Number(params.get('room'));
          const shard = params.get('shard') || 'Master';
          const cmd = params.get('cmd') || '';
          pushLog(state, roomId, shard, `[Console] > ${cmd}`);
          pushLog(state, roomId, shard, '[Mock] 指令已投递至分片控制台');
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/quick-action' && method === 'POST') {
          const roomId = Number(params.get('room'));
          const body = await readJsonBody(req);
          const ws = state.worldStatesByRoom[roomId]?.Master?.state;
          if (body.action === 'advance' && ws) ws.day += 1;
          if (body.action === 'save' && ws) ws.savedAt = nowIso();
          if (body.action === 'regenerate' && ws) { ws.day = 1; ws.season = 'autumn'; ws.remainingDays = 20; ws.savedAt = nowIso(); }
          pushLog(state, roomId, 'Master', `[QuickAction] 执行 ${body.action}${body.message ? `：${body.message}` : ''}`);
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/rollback' && method === 'POST') {
          const roomId = Number(params.get('room'));
          const body = await readJsonBody(req);
          const count = Number(body.count) || 1;
          const ws = state.worldStatesByRoom[roomId]?.Master?.state;
          if (ws) ws.day = Math.max(1, ws.day - count);
          pushLog(state, roomId, 'Master', `[Rollback] 已请求回档 ${count} 天`);
          sendJson(res, { requested: true, retryAfterSeconds: 15 });
          return;
        }

        // 世界状态、宿主机与外网 IP
        if (pathname === '/api/worldstate') {
          const roomId = Number(params.get('room'));
          const map = state.worldStatesByRoom[roomId] || {};
          sendJson(res, Object.entries(map).map(([shard, entry]) => ({ shard, ...entry })));
          return;
        }
        if (pathname === '/api/host') {
          sendJson(res, {
            diskPath: 'C:\\DSTServer\\dst_save',
            memory: { totalBytes: 16 * 1024 ** 3, availableBytes: 5.4 * 1024 ** 3 },
            disk: { totalBytes: 512 * 1024 ** 3, availableBytes: 214.6 * 1024 ** 3 },
          });
          return;
        }
        if (pathname === '/api/network/public-ip') {
          sendJson(res, { ip: state.publicIp, source: 'mock-probe', cached: params.get('refresh') !== '1' });
          return;
        }

        // 在线玩家与名单
        if (pathname === '/api/players') {
          const roomId = Number(params.get('room'));
          sendJson(res, { players: state.playersByRoom[roomId] || [], errors: {} });
          return;
        }
        if (pathname === '/api/players/action' && method === 'POST') {
          const roomId = Number(params.get('room'));
          const body = await readJsonBody(req);
          const list = state.playersByRoom[roomId] || [];
          const p = list.find(x => x.userid === body.userid && x.shard === body.shard);
          if (body.action === 'kick' || body.action === 'ban') {
            state.playersByRoom[roomId] = list.filter(x => x.userid !== body.userid);
            if (body.action === 'ban') {
              const bl = state.listsByRoom[roomId]?.blocklist || [];
              if (!bl.includes(body.userid)) bl.push(body.userid);
            }
          } else if (body.action === 'revive' && p) {
            p.ghost = false;
          }
          pushLog(state, roomId, body.shard || 'Master', `[Player] 对 ${p?.name || body.userid} 执行 ${body.action}`);
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/admins') {
          const roomId = Number(params.get('room'));
          if (!state.adminsByRoom[roomId]) state.adminsByRoom[roomId] = [];
          if (method === 'POST') {
            const body = await readJsonBody(req);
            if (body.kleiId && !state.adminsByRoom[roomId].includes(body.kleiId)) state.adminsByRoom[roomId].push(body.kleiId);
          } else if (method === 'DELETE') {
            const id = params.get('id');
            state.adminsByRoom[roomId] = state.adminsByRoom[roomId].filter(x => x !== id);
          }
          sendJson(res, { admins: state.adminsByRoom[roomId], seen: state.seenPlayers, file: `Cluster_${roomId}/adminlist.txt`, needRestart: false });
          return;
        }
        if (pathname === '/api/player-lists') {
          const roomId = Number(params.get('room'));
          const kind = params.get('kind') || 'blocklist';
          if (!state.listsByRoom[roomId]) state.listsByRoom[roomId] = { blocklist: [], whitelist: [] };
          if (method === 'POST') {
            const body = await readJsonBody(req);
            if (body.kleiId && !state.listsByRoom[roomId][kind].includes(body.kleiId)) state.listsByRoom[roomId][kind].push(body.kleiId);
          } else if (method === 'DELETE') {
            const body = await readJsonBody(req);
            state.listsByRoom[roomId][kind] = state.listsByRoom[roomId][kind].filter(x => x !== body.kleiId);
          }
          const ids = state.listsByRoom[roomId][kind] || [];
          sendJson(res, { ids, seen: state.seenPlayers, file: `Cluster_${roomId}/${kind}.txt`, needRestart: false, reservedSlots: kind === 'whitelist' ? ids.length : 0 });
          return;
        }

        // 玩家日志（支持 NDJSON 流式与分页搜索）
        if (pathname === '/api/chat-log' && method === 'GET') {
          const roomId = Number(params.get('room'));
          const kind = params.get('kind') || 'all';
          const search = (params.get('search') || '').trim().toLowerCase();
          const page = Math.max(1, Number(params.get('page')) || 1);
          const limit = Math.max(1, Number(params.get('limit')) || 10);
          const all = (state.chatLogsByRoom[roomId] || []).filter(item => {
            if (kind !== 'all' && item.kind !== kind) return false;
            if (!search) return true;
            return `${item.name} ${item.userid} ${item.message} ${item.steamid}`.toLowerCase().includes(search);
          });
          const slice = all.slice((page - 1) * limit, page * limit);
          if (params.get('stream') === '1') {
            res.writeHead(200, { 'Content-Type': 'application/x-ndjson; charset=utf-8', 'Cache-Control': 'no-store' });
            res.write(JSON.stringify({ type: 'meta', exists: true, truncated: false, modified: nowIso(3), total: all.length, page, pageSize: limit, count: slice.length }) + '\n');
            for (const entry of slice) {
              res.write(JSON.stringify({ type: 'entry', entry }) + '\n');
            }
            res.write(JSON.stringify({ type: 'done', total: all.length }) + '\n');
            res.end();
            return;
          }
          sendJson(res, { exists: true, truncated: false, modified: nowIso(3), total: all.length, page, pageSize: limit, entries: slice });
          return;
        }
        if (pathname === '/api/chat-log' && method === 'DELETE') {
          const roomId = Number(params.get('room'));
          const body = await readJsonBody(req);
          const ids = new Set(body.ids || []);
          state.chatLogsByRoom[roomId] = (state.chatLogsByRoom[roomId] || []).filter(item => !ids.has(item.id));
          sendJson(res, { deleted: ids.size });
          return;
        }

        // 世界配置
        if (pathname === '/api/worldconfig' && method === 'GET') {
          const roomId = Number(params.get('room'));
          const shard = params.get('shard') || 'Master';
          sendJson(res, clone(getWorldConfig(state, roomId, shard)));
          return;
        }
        if (pathname === '/api/worldconfig' && method === 'PUT') {
          const roomId = Number(params.get('room'));
          const shard = params.get('shard') || 'Master';
          const body = await readJsonBody(req);
          const cfg = getWorldConfig(state, roomId, shard);
          cfg.config.overrideEnabled = !!body.overrideEnabled;
          cfg.config.worldgenPreset = body.worldgenPreset || cfg.config.worldgenPreset;
          cfg.config.settingsPreset = body.settingsPreset || cfg.config.settingsPreset;
          cfg.config.overrides = { ...(body.overrides || {}) };
          sendJson(res, { saved: true, needRestart: false });
          return;
        }

        // 模组与创意工坊
        if (pathname === '/api/mods' && method === 'GET') {
          const roomId = Number(params.get('room'));
          const list = state.modsByRoom[roomId] || [];
          if (params.get('stream') === '1') {
            res.writeHead(200, { 'Content-Type': 'application/x-ndjson; charset=utf-8', 'Cache-Control': 'no-store' });
            res.write(JSON.stringify({ type: 'meta', total: list.length }) + '\n');
            for (const mod of list) {
              res.write(JSON.stringify({ type: 'mod', mod }) + '\n');
            }
            res.write(JSON.stringify({ type: 'done', total: list.length }) + '\n');
            res.end();
            return;
          }
          sendJson(res, list);
          return;
        }
        if (pathname === '/api/mods/toggle') {
          const roomId = Number(params.get('room'));
          const id = params.get('id');
          const enabled = params.get('enabled') === 'true' || params.get('enabled') === '1';
          const list = state.modsByRoom[roomId] || [];
          let mod = list.find(m => m.id === id);
          if (!mod && id) {
            mod = { id, name: id, enabled, installed: false, version: '1.0', author: 'Workshop', localTime: 0, options: {} };
            list.push(mod);
          } else if (mod) {
            mod.enabled = enabled;
          }
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/mods/schema') {
          const id = params.get('id');
          sendJson(res, state.modSchemas[id] || [
            { name: 'enabled_feature', label: '启用扩展特性', default: true, type: 'boolean' },
            { name: 'multiplier', label: '倍率设置', default: '1', choices: [{ data: '0.5', description: '0.5x' }, { data: '1', description: '1x（默认）' }, { data: '2', description: '2x' }] },
          ]);
          return;
        }
        if (pathname === '/api/mods/config') {
          const roomId = Number(params.get('room'));
          const id = params.get('id');
          const key = params.get('key');
          const value = params.get('value');
          const mod = (state.modsByRoom[roomId] || []).find(m => m.id === id);
          if (mod) {
            if (!mod.options) mod.options = {};
            mod.options[key] = value;
          }
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/mods/delete') {
          const roomId = Number(params.get('room'));
          const id = params.get('id');
          state.modsByRoom[roomId] = (state.modsByRoom[roomId] || []).filter(m => m.id !== id);
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/mods/update') {
          if (method === 'POST') {
            const roomId = Number(params.get('room'));
            const id = params.get('id');
            for (const mod of state.modsByRoom[roomId] || []) {
              if (!id || mod.id === id) {
                mod.installed = true;
                mod.localTime = Math.floor(Date.now() / 1000);
              }
            }
          }
          sendJson(res, { running: false, progress: '已完成', error: '', output: '[Mock] 模组校验完成' });
          return;
        }
        if (pathname === '/api/mods/updates') {
          sendJson(res, {
            checked: 6,
            missing: 1,
            updates: [
              { id: 'workshop-1207269058', name: 'Simple Health Bar DST（简易血条）', localTime: Math.floor(Date.now() / 1000) - 86400 * 30, remoteTime: Math.floor(Date.now() / 1000) - 3600 },
            ],
          });
          return;
        }
        if (pathname === '/api/workshop/details') {
          const ids = (params.get('input') || '').split(',').map(s => s.replace(/^workshop-/, '').trim()).filter(Boolean);
          sendJson(res, ids.map(id => ({
            id,
            title: `工坊模组 #${id}`,
            previewUrl: '',
            subscriptions: 128400,
            updatedAt: Math.floor(Date.now() / 1000) - 86400 * 5,
            description: '这是 Mock 环境生成的创意工坊模组详情。',
          })));
          return;
        }
        if (pathname === '/api/workshop/search') {
          const q = params.get('q') || 'DST';
          sendJson(res, {
            total: 3,
            items: [
              { id: '378160973', title: `Global Positions（匹配：${q}）`, subscriptions: 2450000, updatedAt: Math.floor(Date.now() / 1000) - 86400 * 12 },
              { id: '1207269058', title: `Simple Health Bar（匹配：${q}）`, subscriptions: 1820000, updatedAt: Math.floor(Date.now() / 1000) - 86400 * 3 },
              { id: '1595631294', title: `Smart Minisign（匹配：${q}）`, subscriptions: 980000, updatedAt: Math.floor(Date.now() / 1000) - 86400 * 9 },
            ],
          });
          return;
        }
        if (pathname === '/api/workshop/key') {
          if (method === 'PUT') {
            const body = await readJsonBody(req);
            state.steamApiKey = body.steamApiKey || '';
          }
          sendJson(res, { steamApiKey: state.steamApiKey });
          return;
        }

        // 备份与检查
        if (pathname === '/api/backups') {
          const roomId = Number(params.get('room'));
          sendJson(res, state.backupsByRoom[roomId] || []);
          return;
        }
        if (pathname === '/api/backups/create') {
          const roomId = Number(params.get('room'));
          const stamp = new Date().toISOString().replace(/[-:T]/g, '').slice(0, 15);
          const item = {
            name: `Cluster_${roomId}_${stamp}_manual.zip`,
            path: `C:\\DSTServer\\backups\\Cluster_${roomId}\\Cluster_${roomId}_${stamp}_manual.zip`,
            size: 18_640_000,
            createdAt: nowIso(),
          };
          if (!state.backupsByRoom[roomId]) state.backupsByRoom[roomId] = [];
          state.backupsByRoom[roomId].unshift(item);
          sendJson(res, { ...item, syncQueued: state.backupRemote.enabled });
          return;
        }
        if (pathname === '/api/backups/delete') {
          const roomId = Number(params.get('room'));
          const name = params.get('name');
          state.backupsByRoom[roomId] = (state.backupsByRoom[roomId] || []).filter(b => b.name !== name);
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/backups/check') {
          if (method === 'POST') {
            const roomId = Number(params.get('room'));
            const body = await readJsonBody(req);
            const id = `chk-${Date.now()}`;
            const report = {
              id,
              room: roomId,
              name: body.name,
              state: 'ready',
              sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
              files: 142,
              size: 18_420_112,
              shards: [
                { shard: 'Master', location: 'forest', day: 118, season: 'spring', remainingDays: 8, hasMap: true },
                { shard: 'Caves', location: 'cave', day: 118, season: 'spring', remainingDays: 8, hasMap: true },
              ],
              warnings: [],
            };
            state.backupChecks.set(id, report);
            sendJson(res, report);
            return;
          }
          if (method === 'DELETE') {
            state.backupChecks.delete(params.get('id'));
            sendJson(res, { ok: true });
            return;
          }
          sendJson(res, state.backupChecks.get(params.get('id')) || { state: 'ready', shards: [] });
          return;
        }
        if (pathname === '/api/backups/check/map') {
          const shard = params.get('shard') || 'Master';
          sendJson(res, buildMockMap(shard, shard === 'Caves' ? 'cave' : 'forest'));
          return;
        }
        if (pathname === '/api/backups/restore') {
          sendJson(res, { ok: true, message: 'Mock 回滚完成' });
          return;
        }
        if (pathname === '/api/backups/download') {
          res.statusCode = 200;
          res.setHeader('Content-Type', 'application/octet-stream');
          res.setHeader('Content-Disposition', `attachment; filename="${params.get('name') || 'mock-backup.zip'}"`);
          res.end(Buffer.from('MOCK_DST_BACKUP_ARCHIVE'));
          return;
        }

        // 磁盘存档
        if (pathname === '/api/saves' && method === 'GET') {
          sendJson(res, { root: 'C:\\DSTServer\\dst_save', items: state.saves });
          return;
        }
        if (pathname === '/api/saves' && method === 'DELETE') {
          const cluster = params.get('cluster');
          state.saves = state.saves.filter(s => s.cluster !== cluster);
          sendJson(res, { ok: true });
          return;
        }

        // 地图预览：优先透传 --backend，其次直接解码本机 DST 真实 KTEX / minimap 图集，最后回退程序羊皮纸
        if (pathname === '/api/map/texture') {
          if (await tryProxyBinary(cli.backend, req.url, res)) return;
          const tile = params.get('tile') || '__PAPER__';
          const realPng = localGameAssets.getTexturePng(tile);
          res.statusCode = 200;
          res.setHeader('Content-Type', 'image/png');
          res.setHeader('Cache-Control', 'public, max-age=300');
          res.end(realPng || getMockTexturePng(tile));
          return;
        }
        if (pathname === '/api/map/icon') {
          if (await tryProxyBinary(cli.backend, req.url, res)) return;
          const kind = params.get('kind') || '';
          const realPng = localGameAssets.getIconPng(kind);
          if (realPng) {
            res.statusCode = 200;
            res.setHeader('Content-Type', 'image/png');
            res.setHeader('Cache-Control', 'public, max-age=300');
            res.end(realPng);
            return;
          }
          res.statusCode = 200;
          res.setHeader('Content-Type', 'image/svg+xml; charset=utf-8');
          res.setHeader('Cache-Control', 'public, max-age=300');
          res.end(getMockIconSvg(kind));
          return;
        }
        if (pathname === '/api/map') {
          const roomId = Number(params.get('room'));
          const shard = params.get('shard') || 'Master';
          const room = state.rooms.find(r => r.id === roomId);
          if (!room?.hasSave) {
            sendJson(res, null, 400, '尚无世界保存点（可切换至「主力档」或「岛屿冒险」查看模拟地图）');
            return;
          }
          const location = room.locations?.[shard] || room.shards.find(s => s.name === shard)?.worldType || 'forest';
          sendJson(res, buildMockMap(shard, location));
          return;
        }

        // 诊断 & 配置预览
        if (pathname === '/api/diagnostics') {
          const roomId = Number(params.get('room'));
          const room = state.rooms.find(r => r.id === roomId);
          sendJson(res, {
            checks: room?.needToken
              ? [{ code: 'MISSING_TOKEN', stage: '配置检查', title: '未配置 Klei 服务器令牌', suggestion: '在线开服需要填写有效的 cluster_token.txt，或在房间编辑中切换为仅局域网/离线模式。', mods: [], evidence: ['cluster_token.txt is empty'] }]
              : [],
            shards: (room?.shards || []).filter(s => !s.disabled).map(s => ({
              shard: s.name,
              findings: s.name === 'Master' && roomId === 1
                ? [{ code: 'MOD_MISSING', stage: '模组加载', title: '有 1 个已启用模组尚未下载到本地', suggestion: '请在「模组」页点击「更新模组」补齐缺失文件。', mods: [{ id: 'workshop-2798599672', name: 'Extra Equip Slots（五格装备栏）' }], evidence: ['[Warning] Mod workshop-2798599672 is not installed locally, skipping'] }]
                : [],
            })),
            downloads: { findings: [] },
          });
          return;
        }
        if (pathname === '/api/config') {
          const roomId = Number(params.get('room'));
          const room = state.rooms.find(r => r.id === roomId) || state.rooms[0];
          sendJson(res, {
            clusterDir: `C:\\DSTServer\\dst_save\\${room.key}`,
            files: [
              { path: 'cluster.ini', content: `[GAMEPLAY]\ngame_mode = ${room.gameMode}\nmax_players = ${room.maxPlayers}\npvp = ${room.pvp}\npause_when_empty = ${room.pauseEmpty}\n\n[NETWORK]\ncluster_name = ${room.name}\ncluster_description = ${room.description}\ncluster_password = ${room.password}\ncluster_language = ${room.language}\n` },
              ...room.shards.filter(s => !s.disabled).map(s => ({
                path: `${s.name}/server.ini`,
                content: `[NETWORK]\nserver_port = ${s.serverPort || 10999}\n\n[SHARD]\nis_master = ${!!s.isMaster}\nname = ${s.name}\nid = ${s.gameId || 1}\n`,
              })),
            ],
          });
          return;
        }

        // 系统设置相关（自动维护 / 异地备份 / 通知 / 服务端安装）
        if (pathname === '/api/automation') {
          if (method === 'PUT') {
            const body = await readJsonBody(req);
            state.automation.config = { ...state.automation.config, ...body };
          }
          sendJson(res, state.automation);
          return;
        }
        if (pathname === '/api/backup-remote') {
          if (method === 'PUT') {
            const body = await readJsonBody(req);
            state.backupRemote = { ...state.backupRemote, ...body };
          }
          sendJson(res, state.backupRemote);
          return;
        }
        if (pathname === '/api/backup-remote/test' && method === 'POST') {
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/backup-remote/sync' && method === 'POST') {
          const roomId = Number(params.get('room'));
          const body = await readJsonBody(req);
          const existing = state.backupRemote.results.find(r => r.room === roomId && r.name === body.name);
          if (existing) { existing.state = 'synced'; existing.at = nowIso(); existing.error = ''; }
          else state.backupRemote.results.unshift({ room: roomId, name: body.name, state: 'synced', at: nowIso(), error: '' });
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/notifications') {
          if (method === 'PUT') {
            const body = await readJsonBody(req);
            state.notifications = {
              ...state.notifications,
              enabled: !!body.enabled,
              url: body.url || '',
              events: body.events || [],
              rooms: body.rooms || [],
              secretSet: body.secret === '' ? false : (body.secret ? true : state.notifications.secretSet),
            };
          }
          sendJson(res, state.notifications);
          return;
        }
        if (pathname === '/api/notifications/test' && method === 'POST') {
          state.notifications.lastTestAt = nowIso();
          state.notifications.lastError = '';
          sendJson(res, { ok: true });
          return;
        }
        if (pathname === '/api/install' || pathname === '/api/install/update') {
          sendJson(res, { installed: state.serverInfo.installed, installing: false, progress: '', error: '', canceled: false, version: state.serverInfo.buildId, buildId: state.serverInfo.buildId });
          return;
        }
        if (pathname === '/api/server') {
          if (method === 'PUT') {
            const body = await readJsonBody(req);
            state.serverInfo.archPref = body.arch || 'auto';
            if (body.serverDir) {
              state.serverInfo.root = body.serverDir;
              state.serverInfo.pinned = true;
            } else {
              state.serverInfo.pinned = false;
            }
          }
          sendJson(res, state.serverInfo);
          return;
        }

        // 兜底：未知 API 也返回成功空对象，避免检查 UI 时抛错
        sendJson(res, {});
      });
    },
  };
}

/* ---------- 启动 Vite 开发服务器 ---------- */

const webRoot = fileURLToPath(new URL('..', import.meta.url));
const state = createMockState(cli.auth);

const server = await createServer({
  root: webRoot,
  configFile: fileURLToPath(new URL('../vite.config.js', import.meta.url)),
  plugins: [createMockApiPlugin(state, cli.delay)],
  server: {
    host: cli.host,
    port: cli.port,
    open: cli.open,
    proxy: {},
  },
});

await server.listen();

const addr = server.resolvedUrls?.local?.[0] || `http://${cli.host}:${cli.port}/`;
console.log('');
console.log('===============================================');
console.log('  DST 开服控制台 · Mock UI 预览服务已启动');
console.log('===============================================');
console.log(`  访问地址  ${addr}`);
console.log(`  接口延迟  ${cli.delay} ms（加 --delay 600 可观察骨架屏）`);
console.log(`  地图材质  ${cli.backend ? `优先透传 ${cli.backend}` : localGameAssets.hasAssets ? `已直连本机 DST 真实贴图（${localGameAssets.roots.length} 个目录，${localGameAssets.iconCount} 个小地图图标）` : '未探测到本机 DST，回退程序羊皮纸（可加 --game-dir 或 --backend）'}`);
console.log(`  登录模式  ${cli.auth ? '已开启（admin / admin）' : '免密直进（加 --auth 可预览登录页）'}`);
console.log('  预置场景  Room 1 运行中（4 名在线玩家 + 实时日志 + 模拟地图）');
console.log('            Room 2 四世界停服（可编辑世界配置、模组与分片直连）');
console.log('            Room 3 缺令牌新档（可检查告警态与空状态）');
console.log('===============================================');
console.log('  按 Ctrl+C 停止预览；修改 Vue/CSS 支持热更新');
console.log('');
