// 主题：浅色 / 深色 / 跟随系统。
//
// 首屏主题已在 index.html 的内联脚本里定好（防止闪屏），
// 这里借助 VueUse 管理持久化、系统偏好监听与无过渡切换。

import { computed, watch } from 'vue';
import { useColorMode, useStorage } from '@vueuse/core';

const KEY = 'dst-theme';
const MODES = ['auto', 'light', 'dark'];

export const MODE_LABEL = { auto: '跟随系统', light: '浅色', dark: '深色' };

const storage = typeof localStorage !== 'undefined' ? localStorage : undefined;
const storageWindow = typeof window !== 'undefined' && typeof window.dispatchEvent === 'function' ? window : undefined;
const mode = useStorage(KEY, 'auto', storage, {
  flush: 'sync',
  window: storageWindow,
  listenToStorageChanges: !!storageWindow,
  writeDefaults: false,
  onError: () => {},
  serializer: {
    read: v => (MODES.includes(v) ? v : 'auto'),
    write: v => v,
  },
});

let defaultApply = null;

function syncTheme(next) {
  if (typeof document === 'undefined' || !document.documentElement) return;
  const el = document.documentElement;
  if (el.dataset.theme === next && el.dataset.themeMode === mode.value) return;
  el.dataset.themeMode = mode.value;
  if (defaultApply && typeof el.setAttribute === 'function' && typeof window?.document?.querySelector === 'function') {
    defaultApply(next);
  } else {
    el.dataset.theme = next;
  }
}

const colorMode = useColorMode({
  attribute: 'data-theme',
  storageRef: mode,
  disableTransition: true,
  onChanged(next, defaultHandler) {
    defaultApply = defaultHandler;
    syncTheme(next);
  },
});

/** 实际生效的黑白（auto 会跟随系统）。 */
export const resolved = computed(() => colorMode.state.value);

watch([mode, resolved], ([, next]) => syncTheme(next), { flush: 'sync' });

function set(next) {
  if (!MODES.includes(next)) return;
  mode.value = next;
}

function cycle() {
  set(MODES[(MODES.indexOf(mode.value) + 1) % MODES.length]);
}

/** 供入口挂载前确保 DOM 属性与当前主题同步（可重复调用）。 */
export function initTheme() {
  syncTheme(resolved.value);
}

export function useTheme() {
  return {
    mode,
    resolved,
    label: computed(() => MODE_LABEL[mode.value]),
    nextLabel: computed(() => MODE_LABEL[MODES[(MODES.indexOf(mode.value) + 1) % MODES.length]]),
    cycle,
    set,
  };
}
