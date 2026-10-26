export const CHARACTERS = {
  wilson: '威尔逊', willow: '薇洛', wolfgang: '沃尔夫冈', wendy: '温蒂',
  wx78: 'WX-78', wickerbottom: '薇克巴顿', woodie: '伍迪', wes: '韦斯',
  waxwell: '麦斯威尔', wathgrithr: '薇格弗德', webber: '韦伯', winona: '薇诺娜',
  wortox: '沃拓克斯', wormwood: '沃姆伍德', warly: '沃利', wurt: '沃特',
  walter: '沃尔特', wanda: '旺达', wonkey: '芜猴',
};

// 展示层的格式化与字典。
//
// Vue 的插值默认就是转义的，所以这里不需要 esc()。

/** 字节数转可读体积。 */
export function fmtSize(bytes) {
  const n = Number(bytes) || 0;
  if (n < 1024) return n + ' B';
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB';
  if (n < 1073741824) return (n / 1048576).toFixed(1) + ' MB';
  return (n / 1073741824).toFixed(2) + ' GB';
}

/** ISO 时间转本地可读格式。 */
export function fmtTime(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso || '');
  return d.toLocaleString('zh-CN', { hour12: false });
}

/** Unix 秒转本地可读时间（Steam 接口返回的时间戳都是秒）。 */
export function fmtUnix(sec) {
  const n = Number(sec);
  if (!Number.isFinite(n) || n <= 0) return '—';
  return new Date(n * 1000).toLocaleString('zh-CN', { hour12: false });
}

/** 分片运行状态的中文名。 */
export const STATE_LABEL = {
  running: '运行中',
  starting: '启动中',
  stopping: '停止中',
  stopped: '已停止',
  crashed: '已崩溃',
};

/** 游戏模式的中文名（英文值同时保留，方便对照 ini）。 */
export const GAME_MODE_LABEL = {
  survival: '生存 survival',
  endless: '无尽 endless',
  wilderness: '荒野 wilderness',
};

export const TICK_RATE_LABEL = {
  15: '15（省 CPU）',
  30: '30（标准）',
  60: '60（流畅）',
};

export const LANGUAGE_LABEL = {
  zh: '中文 zh',
  en: 'English',
};

/**
 * 分片类型（用于「新增世界」下拉）：只描述存档生成方式，
 * 与具体是哪张地图无关，所以用「型」而不是「世界」。
 */
export const WORLD_TYPE_LABEL = {
  forest: '地表型',
  cave: '洞穴型',
  shipwrecked: '海难',
  volcanoworld: '火山',
};

/** 模组世界的中文标签，与 WORLD_TYPE_LABEL 保持一致。 */
export const MOD_LABEL = {
  'island-adventures': '岛屿冒险',
};

/**
 * 分片显示名：优先按存档里读到的 location，缺失时按目录名。
 *
 * 同一张地图有多个常见目录名（地表可能是 Master 或 Forest），
 * 名字只是目录，location 才是事实；拿不到 location 时才用这里的映射。
 */
export const SHARD_NAME_LABEL = {
  Master: '森林',
  Forest: '森林',
  Caves: '洞穴',
  Cave: '洞穴',
};

export const SHARD_LOCATION_LABEL = {
  forest: '森林',
  cave: '洞穴',
  shipwrecked: '海难',
  volcanoworld: '火山',
};

/** 分片显示名。location 拿不到（世界还没生成）时才回落到目录名。 */
export function shardLabel(name, location) {
  if (location) return SHARD_LOCATION_LABEL[location] || WORLD_TYPE_LABEL[location] || location;
  return SHARD_NAME_LABEL[name] || name;
}

/** 房间列表里的分片显示名：按存档 location 优先，与其它页面保持一致。 */
export function roomShardLabel(room, shard) {
  return shardLabel(shard.name, room?.locations?.[shard.name]);
}

/** 进程事件类型 -> 展示用前缀，undefined 表示按普通日志处理。 */
export const EVENT_LABEL = {
  start: '启动',
  stop: '停止',
  crash: '崩溃',
  restart: '重启',
  ready: '就绪',
  warn: '警告',
};
