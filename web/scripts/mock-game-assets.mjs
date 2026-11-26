// 本地 DST 游戏材质（KTEX）与小地图图标图集（minimap_data*.xml）纯 Node 读取器。
// 与 Go 端 internal/modmgr/tex.go、internal/web/map_textures.go、map_icons.go 保持同源逻辑，
// 让 dev:mock 在安装有 Steam 版 DST 的本机直接展示 100% 真实游戏小地图贴图与原生图标。

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { basename, dirname, join, resolve } from 'node:path';
import { deflateSync } from 'node:zlib';

/* ---------- PNG 编码 ---------- */

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let i = 0; i < 256; i++) {
    let c = i;
    for (let k = 0; k < 8; k++) c = (c & 1) ? (0xedb88320 ^ (c >>> 1)) : (c >>> 1);
    table[i] = c >>> 0;
  }
  return table;
})();

function crc32(buf) {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function pngChunk(type, data) {
  const header = Buffer.alloc(8);
  header.writeUInt32BE(data.length, 0);
  header.write(type, 4, 4, 'ascii');
  const body = Buffer.concat([header.subarray(4), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body), 0);
  return Buffer.concat([header, data, crc]);
}

export function encodePng(width, height, rgba) {
  const raw = Buffer.alloc(height * (1 + width * 4));
  for (let y = 0; y < height; y++) {
    const rowStart = y * (1 + width * 4);
    raw[rowStart] = 0;
    rgba.copy(raw, rowStart + 1, y * width * 4, (y + 1) * width * 4);
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    pngChunk('IHDR', ihdr),
    pngChunk('IDAT', deflateSync(raw, { level: 6 })),
    pngChunk('IEND', Buffer.alloc(0)),
  ]);
}

/* ---------- Klei KTEX 解码器（对标 internal/modmgr/tex.go） ---------- */

function texRGB565(v) {
  const r = (v >> 11) & 31;
  const g = (v >> 5) & 63;
  const b = v & 31;
  return [(r << 3) | (r >> 2), (g << 2) | (g >> 4), (b << 3) | (b >> 2), 255];
}

function setTEXPixel(rgba, w, h, x, y, r, g, b, a) {
  let pr = r; let pg = g; let pb = b;
  if (a !== 0 && a !== 255) {
    pr = Math.min(255, Math.floor((pr * 255) / a));
    pg = Math.min(255, Math.floor((pg * 255) / a));
    pb = Math.min(255, Math.floor((pb * 255) / a));
  } else if (a === 0) {
    pr = 0; pg = 0; pb = 0;
  }
  const o = ((h - 1 - y) * w + x) * 4;
  rgba[o] = pr;
  rgba[o + 1] = pg;
  rgba[o + 2] = pb;
  rgba[o + 3] = a;
}

export function decodeKTEX(data) {
  if (data.length < 18 || data.toString('ascii', 0, 4) !== 'KTEX') {
    throw new Error('无效的 KTEX 文件');
  }
  const header = data.readUInt32LE(4);
  let format = (header >>> 4) & 31;
  let mipmaps = (header >>> 13) & 31;
  if ((header & 0xffffc000) === 0xffffc000) {
    format = (header >>> 3) & 7;
    mipmaps = (header >>> 9) & 15;
  }
  const offset = 8 + mipmaps * 10;
  if (mipmaps === 0 || offset > data.length) {
    throw new Error('无效的 KTEX mipmap');
  }
  const w = data.readUInt16LE(8);
  const h = data.readUInt16LE(10);
  if (w === 0 || h === 0 || w * h > 4 << 20) {
    throw new Error('KTEX 尺寸无效');
  }

  let stride = 0;
  let blockSize = 16;
  if (format === 0) {
    blockSize = 8;
  } else if (format === 4 || format === 5) {
    const channels = format === 5 ? 3 : 4;
    stride = data.readUInt16LE(12) || w * channels;
  } else if (format !== 1 && format !== 2) {
    throw new Error(`不支持的 KTEX 格式: ${format}`);
  }

  const needed = stride > 0
    ? stride * h
    : Math.ceil(w / 4) * Math.ceil(h / 4) * blockSize;
  const size = data.readUInt32LE(14);
  if (size < needed || size > data.length - offset) {
    throw new Error('KTEX 数据不完整');
  }

  const pixels = data.subarray(offset, offset + size);
  const rgba = Buffer.alloc(w * h * 4);

  if (stride > 0) {
    const channels = format === 5 ? 3 : 4;
    for (let y = 0; y < h; y++) {
      for (let x = 0; x < w; x++) {
        const i = y * stride + x * channels;
        setTEXPixel(rgba, w, h, x, y, pixels[i], pixels[i + 1], pixels[i + 2], channels === 4 ? pixels[i + 3] : 255);
      }
    }
    return { width: w, height: h, rgba };
  }

  let pos = 0;
  const alphaTable = new Uint8Array(8);
  for (let by = 0; by < h; by += 4) {
    for (let bx = 0; bx < w; bx += 4) {
      const block = pixels.subarray(pos, pos + blockSize);
      pos += blockSize;
      const colorBlock = format === 0 ? block : block.subarray(8);
      const c0 = colorBlock.readUInt16LE(0);
      const c1 = colorBlock.readUInt16LE(2);
      const p0 = texRGB565(c0);
      const p1 = texRGB565(c1);
      const p2 = [0, 0, 0, 255];
      const p3 = [0, 0, 0, (c0 > c1 || format !== 0) ? 255 : 0];
      for (let ch = 0; ch < 3; ch++) {
        if (c0 > c1 || format !== 0) {
          p2[ch] = ((2 * p0[ch] + p1[ch]) / 3) | 0;
          p3[ch] = ((p0[ch] + 2 * p1[ch]) / 3) | 0;
        } else {
          p2[ch] = ((p0[ch] + p1[ch]) / 2) | 0;
        }
      }
      const palette = [p0, p1, p2, p3];

      let alphaBits = 0n;
      if (format === 2) {
        const a0 = block[0];
        const a1 = block[1];
        alphaTable[0] = a0;
        alphaTable[1] = a1;
        if (a0 > a1) {
          for (let i = 2; i < 8; i++) alphaTable[i] = (((8 - i) * a0 + (i - 1) * a1) / 7) | 0;
        } else {
          for (let i = 2; i < 6; i++) alphaTable[i] = (((6 - i) * a0 + (i - 1) * a1) / 5) | 0;
          alphaTable[6] = 0;
          alphaTable[7] = 255;
        }
        alphaBits = block.readBigUInt64LE(0) >> 16n;
      }

      const indices = colorBlock.readUInt32LE(4);
      for (let n = 0; n < 16; n++) {
        const x = bx + (n & 3);
        const y = by + (n >> 2);
        if (x >= w || y >= h) continue;
        const base = palette[(indices >>> (2 * n)) & 3];
        let a = base[3];
        if (format === 1) {
          a = ((block[n >> 1] >>> (4 * (n & 1))) & 15) * 17;
        } else if (format === 2) {
          a = alphaTable[Number((alphaBits >> BigInt(3 * n)) & 7n)];
        }
        setTEXPixel(rgba, w, h, x, y, base[0], base[1], base[2], a);
      }
    }
  }
  return { width: w, height: h, rgba };
}

function downsampleImage(img, maxDim) {
  let { width: w, height: h, rgba } = img;
  while (w > maxDim || h > maxDim) {
    const nw = Math.max(1, w >> 1);
    const nh = Math.max(1, h >> 1);
    const dst = Buffer.alloc(nw * nh * 4);
    for (let y = 0; y < nh; y++) {
      for (let x = 0; x < nw; x++) {
        const sx = x * 2;
        const sy = y * 2;
        const sx1 = Math.min(w - 1, sx + 1);
        const sy1 = Math.min(h - 1, sy + 1);
        const idx = [
          (sy * w + sx) * 4,
          (sy * w + sx1) * 4,
          (sy1 * w + sx) * 4,
          (sy1 * w + sx1) * 4,
        ];
        const o = (y * nw + x) * 4;
        for (let c = 0; c < 4; c++) {
          dst[o + c] = ((rgba[idx[0] + c] + rgba[idx[1] + c] + rgba[idx[2] + c] + rgba[idx[3] + c]) >> 2);
        }
      }
    }
    w = nw; h = nh; rgba = dst;
  }
  return { width: w, height: h, rgba };
}

/* ---------- 材质与图标映射（与 Go 端保持一致） ---------- */

const TILE_TEXTURES = {
  __PAPER__: ['images/minimap_paper.tex'],
  __OCEAN_NOISE__: ['levels/textures/ocean_noise.tex'],
  __MAP_EDGE__: ['levels/tiles/map_edge.tex'],
  ROAD: ['levels/textures/mini_cobblestone_noise.tex'],
  ROCKY: ['levels/textures/mini_rocky_noise.tex'],
  DIRT: ['levels/textures/mini_dirt_noise.tex'],
  SAVANNA: ['levels/textures/mini_grass2_noise.tex'],
  GRASS: ['levels/textures/mini_grass_noise.tex'],
  FOREST: ['levels/textures/mini_forest_noise.tex'],
  MARSH: ['levels/textures/mini_marsh_noise.tex'],
  DECIDUOUS: ['levels/textures/mini_deciduous_noise.tex'],
  DESERT_DIRT: ['levels/textures/mini_desert_dirt_noise.tex'],
  CAVE: ['levels/textures/mini_cave_noise.tex'],
  FUNGUS: ['levels/textures/mini_fungus_noise.tex'],
  FUNGUSRED: ['levels/textures/mini_fungus_red_noise.tex'],
  FUNGUSGREEN: ['levels/textures/mini_fungus_green_noise.tex'],
  SINKHOLE: ['levels/textures/mini_sinkhole_noise.tex'],
  UNDERROCK: ['levels/textures/mini_rock_noise.tex'],
  MUD: ['levels/textures/mini_mud_noise.tex'],
  BRICK: ['levels/textures/mini_ruinsbrick_noise.tex'],
  BEACH: ['levels/textures/mini_beach_noise.tex'],
  MEADOW: ['levels/textures/mini_savannah_noise.tex'],
  JUNGLE: ['levels/textures/mini_jungle_noise.tex'],
  TIDALMARSH: ['levels/textures/mini_tidalmarsh_noise.tex'],
  MAGMAFIELD: ['levels/textures/mini_magmafield_noise.tex'],
  VOLCANO_ROCK: ['levels/textures/mini_ground_volcano_noise.tex'],
  VOLCANO: ['levels/textures/mini_ground_lava_rock.tex'],
  ASH: ['levels/textures/mini_ash.tex'],
  VOLCANO_LAVA: ['levels/textures/mini_lava_noise.tex'],
  OCEAN_SHALLOW: ['levels/textures/mini_watershallow_noise.tex', 'levels/textures/mini_water_shallow.tex'],
  OCEAN_MEDIUM: ['levels/textures/mini_watermedium_noise.tex'],
  OCEAN_DEEP: ['levels/textures/mini_waterdeep_noise.tex'],
};

const LANDMARK_ICONS = {
  multiplayer_portal: ['portal_dst.png'],
  multiplayer_portal_moonrock: ['portal_dst.png'],
  pigking: ['pigking.png'],
  resurrectionstone: ['resurrection_stone.png', 'resurrect.png'],
  wormhole: ['wormhole.png'],
  cave_entrance: ['cave_closed.png', 'cave_open.png'],
  cave_entrance_open: ['cave_open.png'],
  cave_exit: ['cave_open2.png', 'cave_open.png'],
  atrium_gate: ['atrium_gate.png'],
  ancient_altar: ['tab_crafting_table.png'],
  archive_switch: ['archive_power_switch.png'],
  moonbase: ['moonbase.png'],
  oasislake: ['oasis.png'],
  beequeenhivegrown: ['beequeenhivegrown.png', 'beequeenhive.png'],
  hermithouse: ['hermitcrab_home2.png', 'hermitcrab_home.png'],
  monkeyisland_portal: ['monkey_island_portal.png'],
  terrarium: ['terrarium.png'],
  volcano: ['volcano.tex', 'volcano_active.tex'],
  volcano_exit: ['volcano_entrance.tex'],
  volcano_altar: ['volcano_altar.tex'],
  obsidian_workbench: ['obisidan_workbench.tex', 'obsidian_workbench.tex'],
  octopusking: ['octopus.tex'],
  slotmachine: ['slot_machine.tex'],
  bermudatriangle: ['bermudatriangle.tex'],
  seaworthy: ['shipwrecked_exit.tex', 'seaworthy_kit.tex'],
  coral_brain_rock: ['coral_brain_rock.tex'],
};

/* ---------- Steam / DST 目录自动探测 ---------- */

function isDir(dir) {
  try { return statSync(dir).isDirectory(); }
  catch { return false; }
}

function isFile(file) {
  try { return statSync(file).isFile(); }
  catch { return false; }
}

function discoverSteamRoots(explicitGameDir) {
  const roots = [];
  const seen = new Set();
  const add = dir => {
    if (!dir) return;
    const abs = resolve(dir);
    const key = abs.toLowerCase();
    if (seen.has(key) || !isDir(abs)) return;
    seen.add(key);
    roots.push(abs);
  };

  const addGameDir = gameDir => {
    if (!gameDir) return;
    add(join(gameDir, 'data'));
    add(join(dirname(gameDir), "Don't Starve Together", 'data'));
    const workshopDir = join(dirname(dirname(gameDir)), 'workshop', 'content', '322330');
    if (isDir(workshopDir)) {
      for (const entry of readdirSync(workshopDir, { withFileTypes: true })) {
        if (entry.isDirectory()) add(join(workshopDir, entry.name));
      }
    }
    const modsDir = join(gameDir, 'mods');
    if (isDir(modsDir)) {
      for (const entry of readdirSync(modsDir, { withFileTypes: true })) {
        if (entry.isDirectory()) add(join(modsDir, entry.name));
      }
    }
  };

  if (explicitGameDir) addGameDir(explicitGameDir);
  if (process.env.DST_SERVER_DIR) addGameDir(process.env.DST_SERVER_DIR);

  const steamBases = [
    'C:\\Program Files (x86)\\Steam',
    'C:\\Program Files\\Steam',
    'D:\\Steam',
    'D:\\SteamLibrary',
    'E:\\Steam',
    'E:\\SteamLibrary',
  ];

  // 从 Steam libraryfolders.vdf 读取全部库目录
  for (const base of [...steamBases]) {
    const vdf = join(base, 'steamapps', 'libraryfolders.vdf');
    if (!isFile(vdf)) continue;
    try {
      const content = readFileSync(vdf, 'utf8');
      for (const match of content.matchAll(/"path"\s+"([^"]+)"/g)) {
        const libPath = match[1].replace(/\\\\/g, '\\');
        if (!steamBases.includes(libPath)) steamBases.push(libPath);
      }
    } catch { /* ignore */ }
  }

  for (const base of steamBases) {
    for (const folder of ["Don't Starve Together", "Don't Starve Together Dedicated Server"]) {
      const candidate = join(base, 'steamapps', 'common', folder);
      if (isDir(candidate)) addGameDir(candidate);
    }
  }
  return roots;
}

function loadIconIndex(roots) {
  const index = new Map();
  const xmlPaths = [];
  for (const root of roots) {
    for (const name of ['minimap_data1.xml', 'minimap_data2.xml', 'minimap_data3.xml', 'minimap_data.xml']) {
      const p = join(root, 'minimap', name);
      if (isFile(p)) xmlPaths.push(p);
    }
    const modMinimap = join(root, 'images', 'minimap');
    if (isDir(modMinimap)) {
      for (const entry of readdirSync(modMinimap, { withFileTypes: true })) {
        if (entry.isFile() && entry.name.toLowerCase().endsWith('.xml')) {
          xmlPaths.push(join(modMinimap, entry.name));
        }
      }
    }
  }

  for (const xmlPath of xmlPaths) {
    let raw;
    try { raw = readFileSync(xmlPath, 'utf8'); }
    catch { continue; }
    const texMatch = raw.match(/<Texture[^>]*filename="([^"]+)"/i);
    if (!texMatch) continue;
    const texPath = join(dirname(xmlPath), basename(texMatch[1]));
    if (!isFile(texPath)) continue;

    for (const m of raw.matchAll(/<Element\s+([^>]+)\/>/gi)) {
      const attrs = m[1];
      const name = attrs.match(/name="([^"]+)"/i)?.[1];
      const u1 = Number(attrs.match(/u1="([^"]+)"/i)?.[1]);
      const u2 = Number(attrs.match(/u2="([^"]+)"/i)?.[1]);
      const v1 = Number(attrs.match(/v1="([^"]+)"/i)?.[1]);
      const v2 = Number(attrs.match(/v2="([^"]+)"/i)?.[1]);
      if (!name || ![u1, u2, v1, v2].every(Number.isFinite)) continue;
      const key = name.toLowerCase();
      if (!index.has(key)) index.set(key, { texPath, u1, u2, v1, v2 });
    }
  }
  return index;
}

export function createLocalGameAssets(explicitGameDir = '') {
  const roots = discoverSteamRoots(explicitGameDir);
  const iconIndex = loadIconIndex(roots);
  const texPngCache = new Map();
  const atlasDecodeCache = new Map();
  const iconPngCache = new Map();

  function resolveTexturePath(tile) {
    const candidates = TILE_TEXTURES[tile] || [
      `levels/textures/mini_${tile.toLowerCase()}_noise.tex`,
      `levels/textures/mini_${tile.toLowerCase()}.tex`,
      `levels/textures/ground_noise_${tile.toLowerCase()}_mini.tex`,
    ];
    for (const rel of candidates) {
      for (const root of roots) {
        const full = join(root, ...rel.split('/'));
        if (existsSync(full) && isFile(full)) return full;
      }
    }
    return '';
  }

  function getTexturePng(tile) {
    if (texPngCache.has(tile)) return texPngCache.get(tile);
    const full = resolveTexturePath(tile);
    if (!full) {
      texPngCache.set(tile, null);
      return null;
    }
    try {
      const decoded = downsampleImage(decodeKTEX(readFileSync(full)), 256);
      const png = encodePng(decoded.width, decoded.height, decoded.rgba);
      texPngCache.set(tile, png);
      return png;
    } catch {
      texPngCache.set(tile, null);
      return null;
    }
  }

  function getIconPng(kind) {
    if (iconPngCache.has(kind)) return iconPngCache.get(kind);
    const candidates = LANDMARK_ICONS[kind] || [`${kind.toLowerCase()}.png`, `${kind.toLowerCase()}.tex`];
    let hit = null;
    for (const name of candidates) {
      if (iconIndex.has(name.toLowerCase())) {
        hit = iconIndex.get(name.toLowerCase());
        break;
      }
    }
    if (!hit) {
      iconPngCache.set(kind, null);
      return null;
    }
    try {
      let atlas = atlasDecodeCache.get(hit.texPath);
      if (!atlas) {
        atlas = decodeKTEX(readFileSync(hit.texPath));
        atlasDecodeCache.set(hit.texPath, atlas);
      }
      const x0 = Math.max(0, Math.min(atlas.width - 1, Math.round(hit.u1 * atlas.width)));
      const x1 = Math.max(x0 + 1, Math.min(atlas.width, Math.round(hit.u2 * atlas.width)));
      const y0 = Math.max(0, Math.min(atlas.height - 1, Math.round((1 - hit.v2) * atlas.height)));
      const y1 = Math.max(y0 + 1, Math.min(atlas.height, Math.round((1 - hit.v1) * atlas.height)));
      const cw = x1 - x0;
      const ch = y1 - y0;
      const crop = Buffer.alloc(cw * ch * 4);
      for (let y = 0; y < ch; y++) {
        const srcOff = ((y0 + y) * atlas.width + x0) * 4;
        atlas.rgba.copy(crop, y * cw * 4, srcOff, srcOff + cw * 4);
      }
      const resized = downsampleImage({ width: cw, height: ch, rgba: crop }, 128);
      const png = encodePng(resized.width, resized.height, resized.rgba);
      iconPngCache.set(kind, png);
      return png;
    } catch {
      iconPngCache.set(kind, null);
      return null;
    }
  }

  return {
    roots,
    hasAssets: roots.length > 0,
    iconCount: iconIndex.size,
    getTexturePng,
    getIconPng,
  };
}
