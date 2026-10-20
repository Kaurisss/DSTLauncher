// 保留原有 toast / guard 调用契约，渲染统一交给根节点的 Sonner。
import { toast as notify } from 'vue-sonner';

/** @param {''|'err'|'warn'} kind */
export function toast(message, kind = '') {
  const options = { duration: kind === 'err' ? 6000 : 3200 };
  if (kind === 'err') return notify.error(message, options);
  if (kind === 'warn') return notify.warning(message, options);
  return notify.success(message, options);
}

/**
 * 执行一个可能抛错的异步动作，失败时自动弹错误提示。
 *
 * 视图里大量「调用 -> 捕获 -> 提示」都是同一套写法，收敛到这里：
 *   const { ok } = await guard(() => rooms.start(shard), { success: '启动指令已发送' });
 *   // ok 为 false 时不要继续后续步骤（例如关闭编辑表单）
 */
export async function guard(fn, { success = '', kind = '' } = {}) {
  try {
    const data = await fn();
    if (success) toast(success, kind);
    return { ok: true, data };
  } catch (err) {
    toast(err.message, 'err');
    return { ok: false, error: err };
  }
}
