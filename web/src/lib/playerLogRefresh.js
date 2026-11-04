import { ref, watch } from 'vue';

const KEY = 'dst-log-refresh';

function read() {
  try {
    return localStorage.getItem(KEY) !== '0';
  } catch {
    return true;
  }
}

export const playerLogRefresh = ref(read());

watch(playerLogRefresh, value => {
  try {
    localStorage.setItem(KEY, value ? '1' : '0');
  } catch {
    // 隐私模式写不进本地存储时，本次打开仍然生效。
  }
});
