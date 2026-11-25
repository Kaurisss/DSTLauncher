// 后端接口的唯一定义处。
//
// 所有端点都收敛在这里，视图组件不直接拼 URL——
// 以后接口调整只改这个文件，不用全项目搜索字符串。

const TIMEOUT_MS = 60000;
let unauthorizedHandler = null;

export function onUnauthorized(fn) {
  unauthorizedHandler = fn;
}

function makeUnauthorizedError(msg) {
  const err = new Error(msg || '请先登录面板');
  err.unauthorized = true;
  return err;
}

async function request(path, { method = 'GET', params, body, rawBody, timeoutMs = TIMEOUT_MS } = {}) {
  const url = new URL(path, location.origin);
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') url.searchParams.set(k, v);
    });
  }

  const init = { method };
  if (rawBody !== undefined) init.body = rawBody;
  if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' };
    init.body = JSON.stringify(body);
  }

  // 停止服务器要等 c_save + c_shutdown，可能几十秒，不能按默认超时掐断
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  init.signal = controller.signal;

  let res;
  try {
    res = await fetch(url, init);
  } catch (err) {
    clearTimeout(timer);
    // 保留原始错误，方便在控制台里追根因
    if (err.name === 'AbortError') {
      throw new Error('请求超时，服务器可能仍在处理中', { cause: err });
    }
    throw new Error('无法连接面板服务，请确认 dst-windows.exe 正在运行', { cause: err });
  }
  clearTimeout(timer);

  let json;
  try {
    json = await res.json();
  } catch (err) {
    if (res.status === 401) {
      if (unauthorizedHandler) unauthorizedHandler();
      throw makeUnauthorizedError();
    }
    throw new Error(`接口返回异常（HTTP ${res.status}）`, { cause: err });
  }
  if (res.status === 401 || json.code === 401) {
    if (unauthorizedHandler) unauthorizedHandler();
    throw makeUnauthorizedError(json.message);
  }
  if (json.code !== 200) {
    const error = new Error(json.message || '请求失败');
    if (res.status === 429) {
      const retryAfter = Number(res.headers.get('Retry-After'));
      if (Number.isFinite(retryAfter) && retryAfter > 0) error.retryAfter = retryAfter;
    }
    throw error;
  }
  return json.data;
}

async function readNDJSON(res, onEvent) {
  const reader = res.body.getReader();
  const decoder = new globalThis.TextDecoder();
  let buffer = '';
  let completed = false;
  const consumeLine = line => {
    const trimmed = line.trim();
    if (!trimmed) return;
    const event = JSON.parse(trimmed);
    if (event.type === 'done') completed = true;
    onEvent(event);
  };

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let idx;
      while ((idx = buffer.indexOf('\n')) >= 0) {
        consumeLine(buffer.slice(0, idx));
        buffer = buffer.slice(idx + 1);
      }
    }
    buffer += decoder.decode();
    if (buffer.trim()) consumeLine(buffer);
    if (!completed) throw new Error('流式响应在完成前中断');
  } finally {
    reader.cancel().catch(() => {});
  }
}

/* ---------- 鉴权 ---------- */

export const authApi = {
  status: () => request('/api/auth/status'),
  login: password => request('/api/auth/login', { method: 'POST', body: { password } }),
  logout: () => request('/api/auth/logout', { method: 'POST' }),
  setPassword: (currentPassword, newPassword) => request('/api/auth/password', {
    method: 'PUT',
    body: { currentPassword, newPassword },
  }),
};

/* ---------- 房间 ---------- */

export const roomApi = {
  list: () => request('/api/rooms'),
  create: body => request('/api/rooms', { method: 'POST', body }),
  update: body => request('/api/rooms', { method: 'PUT', body }),
  remove: id => request('/api/rooms', { method: 'DELETE', params: { room: id } }),
  meta: () => request('/api/rooms/meta'),
};

export const profileApi = {
  list: () => request('/api/profiles'),
  save: (room, name) => request('/api/profiles', { method: 'POST', body: { room, name } }),
  import: profile => request('/api/profiles/import', { method: 'POST', body: profile }),
  apply: (id, name) => request('/api/profiles/apply', { method: 'POST', body: { id, name } }),
  copy: (room, name) => request('/api/rooms/copy', { method: 'POST', body: { room, name } }),
  remove: id => request('/api/profiles', { method: 'DELETE', params: { id } }),
  exportUrl: id => '/api/profiles?id=' + encodeURIComponent(id),
};

export const diagnosticsApi = {
  get: room => request('/api/diagnostics', { params: { room } }),
};

export const notificationApi = {
  get: () => request('/api/notifications'),
  save: config => request('/api/notifications', { method: 'PUT', body: config }),
  test: () => request('/api/notifications/test', { method: 'POST' }),
};

/* ---------- 存档管理（磁盘上的集群目录） ---------- */

export const savesApi = {
  /** 返回 { root, items }：root 是存档根目录绝对路径，删之前可据此手动备份 */
  list: () => request('/api/saves'),
  remove: cluster => request('/api/saves', { method: 'DELETE', params: { cluster } }),
};

/* ---------- 房间管理员（adminlist.txt） ---------- */
export const adminApi = {
  list: roomId => request('/api/admins', { params: { room: roomId } }),
  add: (roomId, kleiId) => request('/api/admins', { method: 'POST', params: { room: roomId }, body: { kleiId } }),
  remove: (roomId, id) => request('/api/admins', { method: 'DELETE', params: { room: roomId, id } }),
};

/* ---------- 在线玩家 ---------- */

export const playerListApi = {
  list: (room, kind) => request('/api/player-lists', { params: { room, kind } }),
  add: (room, kind, kleiId) => request('/api/player-lists', {
    method: 'POST', params: { room, kind }, body: { kleiId },
  }),
  remove: (room, kind, kleiId) => request('/api/player-lists', {
    method: 'DELETE', params: { room, kind }, body: { kleiId },
  }),
};

export const playerApi = {
  list: roomId => request('/api/players', { params: { room: roomId } }),
  act: (roomId, body) => request('/api/players/action', {
    method: 'POST', params: { room: roomId }, body,
  }),
};

/* ---------- 宿主机资源 ---------- */

export const hostApi = {
  stats: () => request('/api/host'),
};

/* ---------- 进程控制 ---------- */

export const procApi = {
  quick: (room, body) => request('/api/quick-action', { method: 'POST', params: { room }, body }),
  status: roomId => request('/api/status', { params: { room: roomId } }),
  start: (roomId, shard) => request('/api/start', { params: { room: roomId, shard } }),
  stop: (roomId, shard, timeout) => request('/api/stop', {
    params: { room: roomId, shard, timeout },
  }),
  restart: (roomId, shard) => request('/api/restart', { params: { room: roomId, shard } }),
  rollback: (roomId, count) => request('/api/rollback', {
    method: 'POST', params: { room: roomId }, body: { count },
  }),
  cmd: (roomId, shard, cmd) => request('/api/cmd', { params: { room: roomId, shard, cmd } }),
};

/* ---------- 日志 ---------- */

export const logApi = {
  /**
   * 流式读取玩家日志分页（NDJSON）。当服务端或测试替身返回普通 JSON 时自动回落为一次性解析。
   */
  async chatStream(room, shard, { kind = 'all', search = '', page = 1, limit = 10, signal, onMeta, onEntry, onDone } = {}) {
    const url = new URL('/api/chat-log', location.origin);
    const params = { room, shard, kind, search, page, limit, stream: '1' };
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null && v !== '') url.searchParams.set(k, v);
    });

    let res;
    try {
      res = await fetch(url, {
        method: 'GET',
        headers: { Accept: 'application/x-ndjson, application/json' },
        signal,
      });
    } catch (err) {
      if (err?.name === 'AbortError') throw err;
      throw new Error('无法连接面板服务，请确认 dst-windows.exe 正在运行', { cause: err });
    }

    if (res.status === 401) {
      if (unauthorizedHandler) unauthorizedHandler();
      throw makeUnauthorizedError();
    }

    const contentType = (res.headers.get('content-type') || '').toLowerCase();
    if (contentType.includes('application/x-ndjson') && res.body) {
      let meta = null;
      await readNDJSON(res, ev => {
        if (ev.type === 'meta') {
          meta = {
            exists: !!ev.exists,
            truncated: !!ev.truncated,
            modified: ev.modified || '',
            total: ev.total || 0,
            page: ev.page || page,
            pageSize: ev.pageSize || limit,
            count: ev.count || 0,
          };
          onMeta?.(meta);
        } else if (ev.type === 'entry' && ev.entry) {
          onEntry?.(ev.entry);
        } else if (ev.type === 'error') {
          throw new Error(ev.message || '请求失败');
        } else if (ev.type === 'done') {
          onDone?.(ev.total ?? meta?.total ?? 0);
        }
      });
      return;
    }

    let json;
    try {
      json = await res.json();
    } catch (err) {
      throw new Error(`接口返回异常（HTTP ${res.status}）`, { cause: err });
    }
    if (json.code === 401) {
      if (unauthorizedHandler) unauthorizedHandler();
      throw makeUnauthorizedError(json.message);
    }
    if (json.code !== 200) {
      throw new Error(json.message || '请求失败');
    }
    const data = json.data || {};
    const list = data.entries || [];
    onMeta?.({
      exists: !!data.exists,
      truncated: !!data.truncated,
      modified: data.modified || '',
      total: data.total || 0,
      page: data.page || page,
      pageSize: data.pageSize || limit,
      count: list.length,
    });
    for (const item of list) onEntry?.(item);
    onDone?.(data.total || list.length);
  },
  chat: (room, shard, kind = 'all', search = '', page = 1, limit = 10) => request('/api/chat-log', {
    params: { room, shard, kind, search, page, limit },
  }),
  removeChat: (room, shard, ids) => request('/api/chat-log', {
    method: 'DELETE', params: { room, shard }, body: { ids },
  }),
  tail: (roomId, shard, lines) => request('/api/logs', { params: { room: roomId, shard, lines } }),
  /** SSE 只能用 EventSource，因此这里返回地址而不是发起请求。 */
  streamUrl(roomId, shard, lines = 200) {
    const url = new URL('/api/logs/stream', location.origin);
    if (roomId) url.searchParams.set('room', roomId);
    if (shard) url.searchParams.set('shard', shard);
    url.searchParams.set('lines', lines);
    return url.toString();
  },
};

/* ---------- 世界配置 ---------- */

export const worldApi = {
  get: (roomId, shard) => request('/api/worldconfig', { params: { room: roomId, shard } }),
  save: payload => request('/api/worldconfig', {
    method: 'PUT', params: { room: payload.room, shard: payload.shard }, body: payload,
  }),
};

/* ---------- 服务器文件安装 ---------- */

export const installApi = {
  status: () => request('/api/install'),
  start: () => request('/api/install', { params: { action: 'start' } }),
  cancel: () => request('/api/install', { method: 'DELETE' }),
  update: () => request('/api/install/update'),
};

/* ---------- 服务器文件位置（可切换安装目录与架构） ---------- */

export const serverApi = {
  get: () => request('/api/server'),
  // serverDir 传空字符串表示「保持/恢复自动探测」
  update: (serverDir, arch) => request('/api/server', {
    method: 'PUT',
    body: { serverDir, arch },
  }),};

/* ---------- 模组 ---------- */

export const modApi = {
  list: (roomId, shard) => request('/api/mods', { params: { room: roomId, shard } }),
  /**
   * 流式读取模组列表（NDJSON）。当服务端或测试替身返回普通 JSON 时自动回落为一次性解析。
   */
  async listStream(roomId, shard, { signal, onMeta, onMod, onDone } = {}) {
    const url = new URL('/api/mods', location.origin);
    if (roomId !== undefined && roomId !== null && roomId !== '') url.searchParams.set('room', roomId);
    if (shard !== undefined && shard !== null && shard !== '') url.searchParams.set('shard', shard);
    url.searchParams.set('stream', '1');

    let res;
    try {
      res = await fetch(url, {
        method: 'GET',
        headers: { Accept: 'application/x-ndjson, application/json' },
        signal,
      });
    } catch (err) {
      if (err?.name === 'AbortError') throw err;
      throw new Error('无法连接面板服务，请确认 dst-windows.exe 正在运行', { cause: err });
    }

    if (res.status === 401) {
      if (unauthorizedHandler) unauthorizedHandler();
      throw makeUnauthorizedError();
    }

    const contentType = (res.headers.get('content-type') || '').toLowerCase();
    if (contentType.includes('application/x-ndjson') && res.body) {
      let emitted = 0;
      await readNDJSON(res, ev => {
        if (ev.type === 'meta') onMeta?.(ev.total || 0);
        else if (ev.type === 'mod' && ev.mod) {
          emitted++;
          onMod?.(ev.mod);
        } else if (ev.type === 'error') {
          throw new Error(ev.message || '请求失败');
        } else if (ev.type === 'done') {
          onDone?.(ev.total ?? emitted);
        }
      });
      return;
    }

    let json;
    try {
      json = await res.json();
    } catch (err) {
      throw new Error(`接口返回异常（HTTP ${res.status}）`, { cause: err });
    }
    if (json.code === 401) {
      if (unauthorizedHandler) unauthorizedHandler();
      throw makeUnauthorizedError(json.message);
    }
    if (json.code !== 200) {
      throw new Error(json.message || '请求失败');
    }
    const list = json.data || [];
    onMeta?.(list.length);
    for (const item of list) onMod?.(item);
    onDone?.(list.length);
  },
  toggle: (roomId, id, enabled, shard) => request('/api/mods/toggle', {
    params: { room: roomId, id, enabled, shard },
  }),
  setConfig: (roomId, id, key, value, shard) => request('/api/mods/config', {
    params: { room: roomId, id, key, value, shard },
  }),
  schema: (roomId, id) => request('/api/mods/schema', { params: { room: roomId, id } }),
  remove: (roomId, id) => request('/api/mods/delete', { params: { room: roomId, id } }),
};

/* ---------- 模组预更新（下载创意工坊模组） ---------- */

export const modUpdateApi = {
  /** 查询进度：{ running, progress, error, output } */
  status: () => request('/api/mods/update'),
  /**
   * 触发一次预更新，立即返回，之后轮询 status。
   * id 传工坊模组 ID 时为单模组模式（已安装的会先删旧文件再重下），
   * 不传则全量补齐所有缺失模组。
   */
  start: (roomId, shard, id) => {
    const params = { room: roomId, shard };
    if (id) params.id = id;
    // 必须显式 POST：request() 默认 GET，而 GET 这个端点只是查状态——
    // 少了 method 会导致「点下载实际只查了一次状态」，静默失败且无任何报错
    return request('/api/mods/update', { method: 'POST', params });
  },
  stop: () => request('/api/mods/update', { method: 'DELETE' }),

  /**
   * 检查房间启用的工坊模组有没有新版。
   * 返回 { checked, updates: [{id,name,localTime,remoteTime}], missing }
   */
  check: (roomId, shard) => request('/api/mods/updates', { params: { room: roomId, shard } }),
};

/* ---------- 创意工坊 ---------- */

export const workshopApi = {
  /** 按 ID / 链接查详情（免 key）。input 支持纯数字、workshop-数字、链接、逗号分隔 */
  details: input => request('/api/workshop/details', { params: { input } }),
  /** 关键词搜索（需要 Steam Web API Key）。page 从 1 开始 */
  search: (q, page = 1) => request('/api/workshop/search', { params: { q, page } }),
  getKey: () => request('/api/workshop/key'),
  saveKey: steamApiKey => request('/api/workshop/key', { method: 'PUT', body: { steamApiKey } }),
};

/* ---------- 备份 ---------- */

export const backupApi = {
  list: roomId => request('/api/backups', { params: { room: roomId } }),
  create: roomId => request('/api/backups/create', { params: { room: roomId } }),
  remove: (roomId, name) => request('/api/backups/delete', { params: { room: roomId, name } }),
  restore: (roomId, name) => request('/api/backups/restore', { params: { room: roomId, name } }),
  inspect: (roomId, name) => request('/api/backups/check', { method: 'POST', params: { room: roomId }, body: { name } }),
  check: id => request('/api/backups/check', { params: { id } }),
  clearCheck: id => request('/api/backups/check', { method: 'DELETE', params: { id } }),
  checkMap: (id, shard) => request('/api/backups/check/map', { params: { id, shard } }),
  restoreChecked: (roomId, id, sha256) => request('/api/backups/restore', { method: 'POST', params: { room: roomId }, body: { id, sha256 } }),
  /**
   * 导出备份到浏览器。走原生 <a download> 而不是 request()：
   * 响应体是 zip 文件流而非 JSON，且浏览器自带的下载管理比手动拼 blob 更省事。
   */
  download(roomId, name) {
    const url = new URL('/api/backups/download', location.origin);
    url.searchParams.set('room', roomId);
    url.searchParams.set('name', name);
    const a = document.createElement('a');
    a.href = url.toString();
    a.download = name;
    document.body.appendChild(a);
    a.click();
    a.remove();
  },
};

/* ---------- 存档上传 ---------- */

export const saveApi = {
  /**
   * 上传存档整包（Klei 下载或别的面板导出的 Cluster_* 压缩包）。
   *
   * 用 XHR 而不是 request()：存档动辄几百 MB，必须能报告上传进度，
   * 而 fetch 没有上传进度事件；XHR 的 upload.onprogress 是现成手段。
   * 另外上传 + 服务端解压拷贝可能耗时数分钟，也不能套用 request() 的 60s 超时。
   *
   * @param {File} file
   * @param {{mode: 'new'|'overwrite', roomId?: number}} options
   * @param {(percent: number) => void} [onProgress] 上传进度回调
   * @returns {Promise<{room: object, summary: object}>}
   */
  upload(file, { mode, roomId }, onProgress) {
    return new Promise((resolve, reject) => {
      const form = new FormData();
      form.append('file', file);
      form.append('mode', mode);
      if (roomId) form.append('room', String(roomId));

      const xhr = new XMLHttpRequest();
      xhr.open('POST', '/api/saves/upload');
      // 传输 + 解压 + 拷贝整条链路都可能很慢，给足 30 分钟
      xhr.timeout = 30 * 60 * 1000;

      xhr.upload.onprogress = e => {
        if (onProgress && e.lengthComputable) {
          onProgress(Math.round((e.loaded / e.total) * 100));
        }
      };
      xhr.onload = () => {
        let json;
        try {
          json = JSON.parse(xhr.responseText);
        } catch (_) {
          reject(new Error(`接口返回异常（HTTP ${xhr.status}）`));
          return;
        }
        if (xhr.status === 401 || json.code === 401) {
          if (unauthorizedHandler) unauthorizedHandler();
          reject(makeUnauthorizedError(json.message));
          return;
        }
        if (json.code !== 200) {
          reject(new Error(json.message || '上传失败'));
          return;
        }
        resolve(json.data);
      };
      xhr.onerror = () => reject(new Error('无法连接面板服务，请确认 dst-windows.exe 正在运行'));
      xhr.ontimeout = () => reject(new Error('上传超时，请检查网络或改用更小的存档包'));
      xhr.send(form);
    });
  },
};

/* ---------- 配置预览 ---------- */

export const configApi = {
  preview: roomId => request('/api/config', { params: { room: roomId } }),
};

export { request };

export const automationApi = {
  status: () => request('/api/automation'),
  save: config => request('/api/automation', { method: 'PUT', body: config }),
};

export const worldStateApi = {
  get: roomId => request('/api/worldstate', { params: { room: roomId } }),
};

export const backupRemoteApi = {
  get: () => request('/api/backup-remote'),
  save: config => request('/api/backup-remote', { method: 'PUT', body: config }),
  test: () => request('/api/backup-remote/test', { method: 'POST' }),
  sync: (room, name) => request('/api/backup-remote/sync', { method: 'POST', params: { room }, body: { name } }),
};

export const migrationApi = {
  export: () => request('/api/migration/export', { method: 'POST' }),
  upload: file => request('/api/migration/upload', { method: 'POST', rawBody: file, timeoutMs: 30 * 60 * 1000 }),
  status: id => request('/api/migration/job', { params: { id } }),
  remove: id => request('/api/migration/job', { method: 'DELETE', params: { id } }),
  apply: (id, applyPlan) => request('/api/migration/apply', { method: 'POST', body: { id, applyPlan } }),
  downloadUrl: id => '/api/migration/download?id=' + encodeURIComponent(id),
};

export const mapApi = {
  get: (room, shard) => request('/api/map', { params: { room, shard } }),
  textureUrl: tile => '/api/map/texture?tile=' + encodeURIComponent(tile),
  iconUrl: kind => '/api/map/icon?kind=' + encodeURIComponent(kind),
};
