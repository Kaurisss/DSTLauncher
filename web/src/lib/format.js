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
 * 世界类型。分片的名字是存档目录名，可以随便取；
 * 类型才决定世界生成预设与可用设置项，所以 UI 上一律按类型给标签。
 * 后两项由模组提供（岛屿冒险），不写入 rooms.json，只按存档里的 location 实时判断。
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

/** 常见分片的友好名。自定义分片直接显示原名。 */
export const SHARD_NAME_LABEL = {
  Master: '地表',
  Caves: '洞穴',
};

/**
 * 分片标签按存档里的 location 显示世界名。
 *
 * 分片目录名只是目录名：Cluster_2 里的 Cave 装着火山、Caves 装着洞穴，
 * 靠名字猜一定会错，所以以 location 为准。拿不到（还没生成过世界）
 * 才回落到名字。
 */
export const SHARD_LOCATION_LABEL = {
  forest: '森林',
  cave: '洞穴',
  shipwrecked: '海难',
  volcanoworld: '火山',
};

export function shardLabel(name, location) {
  if (location) return SHARD_LOCATION_LABEL[location] || WORLD_TYPE_LABEL[location] || location;
  return SHARD_NAME_LABEL[name] || name;
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
