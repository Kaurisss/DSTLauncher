// Adapted from DMP Web worldMap.js, commit 161908b3acb531b5e9639a43b7be17ba6fbd307e.
// Copyright (c) 2024 Miracle, MIT. Full notice: /licenses/dmp-map.txt.
// Extended with native DST game minimap texture compositing (levels/textures/mini_*.tex,
// levels/tiles/map_edge.tex parchment tint, images/minimap_paper.tex & ocean_noise.tex)
// with graceful fallback to DMP flat colors when game textures are unavailable.

import { mapApi } from '@/lib/api.js';

const TERRAIN = {
  INVALID: ['边界', '#000000'], IMPASSABLE: ['不可通行', '#3e4e5a'],
  ROAD: ['卵石路', '#a1887f'], ROCKY: ['矿区', '#a9a293'], DIRT: ['泥地', '#a97f52'],
  SAVANNA: ['稀树草原', '#b9b86a'], GRASS: ['草地', '#4e9a5f'], FOREST: ['森林', '#2f6b46'], MARSH: ['沼泽', '#5b4a6b'],
  WOODFLOOR: ['木地板', '#8b6a45'], CARPET: ['地毯', '#8e5b7a'], CHECKER: ['棋盘格', '#8d8f93'],
  CAVE: ['洞穴', '#3d4753'], FUNGUS: ['蓝蘑菇林', '#3f7fa8'], SINKHOLE: ['洞口地形', '#4d3f36'], UNDERROCK: ['岩石地形', '#4a4a52'], MUD: ['泥浆', '#6b5540'],
  BRICK: ['远古砖', '#5a5570'], BRICK_GLOW: ['烁光砖', '#7a6aa8'], TILES: ['远古地砖', '#4d4a63'], TILES_GLOW: ['烁光地砖', '#6f6a94'], TRIM: ['远古镶边', '#3f3c52'], TRIM_GLOW: ['烁光镶边', '#5d5880'],
  FUNGUSRED: ['红蘑菇林', '#a8524f'], FUNGUSGREEN: ['绿蘑菇林', '#4f8a55'], FUNGUSMOON: ['月亮蘑菇林', '#5aa3a0'],
  DECIDUOUS: ['落叶林', '#8a6f3d'], DESERT_DIRT: ['沙漠', '#d9c18a'], SCALE: ['龙蝇地皮', '#8c3f3f'],
  LAVAARENA_FLOOR: ['岩浆竞技场地面', '#5c3a33'], LAVAARENA_TRIM: ['岩浆竞技场镶边', '#7a4a3c'],
  QUAGMIRE_PEATFOREST: ['泥沼森林', '#4c5a3d'], QUAGMIRE_PARKFIELD: ['公园草地', '#5a7a4a'], QUAGMIRE_PARKSTONE: ['公园石径', '#8a8577'], QUAGMIRE_GATEWAY: ['大门', '#6a5a4a'], QUAGMIRE_SOIL: ['沃土', '#6b4f37'], QUAGMIRE_CITYSTONE: ['城市石', '#8a7f6a'],
  PEBBLEBEACH: ['卵石滩', '#9a9187'], METEOR: ['陨石区', '#7a6f66'], SHELLBEACH: ['贝壳滩', '#d8cbb8'], ARCHIVE: ['档案馆', '#8a7f5f'], FARMING_SOIL: ['耕作土壤', '#6b4f37'],
  OCEAN_COASTAL: ['浅海', '#2a7fb8'], OCEAN_COASTAL_SHORE: ['海岸', '#3a92c9'], OCEAN_SWELL: ['中海', '#20689a'], OCEAN_ROUGH: ['深海', '#174d75'],
  OCEAN_BRINEPOOL: ['盐池', '#4fa3b8'], OCEAN_BRINEPOOL_SHORE: ['盐池海岸', '#63b3c6'], OCEAN_HAZARDOUS: ['危险海域', '#3a5f7a'], OCEAN_WATERLOG: ['沉水海域', '#2f6b6b'], OCEAN_ICE: ['浮冰', '#bcd9e6'],
  COTL_BRICK: ['砖地', '#7a6a5a'], COTL_GOLD: ['金砖地', '#b08a3a'], MONKEY_GROUND: ['海盗海滩', '#7a8a5a'], MONKEY_DOCK: ['码头', '#7a6a4a'],
  LUNAR_MARSH: ['月亮沼泽', '#4a7a7a'], SHADOW_MARSH: ['暗影沼泽', '#3f3a4a'], VENT: ['喷气口地面', '#5a5a5a'], VAULT: ['宝库', '#6a5a7a'], VAULT_CLEAN: ['洁净宝库', '#9a8ab0'],
  ROPE_BRIDGE: ['索桥', '#8a7a5a'], CHARLIE_VINE: ['查理藤蔓', '#3f5a3f'], WAGSTAFF_FLOOR: ['瓦格斯特夫地板', '#6a6a6a'], MOSAIC_GREY: ['灰马赛克', '#8a8a8a'], MOSAIC_RED: ['红马赛克', '#a85a5a'], MOSAIC_BLUE: ['蓝马赛克', '#5a6a9a'],
  CARPET2: ['地毯', '#8e5b7a'], BEARD_RUG: ['胡须地毯', '#7a6a5a'], RIFT_MOON: ['月亮裂隙', '#9a8ac8'], FAKE_GROUND: ['假地面', '#7a6f9a'], WEB: ['蛛网', '#c8c8c8'],
  // Island Adventures additions. Match saved names, never a hardcoded mod tile ID.
  BEACH: ['沙滩', '#dbc48b'], JUNGLE: ['丛林', '#315d42'], MEADOW: ['草甸', '#86a651'], MAGMAFIELD: ['岩浆地', '#71504a'], ASH: ['灰烬地', '#80736c'],
  MANGROVE: ['红树林', '#416f60'], MANGROVE_SHORE: ['红树林海岸', '#5a8870'], TIDALMARSH: ['潮汐沼泽', '#687652'], LUNAR_TIDALMARSH: ['月亮潮汐沼泽', '#568980'], SHADOW_TIDALMARSH: ['暗影潮汐沼泽', '#4c445c'],
  VOLCANO: ['火山', '#624c44'], VOLCANO_ROCK: ['火山岩', '#5a5453'], VOLCANO_LAVA: ['熔岩', '#db713b'], SNAKESKIN: ['蛇皮地板', '#78734c'],
  OCEAN_SHALLOW: ['浅水海域', '#4d9fa9'], OCEAN_SHALLOW_SHORE: ['浅水海岸', '#67b5ba'], OCEAN_MEDIUM: ['中海', '#20689a'], OCEAN_MEDIUM_SHORE: ['中海海岸', '#2a7fb8'], OCEAN_DEEP: ['深海', '#174d75'], OCEAN_DEEP_SHORE: ['深海海岸', '#20689a'],
  OCEAN_CORAL: ['珊瑚礁', '#428d99'], OCEAN_CORAL_SHORE: ['珊瑚海岸', '#5aa7ac'], OCEAN_SHIPGRAVEYARD: ['沉船区', '#465365'], OCEAN_SHIPGRAVEYARD_SHORE: ['沉船海岸', '#5a7383'], OCEAN_END: ['海洋边界', '#132a3d'],
};

// DST 原版海洋在小地图着色器（tiledefs.lua + OCEAN_MINIMAP_SHADER）中的基础色调
const DST_OCEAN_COLORS = {
  OCEAN_COASTAL_SHORE: [46, 94, 108],
  OCEAN_COASTAL: [34, 76, 94],
  OCEAN_SWELL: [22, 52, 86],
  OCEAN_ROUGH: [22, 30, 56],
  OCEAN_BRINEPOOL: [48, 102, 110],
  OCEAN_BRINEPOOL_SHORE: [56, 112, 118],
  OCEAN_WATERLOG: [44, 94, 98],
  OCEAN_HAZARDOUS: [22, 32, 46],
};

const VOID_TILES = new Set(['INVALID', 'IMPASSABLE', 'FAKE_GROUND', 'OCEAN_END']);

const TEX_PAPER = '__PAPER__';
const TEX_OCEAN_NOISE = '__OCEAN_NOISE__';
const TEX_MAP_EDGE = '__MAP_EDGE__';

const textureCache = new Map();

function hashCell(x, y) {
  let value = Math.imul(x | 0, 374761393) + Math.imul(y | 0, 668265263);
  value = Math.imul(value ^ (value >>> 13), 1274126177);
  return ((value ^ (value >>> 16)) >>> 0) / 4294967295;
}

function clampByte(v) {
  return v < 0 ? 0 : v > 255 ? 255 : v | 0;
}

function rgbToHex(r, g, b) {
  return '#' + [r, g, b].map(c => clampByte(c).toString(16).padStart(2, '0')).join('');
}

function smoothstep(edge0, edge1, x) {
  const t = Math.max(0, Math.min(1, (x - edge0) / (edge1 - edge0)));
  return t * t * (3 - 2 * t);
}

export function terrainStyle(name) {
  const entry = TERRAIN[name];
  return entry ? { label: entry[0], color: entry[1], known: true } : { label: `未识别地形 ${name}`, color: '#756d80', known: false };
}

async function fetchTexturePixels(key) {
  if (textureCache.has(key)) return textureCache.get(key);
  const promise = (async () => {
    try {
      const res = await fetch(mapApi.textureUrl(key));
      if (!res.ok || !(res.headers.get('content-type') || '').toLowerCase().startsWith('image/')) {
        return null;
      }
      const blob = await res.blob();
      const bitmap = await window.createImageBitmap(blob);
      const canvas = document.createElement('canvas');
      canvas.width = bitmap.width;
      canvas.height = bitmap.height;
      const ctx = canvas.getContext('2d');
      if (!ctx) {
        bitmap.close?.();
        return null;
      }
      ctx.drawImage(bitmap, 0, 0);
      bitmap.close?.();
      const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
      const data = img.data;
      let sr = 0; let sg = 0; let sb = 0; let n = 0;
      const step = Math.max(1, Math.floor((canvas.width * canvas.height) / 1024));
      for (let i = 0; i < data.length; i += step * 4) {
        if (data[i + 3] > 200) {
          sr += data[i];
          sg += data[i + 1];
          sb += data[i + 2];
          n++;
        }
      }
      const avgRGB = n > 0 ? [sr / n, sg / n, sb / n] : [225, 188, 115];
      return { width: canvas.width, height: canvas.height, data, avgRGB };
    } catch {
      return null;
    }
  })();
  textureCache.set(key, promise);
  const result = await promise;
  if (!result) textureCache.delete(key);
  return result;
}

async function loadTextures(keys) {
  if (!Array.isArray(keys) || !keys.length) return new Map();
  const entries = await Promise.all(keys.map(async key => [key, await fetchTexturePixels(key)]));
  const loaded = new Map();
  for (const [key, tex] of entries) {
    if (tex) loaded.set(key, tex);
  }
  return loaded;
}

// 3-pass 可分离盒式模糊，用于模拟 DST OCEAN_MINIMAP_SHADER 的海岸与海域渐变
function blurGrid(src, w, h, passes = 3) {
  let cur = new Float32Array(src);
  let tmp = new Float32Array(src.length);
  for (let p = 0; p < passes; p++) {
    for (let y = 0; y < h; y++) {
      const row = y * w;
      for (let x = 0; x < w; x++) {
        const xm = x > 0 ? x - 1 : 0;
        const xp = x < w - 1 ? x + 1 : w - 1;
        tmp[row + x] = (cur[row + xm] + cur[row + x] * 2 + cur[row + xp]) * 0.25;
      }
    }
    for (let y = 0; y < h; y++) {
      const ym = (y > 0 ? y - 1 : 0) * w;
      const y0 = y * w;
      const yp = (y < h - 1 ? y + 1 : h - 1) * w;
      for (let x = 0; x < w; x++) {
        cur[y0 + x] = (tmp[ym + x] + tmp[y0 + x] * 2 + tmp[yp + x]) * 0.25;
      }
    }
  }
  return cur;
}

function sampleBilinear(grid, w, h, gx, gy) {
  const x = Math.max(0, Math.min(w - 1.001, gx));
  const y = Math.max(0, Math.min(h - 1.001, gy));
  const x0 = x | 0;
  const y0 = y | 0;
  const x1 = Math.min(w - 1, x0 + 1);
  const y1 = Math.min(h - 1, y0 + 1);
  const fx = x - x0;
  const fy = y - y0;
  const i00 = y0 * w + x0;
  const i10 = y0 * w + x1;
  const i01 = y1 * w + x0;
  const i11 = y1 * w + x1;
  const top = grid[i00] + (grid[i10] - grid[i00]) * fx;
  const bot = grid[i01] + (grid[i11] - grid[i01]) * fx;
  return top + (bot - top) * fy;
}

function renderFlatFallback(map, tiles, tileById, counts) {
  const canvas = document.createElement('canvas');
  canvas.width = map.width;
  canvas.height = map.height;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('当前浏览器无法绘制地图');
  const image = ctx.createImageData(map.width, map.height);
  const colors = new Map();
  for (const [id, spec] of tileById) colors.set(id, [1, 3, 5].map(start => parseInt(spec.color.slice(start, start + 2), 16)));
  for (let y = 0; y < map.height; y++) {
    const rowOffset = y * map.width;
    for (let x = 0; x < map.width; x++) {
      const id = tiles[rowOffset + x];
      const rgb = colors.get(id);
      if (!rgb) throw new Error(`地块 ID ${id} 缺少地形名称`);
      counts.set(id, (counts.get(id) || 0) + 1);
      const border = (x > 0 && tiles[rowOffset + x - 1] !== id) ||
        (x < map.width - 1 && tiles[rowOffset + x + 1] !== id) ||
        (y > 0 && tiles[rowOffset - map.width + x] !== id) ||
        (y < map.height - 1 && tiles[rowOffset + map.width + x] !== id);
      const scale = (border ? 0.9 : 1) * (0.96 + hashCell(x, y) * 0.08);
      const offset = (rowOffset + x) * 4;
      image.data[offset] = Math.min(255, rgb[0] * scale);
      image.data[offset + 1] = Math.min(255, rgb[1] * scale);
      image.data[offset + 2] = Math.min(255, rgb[2] * scale);
      image.data[offset + 3] = 255;
    }
  }
  ctx.putImageData(image, 0, 0);
  const legend = [...counts].map(([id, count]) => ({ ...tileById.get(id), count })).sort((a, b) => b.count - a.count);
  return { ...map, terrainImage: canvas.toDataURL('image/png'), terrainLegend: legend, textured: false };
}

function renderGameTextured(map, tiles, tileById, counts, textures) {
  const w = map.width;
  const h = map.height;
  // 按地图大小选择每地块像素数（425x425 -> 4px/格 -> 1700x1700），放大后清晰且渲染快
  const tilePx = Math.max(2, Math.min(4, Math.floor(2048 / Math.max(w, h))));
  const outW = w * tilePx;
  const outH = h * tilePx;

  const canvas = document.createElement('canvas');
  canvas.width = outW;
  canvas.height = outH;
  const ctx = canvas.getContext('2d');
  if (!ctx) throw new Error('当前浏览器无法绘制地图');
  const image = ctx.createImageData(outW, outH);
  const out = image.data;

  // 提取 map_edge.tex 的羊皮纸基调（原版约为 225, 188, 115）
  const edgeTex = textures.get(TEX_MAP_EDGE);
  const tintR = (edgeTex ? edgeTex.avgRGB[0] : 225) / 255;
  const tintG = (edgeTex ? edgeTex.avgRGB[1] : 188) / 255;
  const tintB = (edgeTex ? edgeTex.avgRGB[2] : 115) / 255;

  const paperTex = textures.get(TEX_PAPER);
  const oceanNoiseTex = textures.get(TEX_OCEAN_NOISE);

  // 为每个地块 ID 预计算分类、材质与回退色
  const idInfo = new Map();
  for (const [id, spec] of tileById) {
    const name = spec.name;
    const isVoid = VOID_TILES.has(name);
    const dstOceanColor = DST_OCEAN_COLORS[name] || null;
    const tex = textures.get(name) || null;
    const flatRGB = [1, 3, 5].map(start => parseInt(spec.color.slice(start, start + 2), 16));
    // DST 原版海域（含海岸与盐池海岸）全部走海洋着色器平滑融合，避免海岸线出现异色条带
    const isDstOcean = !!dstOceanColor;
    const isOcean = isDstOcean || name.startsWith('OCEAN_') || name.startsWith('MANGROVE');
    idInfo.set(id, { id, name, isVoid, isDstOcean, isOcean, dstOceanColor, tex, flatRGB });
  }

  // 统计每类地块数量并构造海洋着色器所需的平滑场（海洋 RGB、陆地临近度、边界临近度）
  const size = w * h;
  const oceanR = new Float32Array(size);
  const oceanG = new Float32Array(size);
  const oceanB = new Float32Array(size);
  const landMask = new Float32Array(size);
  const voidMask = new Float32Array(size);
  const isLandGrid = new Uint8Array(size);
  let hasDstOcean = false;

  for (let i = 0; i < size; i++) {
    const id = tiles[i];
    const info = idInfo.get(id);
    if (!info) throw new Error(`地块 ID ${id} 缺少地形名称`);
    counts.set(id, (counts.get(id) || 0) + 1);
    if (info.isVoid) {
      voidMask[i] = 1;
      oceanR[i] = 20; oceanG[i] = 24; oceanB[i] = 34;
    } else if (info.dstOceanColor) {
      hasDstOcean = true;
      oceanR[i] = info.dstOceanColor[0];
      oceanG[i] = info.dstOceanColor[1];
      oceanB[i] = info.dstOceanColor[2];
    } else if (info.isOcean) {
      oceanR[i] = info.flatRGB[0];
      oceanG[i] = info.flatRGB[1];
      oceanB[i] = info.flatRGB[2];
    } else {
      isLandGrid[i] = 1;
      landMask[i] = 1;
      oceanR[i] = 46; oceanG[i] = 94; oceanB[i] = 108;
    }
  }

  const smoothOceanR = hasDstOcean ? blurGrid(oceanR, w, h, 4) : null;
  const smoothOceanG = hasDstOcean ? blurGrid(oceanG, w, h, 4) : null;
  const smoothOceanB = hasDstOcean ? blurGrid(oceanB, w, h, 4) : null;
  const smoothLand = hasDstOcean ? blurGrid(landMask, w, h, 3) : null;
  const smoothVoid = hasDstOcean ? blurGrid(voidMask, w, h, 4) : null;

  // 材质采样步长：每格 8 纹素（256px 纹理每 32 格完整循环，与游戏内小地图草叶/笔触尺度一致）
  const texStep = tilePx >= 4 ? 2 : 3;
  const halfPx = 0.5 / tilePx;

  for (let py = 0; py < outH; py++) {
    const gy = py / tilePx;
    const baseTy = Math.min(h - 1, gy | 0);
    const fy = gy - baseTy;
    const rowOut = py * outW * 4;

    // 纸张纹理 Y 坐标（按屏幕像素无缝平铺）
    const paperY = paperTex ? (py % paperTex.height) * paperTex.width : 0;
    const oceanNoiseY = oceanNoiseTex ? (py % oceanNoiseTex.height) * oceanNoiseTex.width : 0;

    for (let px = 0; px < outW; px++) {
      const gx = px / tilePx;
      const baseTx = Math.min(w - 1, gx | 0);
      const fx = gx - baseTx;

      // 模拟 map_edge.tex 手绘锯齿边缘：在紧贴地块接缝的像素做确定性微扰
      let tx = baseTx;
      let ty = baseTy;
      const baseIdx = baseTy * w + baseTx;
      const baseId = tiles[baseIdx];

      const nearLeft = fx < 0.28 && baseTx > 0;
      const nearRight = fx > 0.72 && baseTx < w - 1;
      const nearTop = fy < 0.28 && baseTy > 0;
      const nearBottom = fy > 0.72 && baseTy < h - 1;

      if (nearLeft || nearRight || nearTop || nearBottom) {
        const j = hashCell(px, py);
        if (nearLeft && tiles[baseIdx - 1] !== baseId && j < 0.36) tx = baseTx - 1;
        else if (nearRight && tiles[baseIdx + 1] !== baseId && j < 0.36) tx = baseTx + 1;
        else if (nearTop && tiles[baseIdx - w] !== baseId && j < 0.36) ty = baseTy - 1;
        else if (nearBottom && tiles[baseIdx + w] !== baseId && j < 0.36) ty = baseTy + 1;
      }

      const idx = ty * w + tx;
      const id = tiles[idx];
      const info = idInfo.get(id);

      let paperLuma = 0.74;
      if (paperTex) {
        const pOff = (paperY + (px % paperTex.width)) * 4;
        paperLuma = paperTex.data[pOff] / 255;
      }

      let r; let g; let b;

      if (info.isVoid) {
        // 地图外暗色羊皮纸（对齐游戏小地图边缘暗纸效果）
        const shade = 0.78 + paperLuma * 0.42;
        r = 22 * shade;
        g = 24 * shade;
        b = 29 * shade;
      } else if (info.isDstOcean && smoothOceanR) {
        const sx = gx - 0.5 + halfPx;
        const sy = gy - 0.5 + halfPx;
        r = sampleBilinear(smoothOceanR, w, h, sx, sy);
        g = sampleBilinear(smoothOceanG, w, h, sx, sy);
        b = sampleBilinear(smoothOceanB, w, h, sx, sy);

        // 叠加上游戏内 ocean_noise.tex 水波纹
        if (oceanNoiseTex) {
          const nOff = (oceanNoiseY + (px % oceanNoiseTex.width)) * 4;
          const nLuma = (oceanNoiseTex.data[nOff] * 0.3 + oceanNoiseTex.data[nOff + 1] * 0.59 + oceanNoiseTex.data[nOff + 2] * 0.11) / 255;
          const wave = 0.84 + nLuma * 0.34;
          r *= wave; g *= wave; b *= wave;
        }

        // 叠加羊皮纸质感
        const paperMod = 0.86 + paperLuma * 0.28;
        r *= paperMod; g *= paperMod; b *= paperMod;

        // 海岸暖羊皮纸晕边（对齐 OCEAN_MINIMAP_SHADER EDGE_COLOR1 = 80,69,54）
        const landAmt = sampleBilinear(smoothLand, w, h, sx, sy);
        if (landAmt > 0.02) {
          const shoreBlend = smoothstep(0.06, 0.58, landAmt) * 0.58;
          const er = 80 * (0.75 + paperLuma * 0.5);
          const eg = 69 * (0.75 + paperLuma * 0.5);
          const eb = 54 * (0.75 + paperLuma * 0.5);
          r = r + (er - r) * shoreBlend;
          g = g + (eg - g) * shoreBlend;
          b = b + (eb - b) * shoreBlend;

          // 贴近陆地的海岸浅影（EDGE_SHADOW_COLOR = 43,43,43）
          const shadow = smoothstep(0.32, 0.72, landAmt) * 0.35;
          r *= 1 - shadow;
          g *= 1 - shadow;
          b *= 1 - shadow;
        }

        // 靠近虚空边界柔和过渡到深靛蓝（EDGE_COLOR0 = 47,52,79）
        const voidAmt = sampleBilinear(smoothVoid, w, h, sx, sy);
        if (voidAmt > 0.04) {
          const edgeBlend = smoothstep(0.08, 0.75, voidAmt) * 0.72;
          const vr = 26 * (0.75 + paperLuma * 0.45);
          const vg = 30 * (0.75 + paperLuma * 0.45);
          const vb = 42 * (0.75 + paperLuma * 0.45);
          r = r + (vr - r) * edgeBlend;
          g = g + (vg - g) * edgeBlend;
          b = b + (vb - b) * edgeBlend;
        }
      } else if (info.tex) {
        // 模组海洋材质本身已带真实海蓝/青绿调色，保留原色调仅叠纸纹；
        // 陆地材质按 shaders/minimap.ksh 乘以 map_edge.tex 暖羊皮纸着色。
        const t = info.tex;
        const u = (px * texStep) % t.width;
        const v = (py * texStep) % t.height;
        const tOff = (v * t.width + u) * 4;
        if (info.isOcean) {
          r = t.data[tOff] * 0.88;
          g = t.data[tOff + 1] * 0.88;
          b = t.data[tOff + 2] * 0.90;
        } else {
          r = t.data[tOff] * tintR;
          g = t.data[tOff + 1] * tintG;
          b = t.data[tOff + 2] * tintB;
        }

        // 轻微叠入纸纹，避免纯色区域过于平滑
        if (paperTex) {
          const grain = 0.93 + paperLuma * 0.14;
          r *= grain; g *= grain; b *= grain;
        }
      } else {
        // 无独立材质的地块回退到羊皮纸柔和色
        const grain = 0.9 + paperLuma * 0.2;
        r = info.flatRGB[0] * grain;
        g = info.flatRGB[1] * grain;
        b = info.flatRGB[2] * grain;
      }

      // 海岸线与群系交界处的细墨线描边（模拟手绘地图轮廓）
      if (isLandGrid[idx]) {
        const edgeLeft = fx < 0.26 && tx > 0;
        const edgeRight = fx > 0.74 && tx < w - 1;
        const edgeTop = fy < 0.26 && ty > 0;
        const edgeBottom = fy > 0.74 && ty < h - 1;
        if (edgeLeft || edgeRight || edgeTop || edgeBottom) {
          const touchesWater =
            (edgeLeft && !isLandGrid[idx - 1]) ||
            (edgeRight && !isLandGrid[idx + 1]) ||
            (edgeTop && !isLandGrid[idx - w]) ||
            (edgeBottom && !isLandGrid[idx + w]);
          if (touchesWater) {
            r *= 0.68; g *= 0.68; b *= 0.68;
          } else {
            const touchesOtherBiome =
              (edgeLeft && tiles[idx - 1] !== id) ||
              (edgeRight && tiles[idx + 1] !== id) ||
              (edgeTop && tiles[idx - w] !== id) ||
              (edgeBottom && tiles[idx + w] !== id);
            if (touchesOtherBiome) {
              r *= 0.92; g *= 0.92; b *= 0.92;
            }
          }
        }
      }

      const o = rowOut + px * 4;
      out[o] = clampByte(r);
      out[o + 1] = clampByte(g);
      out[o + 2] = clampByte(b);
      out[o + 3] = 255;
    }
  }

  ctx.putImageData(image, 0, 0);

  // 图例色块同步采用真实材质经羊皮纸着色后的平均色，所见即所得
  const legend = [...counts].map(([id, count]) => {
    const spec = tileById.get(id);
    const info = idInfo.get(id);
    let color = spec.color;
    if (info.isVoid) {
      color = '#1b1e24';
    } else if (info.isDstOcean && info.dstOceanColor) {
      color = rgbToHex(info.dstOceanColor[0] * 1.05, info.dstOceanColor[1] * 1.05, info.dstOceanColor[2] * 1.05);
    } else if (info.tex) {
      color = info.isOcean
        ? rgbToHex(info.tex.avgRGB[0] * 0.88, info.tex.avgRGB[1] * 0.88, info.tex.avgRGB[2] * 0.90)
        : rgbToHex(info.tex.avgRGB[0] * tintR, info.tex.avgRGB[1] * tintG, info.tex.avgRGB[2] * tintB);
    }
    return { ...spec, color, count };
  }).sort((a, b) => b.count - a.count);

  return { ...map, terrainImage: canvas.toDataURL('image/png'), terrainLegend: legend, textured: true };
}

export async function decodeTerrain(map) {
  if (!map.terrain) throw new Error('此保存点缺少可读取的地块数据');
  const raw = window.atob(map.terrain.tiles);
  if (raw.length !== map.width * map.height * 2) throw new Error('地图地块数据长度不符');
  const tiles = new Uint16Array(map.width * map.height);
  for (let i = 0; i < tiles.length; i++) tiles[i] = raw.charCodeAt(i * 2) | (raw.charCodeAt(i * 2 + 1) << 8);
  const tileById = new Map(map.terrain.specs.map(spec => [spec.id, { ...spec, ...terrainStyle(spec.name) }]));
  const counts = new Map();

  const textures = await loadTextures(map.textures);
  if (textures.size === 0) {
    return renderFlatFallback(map, tiles, tileById, counts);
  }
  return renderGameTextured(map, tiles, tileById, counts, textures);
}
