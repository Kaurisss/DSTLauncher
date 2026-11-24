import assert from 'node:assert/strict';
import { afterEach, test } from 'node:test';

globalThis.location = { origin: 'http://panel.test' };
const { logApi, modApi, onUnauthorized } = await import('../src/lib/api.js');

const originalFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = originalFetch;
  onUnauthorized(null);
});

function responseFromChunks(chunks, contentType = 'application/x-ndjson') {
  const encoder = new globalThis.TextEncoder();
  return new globalThis.Response(new globalThis.ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
      controller.close();
    },
  }), { headers: { 'Content-Type': contentType } });
}

test('NDJSON 跨 chunk 拆行后仍按顺序完成模组读取', async () => {
  globalThis.fetch = async () => responseFromChunks([
    '{"type":"meta","total":1}\n{"type":"mod","mod":{"id":"workshop-1"',
    ',"name":"测试模组"}}\n{"type":"done","total":1}\n',
  ]);
  const mods = [];
  let total = 0;
  await modApi.listStream(1, 'Master', {
    onMod: mod => mods.push(mod),
    onDone: value => { total = value; },
  });
  assert.deepEqual(mods, [{ id: 'workshop-1', name: '测试模组' }]);
  assert.equal(total, 1);
});

test('普通 JSON 回退仍然提供相同回调', async () => {
  globalThis.fetch = async () => new globalThis.Response(JSON.stringify({
    code: 200,
    data: { entries: [{ id: 7 }], total: 1, page: 1, pageSize: 10 },
  }), { headers: { 'Content-Type': 'application/json' } });
  const entries = [];
  await logApi.chatStream(1, 'Master', { onEntry: entry => entries.push(entry) });
  assert.deepEqual(entries, [{ id: 7 }]);
});

test('玩家日志也能读取跨 chunk 的 NDJSON', async () => {
  globalThis.fetch = async () => responseFromChunks([
    '{"type":"meta","exists":true,"total":1}\n{"type":"entry","entry":{"id":8',
    ',"message":"流式日志"}}\n{"type":"done","total":1}\n',
  ]);
  const entries = [];
  let total = 0;
  await logApi.chatStream(1, 'Master', {
    onEntry: entry => entries.push(entry),
    onDone: value => { total = value; },
  });
  assert.deepEqual(entries, [{ id: 8, message: '流式日志' }]);
  assert.equal(total, 1);
});

test('401 会触发统一鉴权回调', async () => {
  let called = 0;
  onUnauthorized(() => { called++; });
  globalThis.fetch = async () => new globalThis.Response('', { status: 401 });
  await assert.rejects(() => modApi.listStream(1, 'Master'), error => error.unauthorized === true);
  assert.equal(called, 1);
});

test('服务端 error 事件会失败', async () => {
  globalThis.fetch = async () => responseFromChunks([
    '{"type":"error","message":"读取失败"}\n',
  ]);
  await assert.rejects(() => modApi.listStream(1, 'Master'), /读取失败/);
});

test('缺少 done 的截断流会失败', async () => {
  globalThis.fetch = async () => responseFromChunks([
    '{"type":"meta","total":2}\n{"type":"mod","mod":{"id":"workshop-1"}}\n',
  ]);
  await assert.rejects(() => modApi.listStream(1, 'Master'), /流式响应在完成前中断/);
});
