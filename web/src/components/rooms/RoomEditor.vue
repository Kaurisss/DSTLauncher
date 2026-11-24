<script setup>
// 房间编辑表单。
//
// 房间配置是全部 ini 的唯一事实来源，所以这里覆盖了 cluster.ini 与 server.ini
// 的所有可配置项。表单状态是本地副本，只有点保存才写回后端。
// 切换编辑对象时由父组件用 :key 重建本组件，因此不需要 watch 同步。

import { computed, reactive, ref } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiField from '@/components/ui/UiField.vue';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import Icon from '@/components/ui/Icon.vue';
import ShardEditor from '@/components/rooms/ShardEditor.vue';
import IconAction from '@/components/ui/IconAction.vue';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Input } from '@/components/ui/input';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard, toast } from '@/lib/toast.js';
import { GAME_MODE_LABEL, TICK_RATE_LABEL, LANGUAGE_LABEL } from '@/lib/format.js';

const props = defineProps({
  /** null 表示新建 */
  room: { type: Object, default: null },
  meta: { type: Object, default: null },
});

const emit = defineEmits(['close', 'saved']);

const rooms = useRoomsStore();
const isNew = computed(() => !props.room);

/** 把后端的分片对象补齐成表单需要的形状（端口 null 表示「待分配」）。 */
function normalizeShard(s, i) {
  return {
    name: s.name || `World${i + 1}`,
    worldType: s.worldType || 'forest',
    isMaster: !!s.isMaster,
    disabled: !!s.disabled,
    gameId: s.gameId || 0,
    serverPort: s.serverPort || null,
    masterServerPort: s.masterServerPort || null,
    authenticationPort: s.authenticationPort || null,
    isNew: false,
  };
}

/**
 * 分片初值：已有房间用房间配置，新建房间用后端的默认值（Master + Caves）。
 * 两个分片都要保留——停用的分片端口也不会被回收，之后再启用不必重新分配。
 */
function initialShards(src, d) {
  const list = (src.shards && src.shards.length) ? src.shards : (d.shards || []);
  if (!list.length) {
    return [normalizeShard({ name: 'Master', worldType: 'forest', isMaster: true, gameId: 1 })];
  }
  return list.map(normalizeShard);
}

function buildForm() {
  const d = (props.meta && props.meta.defaults) || {};
  const src = props.room || {};

  return {
    id: src.id || 0,
    name: src.name ?? d.name ?? '我的饥荒服务器',
    description: src.description ?? d.description ?? '',
    gameMode: src.gameMode || d.gameMode || 'endless',
    maxPlayers: src.maxPlayers || d.maxPlayers || 12,
    tickRate: src.tickRate || d.tickRate || 30,
    language: src.language || d.language || 'zh',
    password: src.password || '',
    token: src.token || '',
    maxSnapshots: src.maxSnapshots || d.maxSnapshots || 10,
    masterIp: src.masterIp || d.masterIp || '127.0.0.1',
    masterPort: src.masterPort || null,
    clusterKey: src.clusterKey || '',

    // 布尔值用 ?? 而不是 ||，否则用户主动关掉的开关会被默认值顶回来
    pvp: src.pvp ?? true,
    pauseEmpty: src.pauseEmpty ?? true,
    voteEnabled: src.voteEnabled ?? true,
    voteKick: src.voteKick ?? true,
    lanOnly: src.lanOnly ?? false,
    offline: src.offline ?? false,

    shards: initialShards(src, d),
  };
}

const form = reactive(buildForm());
const saving = ref(false);
const saveError = ref('');
defineExpose({ saving });

const gameModes = computed(() => (props.meta && props.meta.gameModes) || Object.keys(GAME_MODE_LABEL));
const tickRates = computed(() => (props.meta && props.meta.tickRates) || [15, 30, 60]);
const languages = computed(() => (props.meta && props.meta.languages) || Object.keys(LANGUAGE_LABEL));

const behaviorFlags = [
  { key: 'pvp', label: 'PVP' },
  { key: 'pauseEmpty', label: '无人时暂停' },
  { key: 'voteEnabled', label: '投票' },
  { key: 'voteKick', label: '投票踢人' },
  { key: 'lanOnly', label: '仅局域网' },
  { key: 'offline', label: '离线模式' },
];

async function pasteToken() {
  if (!window.navigator.clipboard?.readText) {
    toast('当前连接不支持读取剪贴板，请直接粘贴到令牌输入框', 'warn');
    return;
  }
  const { ok, data } = await guard(() => window.navigator.clipboard.readText());
  if (ok) form.token = data.trim();
}

/** 数字框清空后会是空字符串，必须转成后端能接的整数。 */
function toNum(value, fallback) {
  const n = Number(value);
  return Number.isFinite(n) && n > 0 ? Math.round(n) : fallback;
}

async function save() {
  if (saving.value) return;
  saveError.value = '';
  saving.value = true;

  try {
    const payload = {
      id: form.id,
      name: form.name.trim(),
      description: form.description.trim(),
      gameMode: form.gameMode,
      maxPlayers: toNum(form.maxPlayers, 12),
      tickRate: toNum(form.tickRate, 30),
      language: form.language,
      password: form.password,
      token: form.token.trim(),
      maxSnapshots: toNum(form.maxSnapshots, 10),
      masterIp: form.masterIp.trim(),
      masterPort: toNum(form.masterPort, 0),
      clusterKey: form.clusterKey.trim(),
      pvp: !!form.pvp,
      pauseEmpty: !!form.pauseEmpty,
      voteEnabled: !!form.voteEnabled,
      voteKick: !!form.voteKick,
      lanOnly: !!form.lanOnly,
      offline: !!form.offline,

      // 分片：名字/类型/启停/世界 id 由用户决定，端口留 0 表示交给后端统一分配
      shards: form.shards.map(s => ({
        name: s.name.trim(),
        worldType: s.worldType,
        isMaster: !!s.isMaster,
        disabled: !!s.disabled,
        gameId: toNum(s.gameId, 0),
        serverPort: toNum(s.serverPort, 0),
        masterServerPort: toNum(s.masterServerPort, 0),
        authenticationPort: toNum(s.authenticationPort, 0),
      })),
    };

    const { ok, data, error } = await guard(() =>
      isNew.value ? rooms.create(payload) : rooms.update(payload));
    if (!ok) {
      saveError.value = (error && error.message) || '保存失败';
      return;
    }

    // 房间运行中改配置不会立即生效，后端会带一条提示
    const hint = data && data.hint;
    if (hint) toast(hint, 'warn');
    else toast(isNew.value ? '房间已创建' : '房间已保存');

    emit('saved');
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <UiCard flush fill class="room-editor min-w-0">
    <UiToolbar aria-label="房间编辑工具栏">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button variant="ghost" size="icon-sm" aria-label="返回房间列表" :disabled="saving" @click="emit('close')">
            <Icon name="chevron" class="rotate-180" size="16" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>返回房间列表</TooltipContent>
      </Tooltip>
      <Separator orientation="vertical" class="h-4 data-[orientation=vertical]:self-center" />
      <h2 class="truncate text-sm font-semibold">{{ isNew ? '新建房间' : '编辑房间 · ' + form.name }}</h2>
      <Badge v-if="!isNew" variant="secondary" class="hidden shrink-0 font-mono text-[10px] lg:inline-flex">{{ room.key || 'Cluster_' + form.id }}</Badge>
      <template #actions>
        <IconAction variant="outline" icon="x" label="取消" :disabled="saving" @click="emit('close')" />
        <IconAction icon="save" :label="saving ? '保存中…' : '保存'" :disabled="saving" @click="save" />
      </template>
    </UiToolbar>

    <div data-slot="page-scroll" class="room-form grid min-h-0 flex-1 content-start gap-5 overflow-y-auto overscroll-contain p-[var(--panel-padding)]" tabindex="0" role="region" aria-label="房间编辑内容">
      <Alert v-if="saveError" variant="destructive">
        <AlertTitle>保存失败</AlertTitle>
        <AlertDescription>{{ saveError }}</AlertDescription>
      </Alert>

      <section class="grid gap-4" aria-label="基本信息">
        <h3 class="text-xs font-semibold text-muted-foreground">基本信息</h3>
        <div class="room-core-grid grid gap-4 md:grid-cols-12">
          <UiField label="房间名" for-id="f_name" block class="min-w-0 md:col-span-4">
            <Input id="f_name" v-model="form.name" type="text" placeholder="显示在服务器列表里的名字" />
          </UiField>
          <UiField label="描述" for-id="f_description" block class="min-w-0 md:col-span-5">
            <Input id="f_description" v-model="form.description" type="text" />
          </UiField>
          <UiField label="房间密码" for-id="f_password" block class="min-w-0 md:col-span-3">
            <Input id="f_password" v-model="form.password" type="text" placeholder="留空表示无密码" />
          </UiField>
        </div>
        <div class="room-rules-grid grid grid-cols-2 gap-4 lg:grid-cols-4">
          <UiField label="游戏模式" for-id="f_gameMode" block class="min-w-0">
            <Select v-model="form.gameMode">
              <SelectTrigger id="f_gameMode" class="w-full"><SelectValue :placeholder="GAME_MODE_LABEL[form.gameMode] || form.gameMode" /></SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectGroup><SelectItem v-for="m in gameModes" :key="m" :value="m">{{ GAME_MODE_LABEL[m] || m }}</SelectItem></SelectGroup>
              </SelectContent>
            </Select>
          </UiField>
          <UiField label="最大人数" for-id="f_maxPlayers" block class="min-w-0">
            <Input id="f_maxPlayers" v-model.number="form.maxPlayers" type="number" min="1" max="64" />
          </UiField>
          <UiField label="语言" for-id="f_language" block class="min-w-0">
            <Select v-model="form.language">
              <SelectTrigger id="f_language" class="w-full"><SelectValue :placeholder="LANGUAGE_LABEL[form.language] || form.language" /></SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectGroup><SelectItem v-for="l in languages" :key="l" :value="l">{{ LANGUAGE_LABEL[l] || l }}</SelectItem></SelectGroup>
              </SelectContent>
            </Select>
          </UiField>
          <UiField label="回档快照上限" for-id="f_maxSnapshots" block class="min-w-0">
            <Input id="f_maxSnapshots" v-model.number="form.maxSnapshots" type="number" min="1" max="100" />
          </UiField>
        </div>
      </section>

      <UiField label="Klei 服务器令牌" for-id="f_token" block :hint="form.lanOnly || form.offline ? '局域网或离线模式可留空' : '在线开服必需，写入 cluster_token.txt'">
        <div class="flex min-w-0 items-center gap-1 rounded-lg border border-input pr-1 focus-within:ring-1 focus-within:ring-ring">
          <Input id="f_token" v-model="form.token" type="password" autocomplete="off" spellcheck="false" class="min-w-0 flex-1 border-0 bg-transparent font-mono shadow-none focus-visible:ring-0" placeholder="pds-g^..." />
          <IconAction type="button" variant="ghost" icon="clipboard" label="粘贴" :disabled="saving" @click="pasteToken" />
          <IconAction as="a" href="https://accounts.klei.com/account/game/servers?game=DontStarveTogether" target="_blank" rel="noopener noreferrer" variant="ghost" class="text-muted-foreground" icon="external" label="前往 Klei 获取令牌（新标签页）" />
        </div>
      </UiField>

      <section class="grid gap-3" aria-label="行为开关">
        <h3 class="text-xs font-semibold text-muted-foreground">行为开关</h3>
        <div class="room-switch-grid grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
          <label v-for="flag in behaviorFlags" :key="flag.key" :for="'f_' + flag.key" class="flex cursor-pointer select-none items-center justify-between gap-2 rounded-lg border border-border bg-muted/20 p-3 text-sm has-data-[state=checked]:border-primary/40 has-data-[state=checked]:bg-primary/5">
            <span>{{ flag.label }}</span>
            <Switch :id="'f_' + flag.key" v-model="form[flag.key]" :aria-label="flag.label" />
          </label>
        </div>
      </section>

      <ShardEditor v-model:shards="form.shards" :room-is-new="isNew" />

      <details class="details">
        <summary>高级设置</summary>
        <div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <UiField label="tick 频率" for-id="f_tickRate" block class="min-w-0">
            <Select :model-value="String(form.tickRate)" @update:model-value="v => form.tickRate = Number(v)">
              <SelectTrigger id="f_tickRate" class="w-full"><SelectValue :placeholder="TICK_RATE_LABEL[form.tickRate] || String(form.tickRate)" /></SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectGroup><SelectItem v-for="t in tickRates" :key="t" :value="String(t)">{{ TICK_RATE_LABEL[t] || t }}</SelectItem></SelectGroup>
              </SelectContent>
            </Select>
          </UiField>
          <UiField label="主分片 IP" for-id="f_masterIp" block class="min-w-0">
            <Input id="f_masterIp" v-model="form.masterIp" type="text" />
          </UiField>
          <UiField label="分片协调端口" for-id="f_masterPort" block class="min-w-0">
            <Input id="f_masterPort" v-model.number="form.masterPort" type="number" min="1024" max="65535" />
          </UiField>
          <UiField label="通信密钥" for-id="f_clusterKey" block class="min-w-0 sm:col-span-2 lg:col-span-3" hint="启用分片必须一致，改动后需重启全部分片">
            <Input id="f_clusterKey" v-model="form.clusterKey" type="text" />
          </UiField>
        </div>
      </details>
      <div v-if="$slots.default" class="stack"><slot /></div>
    </div>
  </UiCard>
</template>
