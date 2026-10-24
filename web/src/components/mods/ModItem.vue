<script setup>
// 单个模组条目：启用开关、删除、展开配置表单。

import { computed, ref, watch } from 'vue';

import UiPill from '@/components/ui/UiPill.vue';
import Icon from '@/components/ui/Icon.vue';
import { Switch } from '@/components/ui/switch';
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
});

const emit = defineEmits(['changed', 'update']);

const rooms = useRoomsStore();
const expanded = ref(false);
const coverFailed = ref(false);
watch(() => props.mod.previewUrl, () => { coverFailed.value = false; });
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
  if (frozen.value) {
    toast('服务器运行中，请先关闭服务器再修改模组', 'warn');
    return;
  }
  const room = rooms.current;
  if (!room) return;

  enabled.value = next; // 先按用户的意图切换，失败再回滚
  const { ok } = await guard(() => modApi.toggle(room.id, props.mod.id, next));
  if (ok) {
    toast(`${props.mod.name || props.mod.id} ${next ? '已启用' : '已禁用'}`);
    return;
  }
  enabled.value = props.mod.enabled;
}

async function remove() {
  if (frozen.value) {
    toast('服务器运行中，请先关闭服务器再删除模组', 'warn');
    return;
  }
  const room = rooms.current;
  if (!room) return;
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
}

/**
 * 单模组更新：未下载的直接下载；已下载的备份旧文件后重新下载，失败恢复。
 * 创意工坊模组才支持，本地模组没有「下载」一说。
 * 有会话在跑时后端会排队串行下载，这里不需要因「更新中」而禁用。
 * 启动交给父组件（轮询归它管），这里只做确认。
 */
async function updateOne() {
  if (frozen.value) return;
  if (!props.mod.id.startsWith('workshop-')) {
    toast('本地模组不支持在线更新', 'warn');
    return;
  }
  const name = props.mod.name || props.mod.id;
  if (props.mod.installed && !await confirmAction(
    `重新下载模组「${name}」？\n\n` +
    `将备份本地文件并从创意工坊重新获取，失败会恢复旧副本；\n` +
    `文件可能与 Steam 客户端共享，请先退出游戏并暂停 Steam 的模组更新；重下期间其它房间也暂时不可用。`,
  )) return;

  emit('update', props.mod);
}
</script>

<template>
  <div class="mod">
    <div class="mod-entry-head">
      <div class="mod-summary">
        <div class="mod-cover" aria-hidden="true">
          <img
            v-if="mod.previewUrl && !coverFailed"
            :src="mod.previewUrl"
            alt=""
            loading="lazy"
            decoding="async"
            @error="coverFailed = true"
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
            <UiPill :tone="enabled ? 'ok' : 'muted'">
              {{ enabled ? '已启用' : '已禁用' }}
            </UiPill>
            <UiPill :tone="mod.installed ? 'muted' : 'warn'">
              {{ mod.installed ? '已下载' : '未下载' }}
            </UiPill>
            <UiPill
              v-if="updateInfo"
              tone="warn"
              :title="`本地 ${fmtUnix(updateInfo.localTime)}，工坊 ${fmtUnix(updateInfo.remoteTime)}`"
            >
              有新版本
            </UiPill>
          </div>
          <div class="mod-id">{{ mod.id }}</div>
        </div>
      </div>

      <div class="mod-entry-actions">
        <label class="mod-toggle" :class="{ 'opacity-50 cursor-not-allowed': frozen }">
          <Switch
            :model-value="enabled"
            :disabled="frozen"
            :aria-label="`启用 ${mod.name || mod.id}`"
            @update:model-value="onToggle($event)"
          />
          <span>启用</span>
        </label>
        <button
          v-if="mod.id.startsWith('workshop-')"
          class="btn btn-sm"
          :disabled="frozen"
          :title="mod.installed ? '删除本地文件并重新下载（强制刷新）' : '从创意工坊下载该模组'"
          @click="updateOne"
        >
          <Icon name="download" size="14" />{{ mod.installed ? '更新' : '下载' }}
        </button>
        <button
          v-if="mod.installed"
          class="btn btn-sm"
          :disabled="frozen && !expanded"
          :title="frozen ? '服务器运行中，配置已冻结' : ''"
          @click="expanded = !expanded"
        >
          {{ expanded ? '收起' : '配置' }}
        </button>
        <button class="btn btn-sm btn-danger" :disabled="frozen" @click="remove">删除</button>
      </div>
    </div>

    <div v-if="expanded" class="mod-body">
      <ModConfigForm :room-id="rooms.currentId" :mod="mod" :disabled="frozen" />
    </div>
  </div>
</template>

<style scoped>
.mod-entry-head { display: flex; align-items: center; flex-wrap: wrap; gap: 16px; }
.mod-summary { display: flex; align-items: center; gap: 14px; flex: 1 1 280px; min-width: 0; }
.mod-cover {
  display: grid;
  place-items: center;
  width: 72px;
  height: 72px;
  flex-shrink: 0;
  overflow: hidden;
  border-radius: 8px;
  background: var(--surface-3);
  color: var(--text-muted);
}
.mod-cover img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  outline: 1px solid oklch(0 0 0 / 0.1);
  outline-offset: -1px;
}
:global(.dark) .mod-cover img { outline-color: oklch(1 0 0 / 0.1); }
.mod-info { min-width: 0; }
.mod-name { font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
.mod-workshop-link { color: var(--text); text-decoration: none; }
.mod-workshop-link svg { display: inline; margin-left: 6px; vertical-align: -2px; color: var(--text-muted); }
.mod-workshop-link:focus-visible { outline: 2px solid var(--ring); outline-offset: 3px; border-radius: 2px; }
@media (hover: hover) {
  .mod-workshop-link:hover { color: var(--legacy-primary); text-decoration: underline; text-underline-offset: 3px; }
}
.mod-status { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin: 5px 0; }
.mod-id { overflow-wrap: anywhere; }
.mod-entry-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; margin-left: auto; }
.mod-toggle { display: inline-flex; align-items: center; gap: 8px; margin-right: 8px; font-size: 13px; font-weight: 500; cursor: pointer; }
@media (max-width: 640px) {
  .mod-summary { flex-basis: 100%; }
  .mod-entry-actions { width: 100%; justify-content: flex-end; }
  .mod-toggle { margin-right: auto; }
}
</style>
