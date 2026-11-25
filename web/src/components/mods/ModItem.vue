<script setup>
// 单个模组条目：卡片外壳先错峰进场，模组信息或工坊详情到达前内部以骨架占位，就地平滑填充。

import { computed, onUnmounted, ref, watch } from 'vue';

import UiPill from '@/components/ui/UiPill.vue';
import Icon from '@/components/ui/Icon.vue';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import IconAction from '@/components/ui/IconAction.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import ModConfigForm from '@/components/mods/ModConfigForm.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { modApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';
import { fmtUnix } from '@/lib/format.js';

const SKELETON_NAME_WIDTHS = ['w-36', 'w-44', 'w-32', 'w-40', 'w-48', 'w-36', 'w-44', 'w-32'];

const props = defineProps({
  mod: { type: Object, default: null },
  shard: { type: String, required: true },
  enterIndex: { type: Number, default: 0 },
  workshopLoading: { type: Boolean, default: false },
  /** 「检查更新」查到的结果：{ id, name, localTime, remoteTime }，无更新时为 null */
  updateInfo: { type: Object, default: null },
  downloading: { type: Boolean, default: false },
  updating: { type: Boolean, default: false },
});

const emit = defineEmits(['changed', 'update', 'toggled', 'coverNeeded', 'configOpen', 'busy']);

const enterStyle = computed(() => ({
  '--mod-enter-delay': `${Math.min(Math.max(props.enterIndex, 0), 10) * 50}ms`,
}));
const skeletonNameWidth = computed(() => SKELETON_NAME_WIDTHS[Math.max(props.enterIndex, 0) % SKELETON_NAME_WIDTHS.length]);

const rooms = useRoomsStore();
const expanded = ref(false);
const configForm = ref(null);
const toggling = ref(false);
const removing = ref(false);
const confirmingClose = ref(false);
watch(expanded, value => emit('configOpen', value));
watch([toggling, removing], ([toggle, remove]) => emit('busy', toggle || remove));
onUnmounted(() => {
  if (expanded.value) emit('configOpen', false);
  if (toggling.value || removing.value) emit('busy', false);
});

const coverFailed = ref(false);
const iconFailed = ref(false);
watch(() => props.mod?.previewUrl, () => { coverFailed.value = false; });
watch(() => props.mod?.iconUrl, () => { iconFailed.value = false; });
const coverUrl = computed(() => (!iconFailed.value && props.mod?.iconUrl)
  || (!coverFailed.value && props.mod?.previewUrl) || '');
const titlePending = computed(() => !props.mod
  || (props.workshopLoading && (!props.mod.name || props.mod.name === props.mod.id)));
const coverPending = computed(() => !props.mod || (!coverUrl.value && props.workshopLoading));

function onCoverError() {
  if (!props.mod) return;
  if (coverUrl.value === props.mod.iconUrl) {
    iconFailed.value = true;
    emit('coverNeeded');
  } else coverFailed.value = true;
}

const workshopUrl = computed(() => (props.mod && /^workshop-\d+$/.test(props.mod.id))
  ? `https://steamcommunity.com/sharedfiles/filedetails/?id=${props.mod.id.slice('workshop-'.length)}`
  : '');

/** 服务器运行中冻结：模组开关与配置重启后才生效，改了也不生效 */
const frozen = computed(() => rooms.starting || rooms.current?.running
  || rooms.statuses.some(status => ['running', 'starting', 'stopping'].includes(status.state)));

// 开关状态用本地副本而不是直接改 props.mod：
// 一是避免修改父组件数据，二是接口失败时可以回滚显示。
const enabled = ref(!!props.mod?.enabled);
watch(() => props.mod?.enabled, v => { enabled.value = !!v; });

async function onToggle(next) {
  if (!props.mod || toggling.value) return;
  if (frozen.value) {
    toast('服务器运行中，请先关闭服务器再修改模组', 'warn');
    return;
  }
  const room = rooms.current;
  if (!room) return;

  toggling.value = true;
  enabled.value = next; // 先按用户的意图切换，失败再回滚
  const { ok } = await guard(() => modApi.toggle(room.id, props.mod.id, next, props.shard));
  toggling.value = false;
  if (ok) {
    emit('toggled', next);
    emit('changed');
    toast(`${props.mod.name || props.mod.id} ${next ? '已启用' : '已禁用'}`);
    return;
  }
  enabled.value = !!props.mod.enabled;
}

async function remove() {
  if (!props.mod || removing.value || props.updating) return;
  if (frozen.value) {
    toast('服务器运行中，请先关闭服务器再删除模组', 'warn');
    return;
  }
  const room = rooms.current;
  if (!room) return;
  removing.value = true;
  try {
    const name = props.mod.name || props.mod.id;
    if (!await confirmAction(
      `删除模组「${name}」？\n\n` +
      '模组文件与其它房间共享，使用 Steam 工坊目录时也会影响游戏客户端；删除后需要重新下载。' +
      '会从当前房间所有分片的 modoverrides 中移除该模组记录。',
    )) return;

    const { ok } = await guard(() => modApi.remove(room.id, props.mod.id));
    if (ok) {
      toast('已删除');
      emit('changed');
    }
  } finally {
    removing.value = false;
  }
}

/**
 * 单模组更新：未下载的直接下载；已下载的备份旧文件后重新下载，失败恢复。
 * 创意工坊模组才支持，本地模组没有「下载」一说。
 * 有会话在跑时后端会排队串行下载，这里不需要因「更新中」而禁用。
 * 确认与下载队列由父组件统一处理。
 */
async function updateOne() {
  if (!props.mod || frozen.value) return;
  if (!props.mod.id.startsWith('workshop-')) {
    toast('本地模组不支持在线更新', 'warn');
    return;
  }
  emit('update', props.mod);
}

async function setExpanded(open) {
  if (open) { expanded.value = true; return; }
  if (configForm.value?.saving || confirmingClose.value) return;
  confirmingClose.value = true;
  try {
    if (configForm.value?.dirty && !await confirmAction({
      title: '放弃未保存的配置？', confirmText: '放弃修改', cancelText: '继续编辑', destructive: true,
    })) return;
    expanded.value = false;
  } finally {
    confirmingClose.value = false;
  }
}
</script>

<template>
  <div
    class="mod mod-enter"
    :role="mod ? 'article' : undefined"
    :aria-label="mod ? (mod.name || mod.id) : undefined"
    :aria-hidden="!mod || undefined"
    :style="enterStyle"
  >
    <div class="mod-entry-head">
      <div class="mod-summary">
        <div class="mod-cover" aria-hidden="true">
          <Skeleton v-if="coverPending" class="size-full rounded-none" />
          <img
            v-else-if="coverUrl"
            class="mod-reveal"
            :src="coverUrl"
            alt=""
            loading="lazy"
            decoding="async"
            @error="onCoverError"
          >
          <Icon v-else name="package" size="28" class="mod-reveal" />
        </div>
        <div class="mod-info">
          <div v-if="titlePending" class="flex h-[21px] items-center">
            <Skeleton class="h-4 rounded" :class="skeletonNameWidth" />
          </div>
          <Tooltip v-else-if="workshopUrl">
            <TooltipTrigger as-child>
              <a
                class="mod-name mod-workshop-link mod-reveal"
                :href="workshopUrl"
                target="_blank"
                rel="noopener noreferrer"
              >{{ mod.name || mod.id }}<Icon name="globe" size="14" /></a>
            </TooltipTrigger>
            <TooltipContent>在 Steam 创意工坊查看（新标签页）</TooltipContent>
          </Tooltip>
          <span v-else class="mod-name mod-reveal">{{ mod.name || mod.id }}</span>
          <div v-if="!mod" class="mod-status h-4">
            <Skeleton class="h-3.5 w-28 rounded" />
            <Skeleton class="h-3.5 w-14 rounded" />
          </div>
          <div v-else class="mod-status mod-reveal">
            <span class="mod-id">{{ mod.id }}</span>
            <Tooltip v-if="mod.installed">
              <TooltipTrigger as-child>
                <span class="break-all text-xs text-muted-foreground" tabindex="0">{{ mod.version ? `版本 ${mod.version}` : '版本未知' }}</span>
              </TooltipTrigger>
              <TooltipContent>本地 modinfo.lua 的版本号</TooltipContent>
            </Tooltip>
            <UiPill v-if="downloading" tone="info">下载中</UiPill>
            <UiPill v-else-if="!mod.installed" tone="warn">未下载</UiPill>
            <Tooltip v-if="updateInfo">
              <TooltipTrigger as-child><UiPill tone="warn" tabindex="0">有新版本</UiPill></TooltipTrigger>
              <TooltipContent>本地 {{ fmtUnix(updateInfo.localTime) }}，工坊 {{ fmtUnix(updateInfo.remoteTime) }}</TooltipContent>
            </Tooltip>
          </div>
        </div>
      </div>

      <div v-if="!mod" class="mod-entry-actions">
        <div class="mod-toggle">
          <Skeleton class="h-[18px] w-8 rounded-full" />
          <Skeleton class="h-3.5 w-10 rounded" />
        </div>
        <Skeleton class="size-7 rounded-lg" />
        <Skeleton class="size-7 rounded-lg" />
        <Skeleton class="size-7 rounded-lg" />
      </div>
      <div v-else class="mod-entry-actions mod-reveal">
        <label class="mod-toggle" :class="{ 'opacity-50 cursor-not-allowed': frozen }">
          <Switch
            :model-value="enabled"
            :disabled="frozen || toggling || removing"
            :aria-label="`启用 ${mod.name || mod.id}`"
            @update:model-value="onToggle($event)"
          />
          <span>{{ toggling ? '保存中…' : enabled ? '已启用' : '已禁用' }}</span>
        </label>
        <Sheet v-if="mod.installed" :open="expanded" @update:open="setExpanded">
          <SheetTrigger as-child>
            <IconAction variant="outline" icon="sliders" label="配置" />
          </SheetTrigger>
          <SheetContent class="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-xl" :show-close-button="false" :aria-describedby="undefined">
            <SheetHeader class="shrink-0 border-b p-4 pr-14 sm:p-6 sm:pr-14">
              <SheetTitle class="break-words">{{ mod.name || mod.id }}</SheetTitle>
              <span v-if="frozen" class="text-sm text-amber-700 dark:text-amber-400">服务器运行中，配置只读</span>
            </SheetHeader>
            <Button variant="ghost" size="icon-sm" class="absolute right-3 top-3" aria-label="关闭配置" :disabled="configForm?.saving" @click="setExpanded(false)"><Icon name="x" :size="18" /></Button>
            <ModConfigForm ref="configForm" :room-id="rooms.currentId" :shard="shard" :mod="mod" :disabled="frozen" @saved="emit('changed')" />
          </SheetContent>
        </Sheet>
        <IconAction
          v-if="mod.id.startsWith('workshop-')"
          variant="outline"
          icon="download"
          :label="downloading ? '下载中…' : mod.installed ? (updateInfo ? '更新' : '重新下载') : '下载'"
          :disabled="frozen || downloading || removing"
          @click="updateOne"
        />
        <Tooltip>
          <TooltipTrigger as="span" :as-child="!(frozen || removing || toggling || downloading || updating)" class="inline-flex" :tabindex="frozen || removing || toggling || downloading || updating ? 0 : undefined">
            <Button variant="ghost" size="icon-sm" class="text-muted-foreground hover:text-destructive" aria-label="删除" :disabled="frozen || removing || toggling || downloading || updating" @click="remove">
              <Icon name="trash" :size="16" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ updating ? '下载进行中，暂时无法删除模组' : '删除模组' }}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  </div>
</template>

<style scoped>
.mod-enter {
  animation: mod-fade-in 180ms ease-out both;
}
.mod-reveal {
  animation: mod-fade-in 180ms ease-out both;
}
@media (prefers-reduced-motion: no-preference) {
  .mod-enter {
    animation: mod-enter-rise 260ms cubic-bezier(0.2, 0, 0, 1) var(--mod-enter-delay, 0ms) both;
  }
}
@keyframes mod-fade-in {
  from { opacity: 0; }
}
@keyframes mod-enter-rise {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}
.mod-entry-head { display: flex; align-items: center; flex-wrap: wrap; gap: 16px; }
.mod-summary { display: flex; align-items: center; gap: 14px; flex: 1 1 280px; min-width: 0; }
.mod-cover {
  display: grid;
  place-items: center;
  width: 56px;
  height: 56px;
  flex-shrink: 0;
  overflow: hidden;
  border-radius: 8px;
  background: var(--surface-3);
  color: var(--text-muted);
}
.mod-cover img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  outline: 1px solid oklch(0 0 0 / 0.1);
  outline-offset: -1px;
}
:global([data-theme="dark"]) .mod-cover img { outline-color: oklch(1 0 0 / 0.1); }
.mod-info { min-width: 0; }
.mod-name { font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
.mod-workshop-link { color: var(--text); text-decoration: none; }
.mod-workshop-link svg { display: inline; margin-left: 6px; vertical-align: -2px; color: var(--text-muted); }
.mod-workshop-link:focus-visible { outline: 2px solid var(--ring); outline-offset: 3px; border-radius: 2px; }
@media (hover: hover) {
  .mod-workshop-link:hover { color: var(--legacy-primary); text-decoration: underline; text-underline-offset: 3px; }
}
.mod-status { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-top: 4px; }
.mod-id { overflow-wrap: anywhere; }
.mod-entry-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-left: auto; }
.mod-toggle { display: inline-flex; align-items: center; min-height: 28px; gap: 8px; margin-right: 8px; font-size: 13px; font-weight: 500; cursor: pointer; }
@media (max-width: 640px) {
  .mod-summary { flex-basis: 100%; }
  .mod-entry-actions { width: 100%; justify-content: flex-end; }
  .mod-toggle { margin-right: auto; }
}
</style>
