import { defineStore } from 'pinia';
import { ref } from 'vue';

import { authApi } from '@/lib/api.js';

export const useAuthStore = defineStore('auth', () => {
  const enabled = ref(false);
  const authenticated = ref(true);
  const envManaged = ref(false);

  function apply(st) {
    if (!st || typeof st.enabled !== 'boolean') return;
    enabled.value = !!st.enabled;
    authenticated.value = st.enabled ? !!st.authenticated : true;
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

  async function login(password) {
    const st = await authApi.login(password);
    apply(st);
    return st;
  }

  async function logout() {
    const st = await authApi.logout();
    apply(st);
    return st;
  }

  async function setPassword(currentPassword, newPassword) {
    const st = await authApi.setPassword(currentPassword, newPassword);
    apply(st);
    return st;
  }

  return {
    enabled,
    authenticated,
    envManaged,
    markUnauthorized,
    load,
    login,
    logout,
    setPassword,
  };
});
