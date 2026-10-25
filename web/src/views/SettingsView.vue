<script setup>
import { onMounted, ref } from 'vue';

import InstallPanel from '@/components/overview/InstallPanel.vue';
import AutomationPanel from '@/components/AutomationPanel.vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiField from '@/components/ui/UiField.vue';
import UiPill from '@/components/ui/UiPill.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useTheme } from '@/composables/useTheme.js';
import { workshopApi } from '@/lib/api.js';
import { guard } from '@/lib/toast.js';
import { useAuthStore } from '@/stores/auth.js';

const auth = useAuthStore();
const theme = useTheme();

const currentPwd = ref('');
const newPwd = ref('');
const savingPwd = ref(false);

const apiKey = ref('');
const savingKey = ref(false);

onMounted(async () => {
  auth.load();
  const { ok, data } = await guard(() => workshopApi.getKey());
  if (ok && data) apiKey.value = data.steamApiKey || '';
});

async function savePassword() {
  if (savingPwd.value) return;
  savingPwd.value = true;
  const { ok } = await guard(
    () => auth.setPassword(currentPwd.value, newPwd.value),
    { success: newPwd.value.trim() ? '面板密码已保存' : '已关闭密码保护' },
  );
  savingPwd.value = false;
  if (ok) {
    currentPwd.value = '';
    newPwd.value = '';
  }
}

async function logout() {
  await guard(() => auth.logout(), { success: '已退出登录' });
}

async function saveApiKey() {
  if (savingKey.value) return;
  savingKey.value = true;
  const val = apiKey.value.trim();
  await guard(
    () => workshopApi.saveKey(val),
    { success: val ? 'Steam API Key 已保存' : 'Steam API Key 已清空' },
  );
  savingKey.value = false;
}
</script>

<template>
  <div class="stack">
    <AutomationPanel />
    <UiCard>
      <template #title>访问密码</template>
      <template #actions>
        <UiPill :tone="auth.enabled ? 'ok' : ''">
          {{ auth.envManaged ? '启动参数锁定' : (auth.enabled ? '已启用' : '未启用') }}
        </UiPill>
        <Button v-if="auth.enabled" variant="destructive" size="sm" @click="logout">
          退出登录
        </Button>
      </template>

      <p v-if="auth.envManaged" class="text-sm text-muted-foreground">
        当前密码由启动参数（<code>-password</code>）或环境变量（<code>DST_PANEL_PASSWORD</code>）指定。
      </p>

      <form v-else class="grid max-w-md gap-4" @submit.prevent="savePassword">
        <UiField v-if="auth.enabled" label="当前密码" for-id="set-cur-pwd" block>
          <Input
            id="set-cur-pwd"
            v-model="currentPwd"
            type="password"
            autocomplete="current-password"
            placeholder="输入当前密码"
          />
        </UiField>
        <UiField :label="auth.enabled ? '新密码' : '访问密码'" for-id="set-new-pwd" block>
          <Input
            id="set-new-pwd"
            v-model="newPwd"
            type="password"
            autocomplete="new-password"
            :placeholder="auth.enabled ? '至少 4 个字符；留空保存则关闭密码' : '至少 4 个字符'"
          />
        </UiField>
        <div class="flex items-center gap-2">
          <Button
            type="submit"
            size="sm"
            :disabled="savingPwd || (!auth.enabled && !newPwd.trim()) || (auth.enabled && !currentPwd)"
          >
            {{ savingPwd ? '保存中…' : '保存密码' }}
          </Button>
        </div>
      </form>
    </UiCard>

    <UiCard>
      <template #title>创意工坊 API Key</template>
      <template #actions>
        <a
          href="https://steamcommunity.com/dev/apikey"
          target="_blank"
          rel="noopener noreferrer"
          class="text-xs text-primary hover:underline"
        >
          申请 Steam Web API Key
        </a>
      </template>

      <form class="grid max-w-md gap-4" @submit.prevent="saveApiKey">
        <UiField label="Steam Web API Key" for-id="set-steam-key" block>
          <Input
            id="set-steam-key"
            v-model="apiKey"
            type="password"
            autocomplete="off"
            placeholder="用于创意工坊关键词搜索；按 ID 查询与下载无需配置"
          />
        </UiField>
        <div class="flex items-center gap-2">
          <Button type="submit" size="sm" :disabled="savingKey">
            {{ savingKey ? '保存中…' : '保存 Key' }}
          </Button>
        </div>
      </form>
    </UiCard>

    <UiCard>
      <template #title>服务器文件与架构</template>
      <InstallPanel />
    </UiCard>

    <UiCard>
      <template #title>界面主题</template>
      <div class="seg">
        <button
          v-for="opt in [
            { key: 'auto', label: '跟随系统' },
            { key: 'light', label: '浅色' },
            { key: 'dark', label: '深色' },
          ]"
          :key="opt.key"
          type="button"
          class="seg-btn"
          :class="{ 'is-active': theme.mode.value === opt.key }"
          :aria-pressed="theme.mode.value === opt.key"
          @click="theme.set(opt.key)"
        >
          {{ opt.label }}
        </button>
      </div>
    </UiCard>
  </div>
</template>
