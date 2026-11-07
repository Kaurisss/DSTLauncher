// Adapted from DMP Web worldMap.js, commit 161908b3acb531b5e9639a43b7be17ba6fbd307e.
// Copyright (c) 2024 Miracle, MIT. Full notice: /licenses/dmp-map.txt.
// Kept: named colors, per-cell border shading/noise and cached bitmap rendering.
// Changed: our API uses verified LE16 IDs; no Lua execution, texture loader or random colors.

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

function hashCell(x, y) {
  let value = Math.imul(x | 0, 374761393) + Math.imul(y | 0, 668265263);
  value = Math.imul(value ^ (value >>> 13), 1274126177);
  return ((value ^ (value >>> 16)) >>> 0) / 4294967295;
}

export function terrainStyle(name) {
  const entry = TERRAIN[name];
  return entry ? { label: entry[0], color: entry[1], known: true } : { label: `未识别地形 ${name}`, color: '#756d80', known: false };
}

export function decodeTerrain(map) {
  if (!map.terrain) throw new Error('此保存点缺少可读取的地块数据');
  const raw = window.atob(map.terrain.tiles);
  if (raw.length !== map.width * map.height * 2) throw new Error('地图地块数据长度不符');
  const tiles = new Uint16Array(map.width * map.height);
  for (let i = 0; i < tiles.length; i++) tiles[i] = raw.charCodeAt(i * 2) | (raw.charCodeAt(i * 2 + 1) << 8);
  const tileById = new Map(map.terrain.specs.map(spec => [spec.id, { ...spec, ...terrainStyle(spec.name) }]));
  const counts = new Map();
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
  return { ...map, terrainImage: canvas.toDataURL('image/png'), terrainLegend: legend };
}
