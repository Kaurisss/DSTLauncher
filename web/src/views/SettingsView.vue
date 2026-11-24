<script setup>
import { onMounted, ref } from 'vue';

import InstallPanel from '@/components/overview/InstallPanel.vue';
import AutomationPanel from '@/components/AutomationPanel.vue';
import NotificationSettings from '@/components/NotificationSettings.vue';
import BackupRemoteSettings from '@/components/BackupRemoteSettings.vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiField from '@/components/ui/UiField.vue';
import UiPill from '@/components/ui/UiPill.vue';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { useTheme } from '@/composables/useTheme.js';
import { hostApi, workshopApi } from '@/lib/api.js';
import { playerLogRefresh } from '@/lib/playerLogRefresh.js';
import { guard } from '@/lib/toast.js';
import { useAuthStore } from '@/stores/auth.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';

const auth = useAuthStore();
const theme = useTheme();
const rooms = useRoomsStore();
const system = useSystemStore();

const section = ref('automation');
const groups = [
  { label: '维护', items: [{ key: 'automation', label: '自动维护' }, { key: 'remote-backup', label: '异地备份' }, { key: 'notifications', label: '事件通知' }] },
  { label: '安全', items: [{ key: 'password', label: '访问密码' }] },
  { label: '创意工坊', items: [{ key: 'workshop', label: 'API Key' }] },
  { label: '服务器', items: [{ key: 'server', label: '文件与架构' }] },
  { label: '界面', items: [
    { key: 'theme', label: '主题' },
    { key: 'logs', label: '玩家日志' },
    { key: 'about', label: '运行信息' },
  ] },
];

const currentPwd = ref('');
const newPwd = ref('');
const savingPwd = ref(false);

const apiKey = ref('');
const savingKey = ref(false);
const saveDir = ref('');

onMounted(async () => {
  auth.load();
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
  <div class="grid items-start gap-4 md:grid-cols-[11rem_minmax(0,1fr)]">
    <nav aria-label="设置导航" class="flex flex-col gap-4">
      <div v-for="group in groups" :key="group.label" class="grid gap-1">
        <p class="px-2 text-xs font-semibold text-muted-foreground">{{ group.label }}</p>
        <button
          v-for="item in group.items"
          :key="item.key"
          type="button"
          class="rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted"
          :class="{ 'bg-muted font-medium': section === item.key }"
          :aria-current="section === item.key ? 'page' : undefined"
          @click="section = item.key"
        >
          {{ item.label }}
        </button>
      </div>
    </nav>

    <div class="min-w-0">
      <AutomationPanel v-show="section === 'automation'" />
      <NotificationSettings v-if="section === 'notifications'" />
      <BackupRemoteSettings v-if="section === 'remote-backup'" />

      <UiCard v-show="section === 'password'">
        <template #title>访问密码</template>
        <template #actions>
          <UiPill :tone="auth.enabled ? 'ok' : ''">
            {{ auth.envManaged ? '启动参数锁定' : (auth.enabled ? '已启用' : '未启用') }}
          </UiPill>
          <IconAction v-if="auth.enabled" variant="destructive" icon="logout" label="退出登录" @click="logout" />
          <IconAction v-if="!auth.envManaged" type="submit" form="settings-password-form" icon="save" :label="savingPwd ? '保存中…' : '保存密码'" :disabled="savingPwd || (!auth.enabled && !newPwd.trim()) || (auth.enabled && !currentPwd)" />
        </template>

        <p v-if="auth.envManaged" class="text-sm text-muted-foreground">
          当前密码由启动参数（<code>-password</code>）或环境变量（<code>DST_PANEL_PASSWORD</code>）指定。
        </p>

        <form v-else id="settings-password-form" class="grid max-w-md gap-4" @submit.prevent="savePassword">
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
        </form>
      </UiCard>

      <UiCard v-show="section === 'workshop'">
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
          <IconAction type="submit" form="settings-api-key-form" icon="save" :label="savingKey ? '保存中…' : '保存 Key'" :disabled="savingKey" />
        </template>

        <form id="settings-api-key-form" class="grid max-w-md gap-4" @submit.prevent="saveApiKey">
          <UiField label="Steam Web API Key" for-id="set-steam-key" block>
            <Input
              id="set-steam-key"
              v-model="apiKey"
              type="password"
              autocomplete="off"
              placeholder="用于创意工坊关键词搜索；按 ID 查询与下载无需配置"
            />
          </UiField>
        </form>
      </UiCard>

      <UiCard v-show="section === 'server'">
        <template #title>服务器文件与架构</template>
        <InstallPanel />
      </UiCard>

      <UiCard v-show="section === 'theme'">
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

      <UiCard v-show="section === 'logs'">
        <template #title>玩家日志</template>
        <div class="flex items-center justify-between gap-4">
          <div class="grid gap-1">
            <label for="player-log-refresh" class="text-sm">自动刷新</label>
            <p class="text-sm text-muted-foreground">打开玩家日志后，每 5 秒更新当前房间、分片和筛选下的记录。</p>
          </div>
          <Switch id="player-log-refresh" v-model="playerLogRefresh" aria-label="自动刷新玩家日志" />
        </div>
      </UiCard>

      <UiCard v-show="section === 'about'">
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
    </div>
  </div>
</template>
