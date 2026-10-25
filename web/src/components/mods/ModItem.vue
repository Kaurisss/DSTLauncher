<script setup>
// 单个模组条目：启用开关、删除、展开配置表单。

import { computed, ref, watch } from 'vue';

import UiPill from '@/components/ui/UiPill.vue';
import Icon from '@/components/ui/Icon.vue';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet';
import ModConfigForm from '@/components/mods/ModConfigForm.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { modApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';
import { fmtUnix } from '@/lib/format.js';

const props = defineProps({
  mod: { type: Object, required: true },
  /** 「检查更新」查到的结果：{ id, name, localTime, remoteTime }，无更新时为 null */
  updateInfo: { type: Object, default: null },
  downloading: { type: Boolean, default: false },
  updating: { type: Boolean, default: false },
});

const emit = defineEmits(['changed', 'update', 'toggled', 'coverNeeded']);

const rooms = useRoomsStore();
const expanded = ref(false);
const configForm = ref(null);
const toggling = ref(false);
const removing = ref(false);
const confirmingClose = ref(false);
const coverFailed = ref(false);
const iconFailed = ref(false);
watch(() => props.mod.previewUrl, () => { coverFailed.value = false; });
watch(() => props.mod.iconUrl, () => { iconFailed.value = false; });
const coverUrl = computed(() => (!iconFailed.value && props.mod.iconUrl)
  || (!coverFailed.value && props.mod.previewUrl) || '');
function onCoverError() {
  if (coverUrl.value === props.mod.iconUrl) {
    iconFailed.value = true;
    emit('coverNeeded');
  }
  else coverFailed.value = true;
}
const workshopUrl = computed(() => /^workshop-\d+$/.test(props.mod.id)
  ? `https://steamcommunity.com/sharedfiles/filedetails/?id=${props.mod.id.slice('workshop-'.length)}`
  : '');

/** 服务器运行中冻结：模组开关与配置重启后才生效，改了也不生效 */
const frozen = computed(() => rooms.running);

// 开关状态用本地副本而不是直接改 props.mod：
// 一是避免修改父组件数据，二是接口失败时可以回滚显示。
const enabled = ref(props.mod.enabled);
watch(() => props.mod.enabled, v => { enabled.value = v; });

async function onToggle(next) {
  if (toggling.value) return;
  if (frozen.value) {
    toast('服务器运行中，请先关闭服务器再修改模组', 'warn');
    return;
  }
  const room = rooms.current;
  if (!room) return;

  toggling.value = true;
  enabled.value = next; // 先按用户的意图切换，失败再回滚
  const { ok } = await guard(() => modApi.toggle(room.id, props.mod.id, next));
  toggling.value = false;
  if (ok) {
    emit('toggled', next);
    emit('changed');
    toast(`${props.mod.name || props.mod.id} ${next ? '已启用' : '已禁用'}`);
    return;
  }
  enabled.value = props.mod.enabled;
}

async function remove() {
  if (removing.value || props.updating) return;
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
      `模组文件与其它房间共享，使用 Steam 工坊目录时也会影响游戏客户端；删除后需要重新下载。` +
      `只会从当前房间的 modoverrides 中移除启用记录。`,
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
  if (frozen.value) return;
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
  <Sheet :open="expanded" @update:open="setExpanded">
    <div class="mod" role="article" :aria-label="mod.name || mod.id">
      <div class="mod-entry-head">
        <div class="mod-summary">
          <div class="mod-cover" aria-hidden="true">
            <img
              v-if="coverUrl"
              :src="coverUrl"
              alt=""
              loading="lazy"
              decoding="async"
              @error="onCoverError"
            >
            <Icon v-else name="package" size="28" />
          </div>
          <div class="mod-info">
            <a
              v-if="workshopUrl"
              class="mod-name mod-workshop-link"
              :href="workshopUrl"
              target="_blank"
              rel="noopener noreferrer"
              title="在 Steam 创意工坊查看（新标签页）"
            >{{ mod.name || mod.id }}<Icon name="globe" size="14" /></a>
            <span v-else class="mod-name">{{ mod.name || mod.id }}</span>
            <div class="mod-status">
              <span class="mod-id">{{ mod.id }}</span>
              <span v-if="mod.installed" class="break-all text-xs text-muted-foreground" title="本地 modinfo.lua 的版本号">{{ mod.version ? `版本 ${mod.version}` : '版本未知' }}</span>
              <UiPill v-if="downloading" tone="info">下载中</UiPill>
              <UiPill v-else-if="!mod.installed" tone="warn">未下载</UiPill>
              <UiPill
                v-if="updateInfo"
                tone="warn"
                :title="`本地 ${fmtUnix(updateInfo.localTime)}，工坊 ${fmtUnix(updateInfo.remoteTime)}`"
              >
                有新版本
              </UiPill>
            </div>
          </div>
        </div>

        <div class="mod-entry-actions">
          <label class="mod-toggle" :class="{ 'opacity-50 cursor-not-allowed': frozen }">
            <Switch
              :model-value="enabled"
              :disabled="frozen || toggling || removing"
              :aria-label="`启用 ${mod.name || mod.id}`"
              @update:model-value="onToggle($event)"
            />
            <span>{{ toggling ? '保存中…' : enabled ? '已启用' : '已禁用' }}</span>
          </label>
          <SheetTrigger v-if="mod.installed" as-child>
            <Button variant="outline" size="sm">配置</Button>
          </SheetTrigger>
          <Button
            v-if="mod.id.startsWith('workshop-')"
            variant="outline" size="sm"
            :disabled="frozen || downloading || removing"
            :title="mod.installed ? '重新下载该模组' : '从创意工坊下载该模组'"
            @click="updateOne"
          >
            <Icon name="download" size="14" />{{ downloading ? '下载中…' : mod.installed ? (updateInfo ? '更新' : '重新下载') : '下载' }}
          </Button>
          <Button variant="ghost" size="icon-sm" class="text-muted-foreground hover:text-destructive" aria-label="删除" :title="updating ? '下载进行中，暂时无法删除模组' : '删除模组'" :disabled="frozen || removing || toggling || downloading || updating" @click="remove">
            <Icon name="trash" :size="16" />
          </Button>
        </div>
      </div>
    </div>
    <SheetContent class="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-xl" :show-close-button="false" :aria-describedby="undefined">
      <SheetHeader class="shrink-0 border-b p-4 pr-14 sm:p-6 sm:pr-14">
        <SheetTitle class="break-words">{{ mod.name || mod.id }}</SheetTitle>
        <span v-if="frozen" class="text-sm text-amber-700 dark:text-amber-400">服务器运行中，配置只读</span>
      </SheetHeader>
      <Button variant="ghost" size="icon-sm" class="absolute right-3 top-3" aria-label="关闭配置" :disabled="configForm?.saving" @click="setExpanded(false)"><Icon name="x" :size="18" /></Button>
      <ModConfigForm ref="configForm" :room-id="rooms.currentId" :mod="mod" :disabled="frozen" @saved="emit('changed')" />
    </SheetContent>
  </Sheet>
</template>

<style scoped>
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
.mod-status { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-top: 5px; }
.mod-id { overflow-wrap: anywhere; }
.mod-entry-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-left: auto; }
.mod-toggle { display: inline-flex; align-items: center; min-height: 28px; gap: 8px; margin-right: 8px; font-size: 13px; font-weight: 500; cursor: pointer; }
@media (max-width: 640px) {
  .mod-summary { flex-basis: 100%; }
  .mod-entry-actions { width: 100%; justify-content: flex-end; }
  .mod-toggle { margin-right: auto; }
}
</style>
