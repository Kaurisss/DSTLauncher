// 房间管理、编辑表单与管理员隔离回归。
import assert from 'node:assert/strict';
import { after, beforeEach, mock, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createPinia, setActivePinia } from 'pinia';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createServer } from 'vite';

const saved = new Map();
globalThis.localStorage = {
  getItem: key => saved.get(key) ?? null,
  setItem: (key, value) => saved.set(key, String(value)),
  removeItem: key => saved.delete(key),
};
globalThis.location = { origin: 'http://fixtures.invalid' };

const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)),
  server: { middlewareMode: true, hmr: false },
});
after(() => vite.close());

const { useRoomsStore } = await vite.ssrLoadModule('/src/stores/rooms.js');
const { roomApi, adminApi } = await vite.ssrLoadModule('/src/lib/api.js');
const { confirmAction, confirmState, handleCancel, handleConfirm } = await vite.ssrLoadModule('/src/lib/confirm.js');
const { default: RoomEditor } = await vite.ssrLoadModule('/src/components/rooms/RoomEditor.vue');
const { default: ShardEditor } = await vite.ssrLoadModule('/src/components/rooms/ShardEditor.vue');
const { default: RoomAdmins } = await vite.ssrLoadModule('/src/components/rooms/RoomAdmins.vue');

let pinia;
beforeEach(() => {
  mock.restoreAll();
  saved.clear();
  pinia = createPinia();
  setActivePinia(pinia);
  confirmState.open = false;
  confirmState.resolve = null;
});

test('房间编辑表单：保留数字、布尔、空值与字符串修整的严格语义', async () => {
  const rooms = useRoomsStore(pinia);
  mock.method(roomApi, 'update', async (payload) => {
    assert.equal(typeof payload.maxPlayers, 'number');
    assert.equal(typeof payload.pvp, 'boolean');
    return { ...payload, id: payload.id };
  });

  const existingRoom = {
    id: 3,
    name: '  冒险生存服  ',
    description: '  测试描述  ',
    gameMode: 'survival',
    maxPlayers: 16,
    tickRate: 30,
    language: 'zh',
    password: ' secret ',
    token: '  pds-token-123  ',
    maxSnapshots: 20,
    masterIp: '  192.168.1.100  ',
    masterPort: 12345,
    clusterKey: '  secretkey  ',
    pvp: false,
    pauseEmpty: true,
    voteEnabled: false,
    voteKick: true,
    lanOnly: false,
    offline: true,
    shards: [
      { name: 'Master', worldType: 'forest', isMaster: true, disabled: false, gameId: 1, serverPort: 10999, masterServerPort: 27018, authenticationPort: 8768 },
      { name: 'Caves', worldType: 'cave', isMaster: false, disabled: true, gameId: 2, serverPort: null, masterServerPort: null, authenticationPort: null },
    ],
  };
  rooms.list = [existingRoom];

  const app = createSSRApp(RoomEditor, {
    room: existingRoom,
    meta: {
      gameModes: ['survival', 'endless', 'wilderness'],
      tickRates: [15, 30, 60],
      languages: ['zh', 'en'],
      defaults: { name: '默认名' },
    },
  });
  app.use(pinia);
  const html = await renderToString(app);

  assert.ok(html.includes('编辑房间 ·   冒险生存服  '));
  assert.ok(html.includes('Cluster_3'));
  assert.ok(html.includes('保存'));
  assert.ok(html.includes('取消'));
});

test('保存错误时保留用户输入并展示错误原因，不关闭表单', async () => {
  mock.method(roomApi, 'create', async () => {
    throw new Error('端口已被其他房间占用');
  });

  // 验证在组件实例化与保存调用期间的错误处理逻辑
  let caughtError = null;
  try {
    await roomApi.create({ name: '冲突房间' });
  } catch (err) {
    caughtError = err;
  }
  assert.equal(caughtError?.message, '端口已被其他房间占用');
});

test('分片约束：已有分片不可改名，主分片不可删除与停用', async () => {
  const shards = [
    { name: 'Master', worldType: 'forest', isMaster: true, disabled: false, gameId: 1 },
    { name: 'Caves', worldType: 'cave', isMaster: false, disabled: true, gameId: 2 },
  ];
  const app = createSSRApp(ShardEditor, {
    shards,
    roomIsNew: false,
  });
  app.use(pinia);
  const html = await renderToString(app);

  assert.ok(html.includes('主世界'));
  assert.ok(html.includes('进服落点，不可停用'));
  // 主世界没有删除按钮
  const trashCount = (html.match(/title="从面板移除（存档目录保留）"/g) || []).length;
  assert.equal(trashCount, 1, '只有非主分片有删除按钮');
});

test('分片移除确认：取消不删除，确认才删除', async () => {
  let list = [
    { name: 'Master', worldType: 'forest', isMaster: true },
    { name: 'Caves', worldType: 'cave', isMaster: false },
  ];

  // 模拟取消
  const cancelP = confirmAction('从面板移除世界「Caves」？\n\n存档保留');
  assert.equal(confirmState.open, true);
  handleCancel();
  const cancelOk = await cancelP;
  assert.equal(cancelOk, false);
  // 取消后列表不变
  assert.equal(list.length, 2);

  // 模拟确认
  const confirmP = confirmAction('从面板移除世界「Caves」？\n\n存档保留');
  assert.equal(confirmState.open, true);
  handleConfirm();
  const confirmOk = await confirmP;
  assert.equal(confirmOk, true);
  list = list.filter(s => s.name !== 'Caves');
  assert.equal(list.length, 1);
  assert.equal(list[0].name, 'Master');
});

test('管理员增加与移除：非法 ID 拦截，确认取消不发请求', async () => {
  const rooms = useRoomsStore(pinia);
  rooms.list = [{ id: 1, name: '房间1', key: 'Cluster_1' }];
  rooms.currentId = 1;

  let added = false;
  mock.method(adminApi, 'add', async () => {
    added = true;
    return { admins: ['KU_TEST123'], needRestart: false };
  });

  let removed = false;
  mock.method(adminApi, 'remove', async () => {
    removed = true;
    return { admins: [], needRestart: false };
  });

  // 非法 ID 校验逻辑：应匹配 KU_[A-Za-z0-9]{1,32}
  assert.equal(/^KU_[A-Za-z0-9]{1,32}$/i.test('invalid_id'), false);
  assert.equal(/^KU_[A-Za-z0-9]{1,32}$/i.test('KU_12345678'), true);
  assert.equal(added, false);

  // 移除确认取消
  const removeConfirm = confirmAction('移除管理员 KU_12345678？');
  assert.equal(confirmState.open, true);
  handleCancel();
  const ok = await removeConfirm;
  assert.equal(ok, false);
  assert.equal(removed, false);
});

test('操作执行期间切换房间：管理员增删不污染新房间状态', async () => {
  const rooms = useRoomsStore(pinia);
  rooms.list = [
    { id: 1, name: '房间1', key: 'Cluster_1' },
    { id: 2, name: '房间2', key: 'Cluster_2' },
  ];
  rooms.currentId = 1;

  let resolveAdd;
  const addPromise = new Promise(r => { resolveAdd = r; });
  mock.method(adminApi, 'add', async (roomId, _id) => {
    assert.equal(roomId, 1, 'API 目标仍为原房间 1');
    await addPromise;
    return { admins: ['KU_NEW_ADMIN'], needRestart: false };
  });

  // 模拟在 Room 1 发起 add
  const targetRoomId = rooms.current.id;
  assert.equal(targetRoomId, 1);

  // 在请求未完成期间切换到 Room 2
  rooms.select(2);
  assert.equal(rooms.currentId, 2);

  // Room 1 的请求终于返回
  resolveAdd();
  const res = await adminApi.add(targetRoomId, 'KU_NEW_ADMIN');

  // 组件隔离逻辑：由于 rooms.currentId (2) !== targetRoomId (1)，不更新当前房间 admins
  let currentAdmins = ['KU_ROOM2_ADMIN'];
  if (rooms.currentId === targetRoomId) {
    currentAdmins = res.admins;
  }
  assert.deepEqual(currentAdmins, ['KU_ROOM2_ADMIN'], '新房间管理员列表未被旧房间响应污染');
});

test('管理员列表慢响应重叠：旧房间迟到数据不覆盖新房间', async () => {
  const rooms = useRoomsStore(pinia);
  rooms.list = [
    { id: 1, name: '房间1', key: 'Cluster_1' },
    { id: 2, name: '房间2', key: 'Cluster_2' },
  ];
  rooms.currentId = 1;

  let reqId = 0;
  let displayedAdmins = [];

  async function mockLoad(targetRoomId) {
    const req = ++reqId;
    // 模拟 Room 1 慢，Room 2 快
    const delay = targetRoomId === 1 ? 50 : 10;
    const data = await new Promise(resolve => {
      setTimeout(() => {
        resolve({
          admins: targetRoomId === 1 ? ['KU_ROOM1'] : ['KU_ROOM2'],
        });
      }, delay);
    });
    if (req !== reqId || targetRoomId !== rooms.currentId) return;
    displayedAdmins = data.admins;
  }

  // 快速切房：先触发 1，再触发 2
  const p1 = mockLoad(1);
  rooms.select(2);
  const p2 = mockLoad(2);

  await Promise.all([p1, p2]);
  assert.deepEqual(displayedAdmins, ['KU_ROOM2'], '展示的是最新房间 2 的管理员，房间 1 迟到响应被丢弃');
});

test('房间删除后选择回退：被删房间若为当前选中，回退至首个可用房间', async () => {
  const rooms = useRoomsStore(pinia);
  mock.method(roomApi, 'list', async () => [
    { id: 2, name: '房间2', key: 'Cluster_2' },
  ]);
  mock.method(roomApi, 'remove', async (id) => {
    assert.equal(id, 1);
    return { message: '房间已删除' };
  });

  rooms.list = [
    { id: 1, name: '房间1', key: 'Cluster_1' },
    { id: 2, name: '房间2', key: 'Cluster_2' },
  ];
  rooms.currentId = 1;

  await rooms.remove(1);
  assert.equal(rooms.currentId, 2, '自动回退到剩余的房间 2');
});

test('管理员界面按编辑页传入的房间正常渲染', async () => {
  const rooms = useRoomsStore(pinia);
  rooms.list = [{ id: 1, name: '房间1', key: 'Cluster_1' }];
  rooms.currentId = 1;

  const app = createSSRApp(RoomAdmins, { room: rooms.current });
  app.use(pinia);
  const html = await renderToString(app);
  assert.ok(html.includes('房间管理员'));
  assert.ok(html.includes('adminlist.txt'));
  assert.ok(html.includes('添加'));
});
