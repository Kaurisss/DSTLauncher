<script setup>
// 单个房间卡片：概览信息 + 操作按钮。整卡点击即选中。

import { computed } from 'vue';

import UiPill from '@/components/ui/UiPill.vue';
import Icon from '@/components/ui/Icon.vue';
import { Button } from '@/components/ui/button';
import { GAME_MODE_LABEL, SHARD_NAME_LABEL } from '@/lib/format.js';

const props = defineProps({
  room: { type: Object, required: true },
  selected: { type: Boolean, default: false },
});

defineEmits(['select', 'edit', 'remove', 'upload']);

const shards = computed(() => props.room.shards || []);
const running = computed(() => (props.room.statuses || []).some(s => s.state === 'running'));
const modeLabel = computed(() => GAME_MODE_LABEL[props.room.gameMode] || props.room.gameMode);
const enabledCount = computed(() => shards.value.filter(s => !s.disabled).length);

/** 常见分片给友好名（地表 / 洞穴），自定义世界直接显示名字 */
const shardLabel = s => SHARD_NAME_LABEL[s.name] || s.name;

/** 端口按「游戏端口 / Steam 主端口 / Steam 认证端口」顺序展示 */
function ports(s) {
  if (!s.serverPort) return '—';
  return `${s.serverPort} / ${s.masterServerPort} / ${s.authenticationPort}`;
}
</script>

<template>
  <article
    class="room-card"
    :class="{ 'is-selected': selected }"
  >
    <!-- 整卡选择层：真正的键盘/读屏控件，覆盖正文与空白区域 -->
    <button
      type="button"
      class="room-select"
      :aria-label="`选择房间 ${room.name}`"
      :aria-pressed="selected"
      @click="$emit('select', room.id)"
    />

    <div class="room-card-head">
      <div class="room-card-identity">
        <h3 class="room-title">{{ room.name }}</h3>
        <div class="room-sub">
          {{ room.key }}{{ room.hasSave ? ' · 已有存档' : '' }}
        </div>
      </div>
      <span v-if="selected" class="room-selection">
        <Icon name="check" size="14" />当前房间
      </span>
    </div>

    <p v-if="room.description" class="room-description">{{ room.description }}</p>

    <div class="room-summary">
      <span class="room-run" :class="{ 'is-on': running }">
        <Icon :name="running ? 'play' : 'stop'" size="13" />{{ running ? '运行中' : '已停止' }}
      </span>
      <span>{{ modeLabel }}</span>
      <span v-if="enabledCount <= 1">仅一个世界</span>
      <UiPill v-if="room.needToken" tone="warn">
        <Icon name="key" size="12" />缺令牌
      </UiPill>
    </div>

    <div class="room-ports">
      <div class="room-ports-head">
        <span>分片端口</span>
        <span>游戏 / Steam / 认证</span>
      </div>
      <dl class="room-port-rows">
        <div v-for="s in shards" :key="s.name" class="room-port-row">
          <dt>{{ shardLabel(s) }}</dt>
          <dd>{{ s.disabled ? '未启用' : ports(s) }}</dd>
        </div>
      </dl>
    </div>

    <div class="room-config">
      <span>tick {{ room.tickRate }}</span>
      <span>回档 {{ room.maxSnapshots }}</span>
    </div>

    <div class="room-actions">
      <Button variant="outline" size="sm" @click="$emit('edit', room.id)">
        <Icon name="pencil" size="14" />编辑
      </Button>
      <Button variant="outline" size="sm" @click="$emit('upload', room.id)">
        <Icon name="upload" size="14" />上传存档
      </Button>
      <Button variant="destructive" size="sm" @click="$emit('remove', room.id)">
        <Icon name="trash" size="14" />删除
      </Button>
    </div>
  </article>
</template>
