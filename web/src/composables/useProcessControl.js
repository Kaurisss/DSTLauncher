// 进程启停的公共动作。
//
// 概览页的卡片头部与分片行都要用到，且确认话术必须一致，因此收敛在这里。
// 这里包含 confirm() —— 它属于界面行为，但两个调用点需要完全相同的交互，
// 分散到各处反而容易写歪。

import { useRoomsStore } from '@/stores/rooms.js';
import { guard } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';

export function useProcessControl() {
  const rooms = useRoomsStore();

  /** 执行动作并在成功后立刻刷新状态（不等 3 秒轮询）。 */
  async function run(fn, success) {
    const { ok } = await guard(fn, { success });
    if (ok) rooms.refreshStatus().catch(() => {});
    return ok;
  }

  const SCOPE_ALL = '全部';
  const label = shard => shard || SCOPE_ALL;

  return {
    /** shard 传空字符串表示操作全部分片 */
    start(shard) {
      return run(() => rooms.start(shard), `${label(shard)}分片启动指令已发送`);
    },

    async stop(shard) {
      const ok = await confirmAction({
        title: `停止${label(shard)}分片？`,
        description: '将先自动保存存档。',
        confirmText: '停止分片',
        destructive: true,
      });
      if (!ok) return false;
      // 停服要等 c_save + c_shutdown，全部分片要给足够长的等待
      return run(() => rooms.stop(shard, shard ? 40 : 60), `${label(shard)}分片已停止`);
    },

    async restart(shard) {
      const ok = await confirmAction({
        title: `重启 ${shard}？`,
        description: '将先自动保存存档。',
        confirmText: '重启分片',
        destructive: true,
      });
      if (!ok) return false;
      return run(() => rooms.restart(shard), `${shard} 已重启`);
    },
  };
}
