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
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';
import { WORLD_TYPE_LABEL } from '@/lib/format.js';
import { confirmAction } from '@/lib/confirm.js';

const props = defineProps({
  /** 新建房间时还没落盘，所有分片都可以改名 */
  roomIsNew: { type: Boolean, default: false },
  /** 嵌入带统一分区标题的编辑器时隐藏自身头部 */
  hideHeader: { type: Boolean, default: false },
  locations: { type: Object, default: () => ({}) },
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

function displayedWorldType(s) {
  return props.locations[s.name] || s.worldType;
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

defineExpose({ addShard });

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
  <section class="grid gap-3" aria-label="世界（分片）">
    <div v-if="!hideHeader" class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-xs font-semibold text-muted-foreground">世界（分片）</h3>
      <IconAction variant="outline" show-label icon="plus" label="添加世界" @click="addShard" />
    </div>

    <div class="shard-editor-grid grid items-start gap-4 lg:grid-cols-2">
      <div
        v-for="(s, i) in shards"
        :key="i"
        class="rounded-xl border border-border/80 bg-card p-4 transition-opacity"
        :class="{ 'opacity-60': s.disabled }"
      >
        <div class="flex min-h-7 flex-wrap items-center gap-2">
          <template v-if="canRename(s)">
            <Tooltip>
              <TooltipTrigger as-child>
                <Input v-model.trim="s.name" class="h-8 min-w-24 flex-1 font-mono text-sm font-medium" aria-label="世界名" placeholder="世界名" />
              </TooltipTrigger>
              <TooltipContent>英文名，不含空格；保存后不可修改</TooltipContent>
            </Tooltip>
            <Select v-model="s.worldType">
              <Tooltip>
                <TooltipTrigger as-child>
                  <SelectTrigger class="h-8 w-24 shrink-0 text-xs" aria-label="世界类型">
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
                <TooltipContent>决定世界生成预设与可用设置项</TooltipContent>
              </Tooltip>
            </Select>
          </template>
          <template v-else>
            <span class="truncate font-mono text-sm font-semibold text-foreground">{{ s.name }}</span>
            <Badge variant="secondary" class="text-[11px] font-normal">{{ typeLabel(displayedWorldType(s)) }}</Badge>
          </template>

          <Badge
            v-if="s.isMaster"
            variant="outline"
            class="border-primary/35 bg-primary/8 text-[11px] font-medium text-primary dark:text-emerald-400"
          >
            主世界
          </Badge>

          <div class="ml-auto flex items-center gap-1.5">
            <Label class="cursor-pointer select-none font-normal">
              <Switch
                :model-value="!s.disabled"
                :disabled="s.isMaster"
                @update:model-value="v => (s.disabled = !v)"
              />
              <span>启用</span>
            </Label>
            <Tooltip v-if="!s.isMaster">
              <TooltipTrigger as="span" :as-child="!removing" class="inline-flex" :tabindex="removing ? 0 : undefined">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  class="size-7 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                  aria-label="从面板移除（存档目录保留）"
                  :disabled="removing"
                  @click="removeShard(i)"
                >
                  <Icon name="trash" size="14" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>从面板移除（存档目录保留）</TooltipContent>
            </Tooltip>
          </div>
        </div>

        <div class="mt-3.5 grid grid-cols-1 gap-2.5 sm:grid-cols-3">
          <label v-for="p in PORTS" :key="p.key" class="flex min-w-0 flex-col gap-1">
            <span class="text-[11px] text-muted-foreground">{{ p.label }}</span>
            <Input
              v-model.number="s[p.key]"
              class="h-8 font-mono text-xs tabular-nums"
              type="number"
              min="1024"
              max="65535"
              :placeholder="s[p.key] ? '' : '自动分配'"
            />
          </label>
        </div>

        <div class="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-border/60 pt-2.5 text-xs text-muted-foreground">
          <span>
            世界 ID <code>{{ s.gameId || '自动' }}</code>
            <template v-if="s.isMaster">（进服落点，不可停用）</template>
            <template v-else-if="s.disabled">（已停用，不会启动）</template>
          </span>
          <span v-if="!canRename(s)" class="text-[11px] text-muted-foreground/75">目录名与类型不可修改</span>
        </div>
      </div>
    </div>
  </section>
</template>
