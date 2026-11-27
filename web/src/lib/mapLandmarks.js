import { PhCrown, PhDiamond, PhDoorOpen, PhFlask, PhKeyhole, PhMountains, PhSparkle, PhSpiral } from '@phosphor-icons/vue';
import { mapApi } from '@/lib/api.js';
import { CHARACTERS } from '@/lib/format.js';

// 彩色配对虫洞图标（共 22 组，对应 web/public/world-icons/dst/wormhole/mark_1..22.png）
export const WORMHOLE_MARK_ICONS = Array.from({ length: 22 }, (_, i) => `dst/wormhole/mark_${i + 1}.png`);

// 彩色配对电光三角图标（共 24 组：优先 10 个高区分度纯色，再接 14 个双色）
export const ELECTRIC_ISOSCELES_ICONS = [
  'red.png',
  'blue.png',
  'green.png',
  'yellow.png',
  'purple.png',
  'orange.png',
  'lightblue.png',
  'pink.png',
  'white.png',
  'brown.png',
  'red_yellow.png',
  'blue_green.png',
  'red_lightblue.png',
  'green_pink.png',
  'blue_yellow.png',
  'purple_white.png',
  'lightblue_orange.png',
  'red_green.png',
  'blue_pink.png',
  'green_yellow.png',
  'red_white.png',
  'blue_white.png',
  'green_white.png',
  'yellow_pink.png',
].map(file => `island-adventures/electric isosceles/${file}`);

function publicIconUrl(relPath) {
  return '/world-icons/' + relPath.split('/').map(encodeURIComponent).join('/');
}

const PAIRED_ICON_PACKS = {
  wormhole: WORMHOLE_MARK_ICONS.map(publicIconUrl),
  bermudatriangle: ELECTRIC_ISOSCELES_ICONS.map(publicIconUrl),
};

/**
 * 为已配对的虫洞与电光三角分配成对的颜色图标 URL（同一对两端同色）。
 * 优先使用后端给出的 pair 序号；若缺失则按 target 关系稳定顺排。
 */
export function buildPairedLandmarkIconMap(landmarks = []) {
  const result = new Map();
  const byId = new Map(landmarks.map(point => [point.id, point]));
  const nextSeq = { wormhole: 0, bermudatriangle: 0 };

  for (const point of landmarks) {
    const pack = PAIRED_ICON_PACKS[point.kind];
    if (!pack || !pack.length || result.has(point.id)) continue;

    let pair = Number(point.pair) || 0;
    const other = point.target ? byId.get(point.target) : null;
    if (!pair && other && other.kind === point.kind) {
      pair = Number(other.pair) || ++nextSeq[point.kind];
    }
    if (pair < 1) continue;

    if (pair > nextSeq[point.kind]) nextSeq[point.kind] = pair;
    const url = pack[(pair - 1) % pack.length];
    result.set(point.id, url);
    if (other && other.kind === point.kind) {
      result.set(other.id, url);
    }
  }
  return result;
}

/** 解析单个地标的图标地址：优先成对彩色图标，其次服务端原生小地图图标，否则回退矢量图标。 */
export function landmarkIconUrl(point, iconKinds, pairedIconMap) {
  if (!point) return '';
  if (pairedIconMap?.has(point.id)) return pairedIconMap.get(point.id);
  if (iconKinds?.has(point.kind)) return mapApi.iconUrl(point.kind);
  return '';
}

const portal = { icon: PhDoorOpen, color: '#28786f', priority: 1 };
const cave = { icon: PhMountains, color: '#705543', priority: 3 };
const relic = { icon: PhFlask, color: '#4c6789', priority: 2 };
const styles = {
  pigking: { icon: PhCrown, color: '#a87924', priority: 0 },
  octopusking: { icon: PhCrown, color: '#a87924', priority: 0 },
  volcano: { icon: PhMountains, color: '#b4482a', priority: 0 },
  volcano_altar: { icon: PhFlask, color: '#b4482a', priority: 0 },
  wormhole: { icon: PhSpiral, color: '#7e549a', priority: 4 },
  bermudatriangle: { icon: PhSpiral, color: '#3b7fa8', priority: 4 },
  multiplayer_portal: portal, multiplayer_portal_moonrock: portal, multiplayer_portal_moonrock_constr: portal, monkeyisland_portal: portal,
  seaworthy: portal, skyworthy: portal,
  cave_entrance: cave, cave_entrance_open: cave, cave_exit: cave, volcano_exit: { icon: PhMountains, color: '#b4482a', priority: 1 },
  ancient_altar: relic, ancient_altar_broken: relic, archive_switch: relic, obsidian_workbench: { icon: PhFlask, color: '#b4482a', priority: 1 },
  atrium_gate: { icon: PhKeyhole, color: '#536a7b', priority: 1 },
  slotmachine: { icon: PhSparkle, color: '#b4872a', priority: 1 },
  sharkittenspawner: { icon: PhMountains, color: '#4c6789', priority: 1 },
  packim_fishbone: { icon: PhSparkle, color: '#28786f', priority: 1 },
  doydoy: { icon: PhSparkle, color: '#4f8a55', priority: 1 },
  woodlegs_cage: { icon: PhKeyhole, color: '#87634c', priority: 1 },
  kraken: { icon: PhSparkle, color: '#5b4a6b', priority: 1 },
  resurrectionstone: { icon: PhDiamond, color: '#77675a', priority: 3 },
  tar_pool: { icon: PhDiamond, color: '#3d4753', priority: 3 },
  buriedtreasure: { icon: PhDiamond, color: '#a87924', priority: 3 },
  mermhouse_fisher: { icon: PhDiamond, color: '#416f60', priority: 3 },
};
const landmark = { icon: PhSparkle, color: '#87634c', priority: 2 };

export function landmarkStyle(kind) { return styles[kind] || landmark; }
export function landmarkCoordinates(point, map) {
  return `${Math.round((point.x - map.width / 2) * 4)}, ${Math.round((point.y - map.height / 2) * 4)}`;
}

/** 将游戏世界坐标 (x, z) 投影到地图网格坐标（与后端 internal/worldsettings/map.go 一致）。 */
export function worldToMapPoint(x, z, map) {
  if (!map || typeof x !== 'number' || typeof z !== 'number' || !Number.isFinite(x) || !Number.isFinite(z)) {
    return null;
  }
  const mx = map.width / 2 + x / 4;
  const my = map.height / 2 + z / 4;
  if (Math.abs(mx) > 8192 || Math.abs(my) > 8192) return null;
  return { x: mx, y: my };
}

export function formatWorldCoordinates(x, z) {
  if (typeof x !== 'number' || typeof z !== 'number' || !Number.isFinite(x) || !Number.isFinite(z)) return '';
  return `${Math.round(x)}, ${Math.round(z)}`;
}

export function playerAvatarUrl(prefab) {
  const key = prefab && Object.hasOwn(CHARACTERS, prefab) ? prefab : 'unknown';
  return `/character-avatars/${key}.png`;
}
