import { defineStore } from 'pinia';
import { ref } from 'vue';

import { authApi } from '@/lib/api.js';

export const useAuthStore = defineStore('auth', () => {
  const enabled = ref(false);
  const authenticated = ref(true);
  const username = ref('admin');
  const envManaged = ref(false);

  function apply(st) {
    if (!st || typeof st.enabled !== 'boolean') return;
    enabled.value = !!st.enabled;
    authenticated.value = st.enabled ? !!st.authenticated : true;
    if (typeof st.username === 'string' && st.username.trim()) username.value = st.username;
    envManaged.value = !!st.envManaged;
  }

  function markUnauthorized() {
    enabled.value = true;
    authenticated.value = false;
  }

  async function load() {
    try {
      const status = await authApi.status();
      apply(status);
      return status;
    } catch (_) {
      // 忽略探活失败，交由具体业务请求处理
    }
  }

  async function login(loginUsername, password) {
    const st = await authApi.login(loginUsername, password);
    apply(st);
    return st;
  }

  async function logout() {
    const st = await authApi.logout();
    apply(st);
    return st;
  }

  async function setPassword(loginUsername, currentPassword, newPassword) {
    const st = await authApi.setPassword(loginUsername, currentPassword, newPassword);
    apply(st);
    return st;
  }

  return {
    enabled,
    authenticated,
    username,
    envManaged,
    markUnauthorized,
    load,
    login,
    logout,
    setPassword,
  };
});
