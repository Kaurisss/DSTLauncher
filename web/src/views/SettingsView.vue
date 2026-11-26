<script setup>
import { computed, onMounted, ref } from 'vue';

import InstallPanel from '@/components/overview/InstallPanel.vue';
import AutomationPanel from '@/components/AutomationPanel.vue';
import NotificationSettings from '@/components/NotificationSettings.vue';
import BackupRemoteSettings from '@/components/BackupRemoteSettings.vue';
import UiCard from '@/components/ui/UiCard.vue';
import { Badge } from '@/components/ui/badge';
import { Field, FieldLabel } from '@/components/ui/field';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { useTheme } from '@/composables/useTheme.js';
import { hostApi, workshopApi } from '@/lib/api.js';
import { guard } from '@/lib/toast.js';
import { useAuthStore } from '@/stores/auth.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { useUiStore } from '@/stores/ui.js';

const auth = useAuthStore();
const theme = useTheme();
const rooms = useRoomsStore();
const system = useSystemStore();
const ui = useUiStore();

const sections = [
  { key: 'maintenance', label: '自动维护与异地备份', desc: '定时备份、更新计划与异地同步' },
  { key: 'notifications', label: '事件通知', desc: 'Webhook 告警与测试' },
  { key: 'server', label: '服务器与创意工坊', desc: '服务端安装路径、架构与 API Key' },
  { key: 'system', label: '安全、外观与关于', desc: '登录凭据、界面主题与运行信息' },
];
const validKeys = new Set(sections.map(s => s.key));
const section = computed({
  get: () => (validKeys.has(ui.settingsSection) ? ui.settingsSection : 'maintenance'),
  set: value => { ui.settingsSection = value; },
});

const currentPwd = ref('');
const newPwd = ref('');
const username = ref('admin');
const savingPwd = ref(false);

const apiKey = ref('');
const savingKey = ref(false);
const saveDir = ref('');

onMounted(async () => {
  const authStatus = await auth.load();
  if (authStatus?.username) username.value = authStatus.username;
  rooms.loadMeta().catch(() => {});
  system.loadServer().catch(() => {});
  hostApi.stats().then(data => { saveDir.value = data?.diskPath || ''; }).catch(() => {});
  const { ok, data } = await guard(() => workshopApi.getKey());
  if (ok && data) apiKey.value = data.steamApiKey || '';
});

async function savePassword() {
  if (savingPwd.value) return;
  savingPwd.value = true;
  const { ok } = await guard(
    () => auth.setPassword(username.value, currentPwd.value, newPwd.value),
    { success: newPwd.value.trim() ? '登录凭据已保存' : '已关闭密码保护' },
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
  <div class="grid items-start gap-4 md:grid-cols-[12.5rem_minmax(0,1fr)]">
    <nav aria-label="设置导航" class="grid gap-1">
      <button
        v-for="item in sections"
        :key="item.key"
        type="button"
        class="rounded-md px-2.5 py-2 text-left text-sm transition-colors hover:bg-muted"
        :class="{ 'bg-muted font-medium': section === item.key }"
        :aria-current="section === item.key ? 'page' : undefined"
        @click="section = item.key"
      >
        {{ item.label }}
      </button>
    </nav>

    <div class="grid min-w-0 gap-4">
      <template v-if="section === 'maintenance'">
        <AutomationPanel />
        <BackupRemoteSettings />
      </template>

      <NotificationSettings v-else-if="section === 'notifications'" />

      <template v-else-if="section === 'server'">
        <UiCard>
          <template #title>服务器文件与架构</template>
          <InstallPanel />
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
              申请 Key
            </a>
            <IconAction type="submit" form="settings-api-key-form" show-label icon="save" :label="savingKey ? '保存中…' : '保存 Key'" :disabled="savingKey" />
          </template>

          <form id="settings-api-key-form" class="grid max-w-md gap-4" @submit.prevent="saveApiKey">
            <Field>
              <FieldLabel for="set-steam-key">Steam Web API Key</FieldLabel>
              <Input
                id="set-steam-key"
                v-model="apiKey"
                type="password"
                autocomplete="off"
                placeholder="用于创意工坊关键词搜索；按 ID 查询与下载无需配置"
              />
            </Field>
          </form>
        </UiCard>
      </template>

      <template v-else>
        <UiCard>
          <template #title>登录凭据</template>
          <template #actions>
            <Badge :variant="auth.enabled ? 'ok' : 'muted'">
              {{ auth.envManaged ? '启动参数锁定' : (auth.enabled ? '已启用' : '未启用') }}
            </Badge>
            <IconAction v-if="auth.enabled" variant="destructive" show-label icon="logout" label="退出登录" @click="logout" />
            <IconAction v-if="!auth.envManaged" type="submit" form="settings-password-form" show-label icon="save" :label="savingPwd ? '保存中…' : '保存凭据'" :disabled="savingPwd || !username.trim() || (!auth.enabled && !newPwd.trim()) || (auth.enabled && !currentPwd)" />
          </template>

          <p v-if="auth.envManaged" class="text-sm text-muted-foreground">
            当前用户名或密码由启动参数（<code>-username</code>、<code>-password</code>）或环境变量（<code>DST_PANEL_USERNAME</code>、<code>DST_PANEL_PASSWORD</code>）指定。
          </p>

          <form v-else id="settings-password-form" class="grid max-w-md gap-4" @submit.prevent="savePassword">
            <Field>
              <FieldLabel for="set-username">用户名</FieldLabel>
              <Input
                id="set-username"
                v-model="username"
                autocomplete="username"
              />
            </Field>
            <Field v-if="auth.enabled">
              <FieldLabel for="set-cur-pwd">当前密码</FieldLabel>
              <Input
                id="set-cur-pwd"
                v-model="currentPwd"
                type="password"
                autocomplete="current-password"
              />
            </Field>
            <Field>
              <FieldLabel for="set-new-pwd">{{ auth.enabled ? '新密码' : '访问密码' }}</FieldLabel>
              <Input
                id="set-new-pwd"
                v-model="newPwd"
                type="password"
                autocomplete="new-password"
                :placeholder="auth.enabled ? '至少 4 个字符；留空保存则关闭密码' : '至少 4 个字符'"
              />
            </Field>
          </form>
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

        <UiCard>
          <template #title>运行信息</template>
          <dl class="grid gap-3 text-sm">
            <div class="grid gap-1">
              <dt class="text-muted-foreground">面板版本</dt>
              <dd>{{ rooms.meta?.panelVersion || '未知' }}</dd>
            </div>
            <div class="grid gap-1">
              <dt class="text-muted-foreground">面板配置</dt>
              <dd class="break-all font-mono text-xs">{{ system.server?.configPath || '—' }}</dd>
            </div>
            <div class="grid gap-1">
              <dt class="text-muted-foreground">存档目录</dt>
              <dd class="break-all font-mono text-xs">{{ saveDir || '—' }}</dd>
            </div>
            <div class="grid gap-1">
              <dt class="text-muted-foreground">服务器目录</dt>
              <dd class="break-all font-mono text-xs">{{ system.server?.root || '未安装' }}</dd>
            </div>
            <div class="grid gap-1">
              <dt class="text-muted-foreground">模组目录</dt>
              <dd class="break-all font-mono text-xs">{{ system.server?.ugcDir || '—' }}</dd>
            </div>
          </dl>
          <p class="mt-4 text-sm text-muted-foreground">监听地址和端口由启动参数决定，改完需要重新打开面板。</p>
        </UiCard>
      </template>
    </div>
  </div>
</template>
