<script setup>
// 房间编辑表单。
//
// 房间配置是全部 ini 的唯一事实来源，所以这里覆盖了 cluster.ini 与 server.ini
// 的所有可配置项。表单状态是本地副本，只有点保存才写回后端。
// 切换编辑对象时由父组件用 :key 重建本组件，因此不需要 watch 同步。

import { computed, reactive, ref, useSlots } from 'vue';
import {
  PhEye, PhEyeSlash, PhGear, PhInfo, PhKey, PhSlidersHorizontal, PhStack, PhUsers,
} from '@phosphor-icons/vue';

import UiCard from '@/components/ui/UiCard.vue';
import { Field, FieldLabel } from '@/components/ui/field';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Separator } from '@/components/ui/separator';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import Icon from '@/components/ui/Icon.vue';
import ShardEditor from '@/components/rooms/ShardEditor.vue';
import ConfigPreviewDialog from '@/components/config/ConfigPreviewDialog.vue';
import IconAction from '@/components/ui/IconAction.vue';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Input } from '@/components/ui/input';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';
import { useSlidingIndicator } from '@/composables/useSlidingIndicator.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard, toast } from '@/lib/toast.js';
import { GAME_MODE_LABEL, TICK_RATE_LABEL, LANGUAGE_LABEL } from '@/lib/format.js';

const props = defineProps({
  /** null 表示新建 */
  room: { type: Object, default: null },
  meta: { type: Object, default: null },
  initialSection: { type: String, default: 'general' },
});

const emit = defineEmits(['close', 'saved']);

const rooms = useRoomsStore();
const slots = useSlots();
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
const initialSnapshot = JSON.stringify(form);
const saving = ref(false);
const saveError = ref('');
const showToken = ref(false);
const scrollPane = ref(null);
const sectionNavRef = ref(null);
const shardEditorRef = ref(null);
defineExpose({ saving });

const hasAccessSlot = computed(() => !isNew.value && !!slots.access);
const formSections = [
  { key: 'general', label: '基础与规则', desc: '房间名称、玩法参数与游戏行为开关', icon: PhSlidersHorizontal },
  { key: 'network', label: '联机与令牌', desc: '在线认证令牌、局域网与离线模式设置', icon: PhKey },
  { key: 'shards', label: '世界分片', desc: '配置房间的世界进程、端口分配与启停状态', icon: PhStack },
  { key: 'advanced', label: '高级网络', desc: '服务端 Tick 频率与多世界分片通信参数', icon: PhGear },
];
const accessSection = {
  key: 'access',
  label: '管理员与名单',
  desc: '维护房间管理员、黑名单与白名单（直接写入磁盘文件）',
  icon: PhUsers,
};

const validSectionKeys = computed(() => [
  ...formSections.map(s => s.key),
  ...(hasAccessSlot.value ? [accessSection.key] : []),
]);

const activeSection = ref(
  ['general', 'network', 'shards', 'advanced', 'access'].includes(props.initialSection)
    ? props.initialSection
    : 'general',
);

const currentSection = computed(() => (
  validSectionKeys.value.includes(activeSection.value) ? activeSection.value : 'general'
));
const activeSectionMeta = computed(() => (
  currentSection.value === accessSection.key
    ? accessSection
    : formSections.find(s => s.key === currentSection.value) || formSections[0]
));

const isDirty = computed(() => !isNew.value && JSON.stringify(form) !== initialSnapshot);
const tokenOptional = computed(() => form.lanOnly || form.offline);
const tokenMissing = computed(() => !tokenOptional.value && !form.token.trim());
const enabledShardCount = computed(() => form.shards.filter(s => !s.disabled).length);
const sectionIndicator = useSlidingIndicator(
  sectionNavRef,
  'button[aria-current="page"]',
  () => [currentSection.value, hasAccessSlot.value, tokenMissing.value, enabledShardCount.value, form.shards.length],
);

const gameModes = computed(() => (props.meta && props.meta.gameModes) || Object.keys(GAME_MODE_LABEL));
const tickRates = computed(() => (props.meta && props.meta.tickRates) || [15, 30, 60]);
const languages = computed(() => (props.meta && props.meta.languages) || Object.keys(LANGUAGE_LABEL));

const gameplayFlags = [
  { key: 'pvp', label: 'PVP 伤害', desc: '允许玩家之间互相攻击与造成伤害' },
  { key: 'pauseEmpty', label: '无人时暂停', desc: '房间内没有在线玩家时自动暂停世界时间' },
  { key: 'voteEnabled', label: '投票功能', desc: '允许在线玩家发起重置或回档等投票' },
  { key: 'voteKick', label: '投票踢人', desc: '允许玩家发起投票将指定成员移出房间' },
];

const networkFlags = [
  { key: 'lanOnly', label: '仅局域网', desc: '仅限同一局域网内玩家发现与加入，无需 Klei 令牌' },
  { key: 'offline', label: '离线模式', desc: '不连接 Klei 大厅服务器，允许离线开服，无需令牌' },
];

function selectSection(key) {
  activeSection.value = key;
}

function onSectionBeforeEnter() {
  scrollPane.value?.scrollTo({ top: 0 });
}

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
    <UiToolbar aria-label="房间编辑工具栏" class="border-b border-border/75">
      <Tooltip>
        <TooltipTrigger as-child>
          <Button variant="ghost" size="icon-sm" aria-label="返回房间列表" :disabled="saving" @click="emit('close')">
            <Icon name="chevron" class="rotate-180" size="16" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>返回房间列表</TooltipContent>
      </Tooltip>
      <Separator orientation="vertical" class="h-4 self-center" />
      <h2 class="truncate text-sm font-semibold">{{ isNew ? '新建房间' : '编辑房间：' + form.name }}</h2>
      <Badge v-if="!isNew" variant="secondary" class="hidden shrink-0 font-mono text-[10px] lg:inline-flex">
        {{ room.key || 'Cluster_' + form.id }}
      </Badge>
      <Badge
        v-if="isDirty"
        variant="outline"
        class="hidden shrink-0 border-amber-500/40 bg-amber-500/10 text-[11px] font-normal text-amber-700 dark:text-amber-400 sm:inline-flex"
      >
        未保存更改
      </Badge>
      <template #actions>
        <ConfigPreviewDialog v-if="!isNew" :room="room" show-label="sm" />
        <IconAction variant="outline" show-label icon="x" label="取消" :disabled="saving" @click="emit('close')" />
        <IconAction show-label icon="save" :label="saving ? '保存中…' : '保存'" :disabled="saving" @click="save" />
      </template>
    </UiToolbar>

    <div class="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] overflow-hidden md:grid-cols-[196px_minmax(0,1fr)] md:grid-rows-none">
      <!-- 左侧分区导航（移动端退化为顶部横向滚动胶囊栏） -->
      <nav
        ref="sectionNavRef"
        aria-label="房间配置分组"
        class="relative flex shrink-0 items-center gap-1 overflow-x-auto border-b border-border/75 bg-muted/25 px-3 py-2 md:flex-col md:items-stretch md:overflow-x-visible md:overflow-y-auto md:border-r md:border-b-0 md:px-3 md:py-3"
      >
        <span
          aria-hidden="true"
          class="sliding-nav-indicator pointer-events-none absolute top-0 left-0 z-0 rounded-lg bg-card shadow-2xs ring-1 ring-border/75 dark:bg-accent dark:ring-transparent"
          :class="{ 'is-animated': sectionIndicator.animated.value }"
          :style="sectionIndicator.style.value"
        />

        <span class="relative z-10 hidden px-2.5 pb-1 text-[11px] font-medium tracking-wider text-muted-foreground md:block">
          房间配置
        </span>

        <button
          v-for="item in formSections"
          :key="item.key"
          type="button"
          :aria-current="currentSection === item.key ? 'page' : undefined"
          :class="[
            'relative z-10 flex h-[32px] shrink-0 items-center gap-2 rounded-lg ps-2.5 pe-2.5 text-left text-[13px] transition-colors md:h-[34px] md:w-full',
            currentSection === item.key
              ? (sectionIndicator.ready.value
                ? 'font-medium text-foreground'
                : 'bg-card font-medium text-foreground shadow-2xs ring-1 ring-border/75 dark:bg-accent dark:ring-transparent')
              : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground',
          ]"
          @click="selectSection(item.key)"
        >
          <component :is="item.icon" class="size-4 shrink-0" aria-hidden="true" />
          <span class="truncate">{{ item.label }}</span>
          <span
            v-if="item.key === 'network' && tokenMissing"
            class="ml-auto shrink-0 rounded bg-amber-500/12 px-1.5 py-0.5 text-[10.5px] font-medium leading-none text-amber-700 dark:text-amber-400"
          >
            缺令牌
          </span>
          <span
            v-else-if="item.key === 'shards'"
            class="ml-auto shrink-0 text-[11px] text-muted-foreground tabular-nums"
          >
            {{ enabledShardCount }}/{{ form.shards.length }}
          </span>
        </button>

        <template v-if="hasAccessSlot">
          <div aria-hidden="true" class="relative z-10 mx-1 h-4 w-px shrink-0 bg-border/80 md:mx-0 md:my-2 md:h-px md:w-full" />
          <span class="relative z-10 hidden px-2.5 pb-1 text-[11px] font-medium tracking-wider text-muted-foreground md:block">
            即时写入
          </span>
          <button
            type="button"
            :aria-current="currentSection === accessSection.key ? 'page' : undefined"
            :class="[
              'relative z-10 flex h-[32px] shrink-0 items-center gap-2 rounded-lg ps-2.5 pe-2.5 text-left text-[13px] transition-colors md:h-[34px] md:w-full',
              currentSection === accessSection.key
                ? (sectionIndicator.ready.value
                  ? 'font-medium text-foreground'
                  : 'bg-card font-medium text-foreground shadow-2xs ring-1 ring-border/75 dark:bg-accent dark:ring-transparent')
                : 'text-muted-foreground hover:bg-muted/70 hover:text-foreground',
            ]"
            @click="selectSection(accessSection.key)"
          >
            <component :is="accessSection.icon" class="size-4 shrink-0" aria-hidden="true" />
            <span class="truncate">{{ accessSection.label }}</span>
          </button>
        </template>
      </nav>

      <!-- 右侧内容区 -->
      <div
        ref="scrollPane"
        data-slot="page-scroll"
        class="room-form min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 md:px-6 md:py-5"
        tabindex="0"
        role="region"
        aria-label="房间编辑内容"
      >
        <Alert v-if="saveError" variant="destructive" class="mb-5 w-full max-w-4xl">
          <AlertTitle>保存失败</AlertTitle>
          <AlertDescription>{{ saveError }}</AlertDescription>
        </Alert>

        <Transition name="panel-switch" mode="out-in" @before-enter="onSectionBeforeEnter">
          <div :key="currentSection" :class="['w-full space-y-5', currentSection === 'access' ? 'max-w-5xl' : 'max-w-4xl']">
            <div class="flex flex-wrap items-center justify-between gap-3 border-b border-border/75 pb-3">
              <div class="min-w-0">
                <h3 class="text-[15px] font-semibold text-foreground">{{ activeSectionMeta.label }}</h3>
                <p class="mt-0.5 text-xs text-muted-foreground">{{ activeSectionMeta.desc }}</p>
              </div>
              <IconAction
                v-if="currentSection === 'shards'"
                variant="outline"
                show-label
                icon="plus"
                label="添加世界"
                @click="shardEditorRef?.addShard()"
              />
            </div>

            <!-- 分区 1：基础与规则 -->
            <template v-if="currentSection === 'general'">
              <section class="rounded-xl border border-border/80 bg-card p-4" aria-label="基本信息">
                <h4 class="mb-3.5 text-xs font-semibold text-muted-foreground">基本信息</h4>
                <div class="grid gap-4">
                  <div class="grid gap-4 md:grid-cols-12">
                    <Field class="min-w-0 md:col-span-5">
                      <FieldLabel for="f_name">房间名</FieldLabel>
                      <Input id="f_name" v-model="form.name" type="text" placeholder="显示在服务器列表里的名字" />
                    </Field>
                    <Field class="min-w-0 md:col-span-3">
                      <FieldLabel for="f_password">房间密码</FieldLabel>
                      <Input id="f_password" v-model="form.password" type="text" placeholder="留空表示无密码" />
                    </Field>
                    <Field class="min-w-0 md:col-span-4">
                      <FieldLabel for="f_gameMode">游戏模式</FieldLabel>
                      <Select v-model="form.gameMode">
                        <SelectTrigger id="f_gameMode" class="w-full">
                          <SelectValue :placeholder="GAME_MODE_LABEL[form.gameMode] || form.gameMode" />
                        </SelectTrigger>
                        <SelectContent position="popper" align="start">
                          <SelectGroup>
                            <SelectItem v-for="m in gameModes" :key="m" :value="m">{{ GAME_MODE_LABEL[m] || m }}</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </Field>
                  </div>

                  <Field class="min-w-0">
                    <FieldLabel for="f_description">描述</FieldLabel>
                    <Input id="f_description" v-model="form.description" type="text" placeholder="房间简介、公告或进服须知" />
                  </Field>

                  <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
                    <Field class="min-w-0">
                      <FieldLabel for="f_maxPlayers">最大人数</FieldLabel>
                      <Input id="f_maxPlayers" v-model.number="form.maxPlayers" type="number" min="1" max="64" class="tabular-nums" />
                    </Field>
                    <Field class="min-w-0">
                      <FieldLabel for="f_language">语言</FieldLabel>
                      <Select v-model="form.language">
                        <SelectTrigger id="f_language" class="w-full">
                          <SelectValue :placeholder="LANGUAGE_LABEL[form.language] || form.language" />
                        </SelectTrigger>
                        <SelectContent position="popper" align="start">
                          <SelectGroup>
                            <SelectItem v-for="l in languages" :key="l" :value="l">{{ LANGUAGE_LABEL[l] || l }}</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </Field>
                    <Field class="min-w-0">
                      <FieldLabel for="f_maxSnapshots">回档快照上限</FieldLabel>
                      <Input id="f_maxSnapshots" v-model.number="form.maxSnapshots" type="number" min="1" max="100" class="tabular-nums" />
                    </Field>
                  </div>
                </div>
              </section>

              <section class="rounded-xl border border-border/80 bg-card p-4" aria-label="行为开关">
                <h4 class="mb-3.5 text-xs font-semibold text-muted-foreground">游戏规则开关</h4>
                <div class="grid gap-3 sm:grid-cols-2">
                  <label
                    v-for="flag in gameplayFlags"
                    :key="flag.key"
                    :for="'f_' + flag.key"
                    :class="[
                      'flex cursor-pointer select-none items-center justify-between gap-3 rounded-lg border border-border/80 bg-muted/20 p-3 transition-colors has-data-[state=checked]:border-primary/40 has-data-[state=checked]:bg-primary/5',
                      flag.key === 'voteKick' && !form.voteEnabled ? 'opacity-60' : '',
                    ]"
                  >
                    <span class="min-w-0">
                      <span class="block text-sm font-medium text-foreground">{{ flag.label }}</span>
                      <span class="mt-0.5 block text-xs text-muted-foreground">{{ flag.desc }}</span>
                    </span>
                    <Switch :id="'f_' + flag.key" v-model="form[flag.key]" :aria-label="flag.label" class="shrink-0" />
                  </label>
                </div>
              </section>
            </template>

            <!-- 分区 2：联机与令牌 -->
            <template v-else-if="currentSection === 'network'">
              <section class="rounded-xl border border-border/80 bg-card p-4" aria-label="联机模式">
                <h4 class="mb-3.5 text-xs font-semibold text-muted-foreground">联机模式</h4>
                <div class="grid gap-3 sm:grid-cols-2">
                  <label
                    v-for="flag in networkFlags"
                    :key="flag.key"
                    :for="'f_' + flag.key"
                    class="flex cursor-pointer select-none items-center justify-between gap-3 rounded-lg border border-border/80 bg-muted/20 p-3 transition-colors has-data-[state=checked]:border-primary/40 has-data-[state=checked]:bg-primary/5"
                  >
                    <span class="min-w-0">
                      <span class="block text-sm font-medium text-foreground">{{ flag.label }}</span>
                      <span class="mt-0.5 block text-xs text-muted-foreground">{{ flag.desc }}</span>
                    </span>
                    <Switch :id="'f_' + flag.key" v-model="form[flag.key]" :aria-label="flag.label" class="shrink-0" />
                  </label>
                </div>
              </section>

              <section class="rounded-xl border border-border/80 bg-card p-4" aria-label="Klei 服务器令牌">
                <div class="mb-3.5 flex flex-wrap items-center justify-between gap-2">
                  <h4 class="text-xs font-semibold text-muted-foreground">Klei 服务器令牌</h4>
                  <Badge
                    v-if="tokenOptional"
                    variant="secondary"
                    class="text-[11px] font-normal"
                  >
                    当前模式可留空
                  </Badge>
                  <Badge
                    v-else-if="form.token.trim()"
                    variant="outline"
                    class="border-emerald-500/35 bg-emerald-500/10 text-[11px] font-normal text-emerald-700 dark:text-emerald-400"
                  >
                    已配置
                  </Badge>
                  <Badge
                    v-else
                    variant="outline"
                    class="border-amber-500/40 bg-amber-500/10 text-[11px] font-normal text-amber-700 dark:text-amber-400"
                  >
                    在线开服必需
                  </Badge>
                </div>

                <Field>
                  <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <FieldLabel for="f_token">cluster_token.txt</FieldLabel>
                    <span class="text-[11px] text-muted-foreground">
                      {{ tokenOptional ? '已开启局域网或离线模式，不填令牌也能开服' : '在线开服必需，保存后自动写入房间目录的 cluster_token.txt' }}
                    </span>
                  </div>
                  <div class="flex min-w-0 items-center gap-1 rounded-lg border border-input bg-background pr-1 focus-within:ring-1 focus-within:ring-ring">
                    <Input
                      id="f_token"
                      v-model="form.token"
                      :type="showToken ? 'text' : 'password'"
                      autocomplete="off"
                      spellcheck="false"
                      class="min-w-0 flex-1 border-0 bg-transparent font-mono shadow-none focus-visible:ring-0"
                      placeholder="pds-g^..."
                    />
                    <Tooltip>
                      <TooltipTrigger as-child>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon-sm"
                          class="text-muted-foreground"
                          :aria-label="showToken ? '隐藏令牌' : '显示令牌'"
                          @click="showToken = !showToken"
                        >
                          <component :is="showToken ? PhEyeSlash : PhEye" class="size-4" aria-hidden="true" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{{ showToken ? '隐藏令牌' : '显示令牌' }}</TooltipContent>
                    </Tooltip>
                    <IconAction type="button" variant="ghost" icon="clipboard" label="粘贴" :disabled="saving" @click="pasteToken" />
                    <IconAction
                      as="a"
                      href="https://accounts.klei.com/account/game/servers?game=DontStarveTogether"
                      target="_blank"
                      rel="noopener noreferrer"
                      variant="ghost"
                      class="text-muted-foreground"
                      icon="external"
                      label="前往 Klei 获取令牌（新标签页）"
                    />
                  </div>
                </Field>
              </section>
            </template>

            <!-- 分区 3：世界分片 -->
            <ShardEditor
              v-else-if="currentSection === 'shards'"
              ref="shardEditorRef"
              v-model:shards="form.shards"
              :room-is-new="isNew"
              :locations="room?.locations"
              hide-header
            />

            <!-- 分区 4：高级网络 -->
            <section v-else-if="currentSection === 'advanced'" class="rounded-xl border border-border/80 bg-card p-4" aria-label="高级设置">
              <h4 class="mb-3.5 text-xs font-semibold text-muted-foreground">分片通信与性能</h4>
              <div class="grid gap-4 sm:grid-cols-3">
                <Field class="min-w-0">
                  <FieldLabel for="f_tickRate">tick 频率</FieldLabel>
                  <Select :model-value="String(form.tickRate)" @update:model-value="v => form.tickRate = Number(v)">
                    <SelectTrigger id="f_tickRate" class="w-full">
                      <SelectValue :placeholder="TICK_RATE_LABEL[form.tickRate] || String(form.tickRate)" />
                    </SelectTrigger>
                    <SelectContent position="popper" align="start">
                      <SelectGroup>
                        <SelectItem v-for="t in tickRates" :key="t" :value="String(t)">{{ TICK_RATE_LABEL[t] || t }}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field class="min-w-0">
                  <FieldLabel for="f_masterIp">主分片 IP</FieldLabel>
                  <Input id="f_masterIp" v-model="form.masterIp" type="text" class="font-mono" />
                </Field>
                <Field class="min-w-0">
                  <FieldLabel for="f_masterPort">分片协调端口</FieldLabel>
                  <Input
                    id="f_masterPort"
                    v-model.number="form.masterPort"
                    type="number"
                    min="1024"
                    max="65535"
                    class="font-mono tabular-nums"
                    placeholder="自动分配"
                  />
                </Field>
                <Field class="min-w-0 sm:col-span-3">
                  <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                    <FieldLabel for="f_clusterKey">通信密钥</FieldLabel>
                    <span class="text-[11px] text-muted-foreground">所有启用分片必须一致，留空由系统自动生成；改动后需重启全部分片</span>
                  </div>
                  <Input id="f_clusterKey" v-model="form.clusterKey" type="text" class="font-mono" placeholder="留空自动生成" />
                </Field>
              </div>
            </section>

            <!-- 分区 5：管理员与准入名单（独立于表单保存，即时写入磁盘） -->
            <template v-else-if="currentSection === 'access' && hasAccessSlot">
              <div class="flex items-start gap-2.5 rounded-lg border border-border/80 bg-muted/35 px-3.5 py-2.5 text-xs text-muted-foreground">
                <PhInfo class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <span>以下增删操作会立即写入房间文件，与右上角表单「保存」按钮相互独立；运行中的房间需重启后生效。也可以直接在「玩家管理」中维护当前房间的名单。</span>
              </div>
              <div class="grid items-start gap-4 xl:grid-cols-2">
                <slot name="access" />
              </div>
            </template>
          </div>
        </Transition>
      </div>
    </div>
  </UiCard>
</template>
