import { reactive } from 'vue';

export const confirmState = reactive({
  open: false,
  title: '确认操作',
  description: '',
  confirmText: '确定',
  cancelText: '取消',
  destructive: false,
  resolve: null,
});

/**
 * 弹出基于 shadcn AlertDialog 的通用异步确认框，替代原生 window.confirm()。
 * @param {string | { title: string, description?: string, confirmText?: string, cancelText?: string, destructive?: boolean }} input
 * @param {object} [options]
 * @returns {Promise<boolean>}
 */
export function confirmAction(input, options = {}) {
  let title = '确认操作';
  let description = '';

  if (typeof input === 'string') {
    const parts = input.split(/\n\n+/);
    title = parts[0];
    description = parts.slice(1).join('\n\n');
  } else if (input && typeof input === 'object') {
    title = input.title || title;
    description = input.description || description;
    options = { ...input, ...options };
  }

  return new Promise((resolve) => {
    confirmState.title = title;
    confirmState.description = description;
    confirmState.confirmText = options.confirmText || '确定';
    confirmState.cancelText = options.cancelText || '取消';
    confirmState.destructive = options.destructive ?? (
      /删除|停止|覆盖|回滚|卸载|移除|重新下载/.test(title)
    );
    confirmState.resolve = resolve;
    confirmState.open = true;
  });
}

export function handleConfirm() {
  confirmState.open = false;
  if (confirmState.resolve) {
    confirmState.resolve(true);
    confirmState.resolve = null;
  }
}

export function handleCancel() {
  confirmState.open = false;
  if (confirmState.resolve) {
    confirmState.resolve(false);
    confirmState.resolve = null;
  }
}
