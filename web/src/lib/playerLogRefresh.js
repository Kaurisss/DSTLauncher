import { useStorage } from '@vueuse/core';

const KEY = 'dst-log-refresh';

const storage = typeof localStorage !== 'undefined' ? localStorage : undefined;
const storageWindow = typeof window !== 'undefined' && typeof window.dispatchEvent === 'function' ? window : undefined;

export const playerLogRefresh = useStorage(KEY, true, storage, {
  window: storageWindow,
  listenToStorageChanges: false,
  writeDefaults: false,
  serializer: {
    read: v => v !== '0',
    write: v => (v ? '1' : '0'),
  },
  onError: () => {},
});
