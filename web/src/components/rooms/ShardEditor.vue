<script setup>
// 分片（世界）编辑器。
//
// 每个分片是一个独立的服务器进程：自己的存档目录、server.ini 与三组端口。
// 分片名同时是**存档目录名**和 `-shard` 启动参数，所以已有分片不能改名——
// 改名等于换目录，原存档会留在旧目录里，看起来就像世界丢了。
//
// 主分片固定存在、不可停用也不可删除：房间至少要有一个世界。

import { ref } from 'vue';

import Icon from '@/components/ui/Icon.vue';
import UiSwitch from '@/components/ui/UiSwitch.vue';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';
import { WORLD_TYPE_LABEL } from '@/lib/format.js';
import { confirmAction } from '@/lib/confirm.js';

const props = defineProps({
  /** 新建房间时还没落盘，所有分片都可以改名 */
  roomIsNew: { type: Boolean, default: false },
});

const shards = defineModel('shards', { type: Array, required: true });

const WORLD_TYPES = Object.entries(WORLD_TYPE_LABEL).map(([value, label]) => ({ value, label }));

const PORTS = [
  { key: 'serverPort', label: '游戏端口' },
  { key: 'masterServerPort', label: 'Steam 主端口' },
  { key: 'authenticationPort', label: 'Steam 认证口' },
];

/** 已有分片的名字锁死；本次新增的、或新建房间时的分片可以改 */
function canRename(s) {
  return props.roomIsNew || s.isNew;
}

function typeLabel(value) {
  return WORLD_TYPE_LABEL[value] || value || WORLD_TYPE_LABEL.forest;
}

function addShard() {
  const used = new Set(shards.value.map(s => s.name));
  let n = shards.value.length + 1;
  while (used.has('World' + n)) n += 1;

  shards.value = [
    ...shards.value,
    {
      name: 'World' + n,
      worldType: 'forest',
      isMaster: false,
      disabled: false,
      gameId: 0,
      serverPort: null,
      masterServerPort: null,
      authenticationPort: null,
      isNew: true,
    },
  ];
}

const removing = ref(false);

async function removeShard(index) {
  const s = shards.value[index];
  if (!s || s.isMaster || removing.value) return;
  removing.value = true;
  try {
    const ok = await confirmAction(
      `从面板移除世界「${s.name}」？\n\n`
      + '存档目录会保留在磁盘上，但面板不再管理它。\n'
      + '以后用同名的分片重新添加，就能继续使用这份存档。',
    );
    if (!ok) return;
    shards.value = shards.value.filter(shard => shard !== s);
  } finally {
    removing.value = false;
  }
}
</script>

<template>
  <section class="grid gap-3">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-xs font-semibold tracking-wide text-muted-foreground uppercase">世界（分片）</h3>
      <Button variant="outline" size="sm" type="button" @click="addShard">
        <Icon name="plus" size="14" />添加世界
      </Button>
    </div>

    <div class="shard-editor-grid grid items-start gap-4 lg:grid-cols-2">
      <div
        v-for="(s, i) in shards"
        :key="i"
        class="shard-card rounded-lg border border-border p-3 transition-opacity"
        :class="{ 'opacity-60': s.disabled }"
      >
        <div class="flex flex-wrap items-center gap-2">
          <Tooltip>
            <TooltipTrigger as="span" :as-child="canRename(s)" class="inline-flex min-w-24 flex-1" :tabindex="!canRename(s) ? 0 : undefined">
              <Input v-model.trim="s.name" class="font-medium disabled:pointer-events-none" :disabled="!canRename(s)" aria-label="世界名" placeholder="世界名" />
            </TooltipTrigger>
            <TooltipContent>{{ canRename(s) ? '英文名，不含空格；保存后不可修改' : '存档目录名，保存后不可修改' }}</TooltipContent>
          </Tooltip>
          <Select v-model="s.worldType" :disabled="!canRename(s)">
            <Tooltip>
              <TooltipTrigger as="span" :as-child="canRename(s)" class="inline-flex w-24 shrink-0" :tabindex="!canRename(s) ? 0 : undefined">
                <SelectTrigger class="w-full disabled:pointer-events-none" aria-label="世界类型">
                  <SelectValue :placeholder="typeLabel(s.worldType)" />
                </SelectTrigger>
              </TooltipTrigger>
              <SelectContent position="popper">
                <SelectGroup>
                  <SelectItem v-for="t in WORLD_TYPES" :key="t.value" :value="t.value">
                    {{ t.label }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
              <TooltipContent>{{ canRename(s) ? '决定世界生成预设与可用设置项' : '存档类型' }}</TooltipContent>
            </Tooltip>
          </Select>
          <span v-if="s.isMaster" class="pill pill-info">主世界</span>
          <div class="ml-auto flex items-center gap-2">
            <UiSwitch
              :model-value="!s.disabled"
              :disabled="s.isMaster"
              label="启用"
              @update:model-value="v => (s.disabled = !v)"
            />
            <Tooltip v-if="!s.isMaster">
              <TooltipTrigger as="span" :as-child="!removing" class="inline-flex" :tabindex="removing ? 0 : undefined">
                <Button type="button" variant="destructive" size="icon-sm" aria-label="从面板移除（存档目录保留）" :disabled="removing" @click="removeShard(i)">
                  <Icon name="trash" size="14" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>从面板移除（存档目录保留）</TooltipContent>
            </Tooltip>
          </div>
        </div>

        <div class="mt-3 grid grid-cols-3 gap-2">
          <label v-for="p in PORTS" :key="p.key" class="flex min-w-0 flex-col gap-1">
            <span class="text-[11px] text-muted-foreground">{{ p.label }}</span>
            <Input
              v-model.number="s[p.key]"
              class="h-8 text-sm"
              type="number"
              min="1024"
              max="65535"
              :placeholder="s[p.key] ? '' : '自动分配'"
            />
          </label>
        </div>

        <p class="mt-2 text-xs text-muted-foreground">
          世界 ID <code>{{ s.gameId || '自动' }}</code>
          <template v-if="s.isMaster"> · 进服落点，不可停用</template>
          <template v-else-if="s.disabled">· 已停用，不会启动</template>
        </p>
      </div>
    </div>
  </section>
</template>
