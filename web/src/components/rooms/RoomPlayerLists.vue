<script setup>
import { computed, onUnmounted, ref, watch } from 'vue';
import { PhInfo } from '@phosphor-icons/vue';
import UiCard from '@/components/ui/UiCard.vue';
import { Empty, EmptyDescription } from '@/components/ui/empty';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { playerListApi } from '@/lib/api.js';
import { confirmAction } from '@/lib/confirm.js';
import { lookupPlayer, playerOptionLabel, unseenPlayers } from '@/lib/format.js';
import { toast } from '@/lib/toast.js';
import PlayerIdentity from '@/components/rooms/PlayerIdentity.vue';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';

const props = defineProps({ room: { type: Object, required: true } });
const kinds = [
  { key: 'blocklist', title: '黑名单', hint: '禁止名单内的玩家进入房间；不会立即踢出现有玩家。' },
  { key: 'whitelist', title: '白名单', hint: '为名单内的玩家预留位置，不是仅允许名单玩家进入。预留位计入最大人数。' },
];
const kind = ref('blocklist');
const selected = computed(() => kinds.find(item => item.key === kind.value));
const ids = ref([]);
const seen = ref({});
const picked = ref('');
const input = ref('');
const search = ref('');
const loading = ref(false);
const busy = ref(false);
const error = ref('');
const file = ref('');
const needRestart = ref(false);
const slots = ref(0);
let generation = 0;

const candidates = computed(() => unseenPlayers(seen.value, ids.value));
const filtered = computed(() => {
  const query = search.value.trim().toLowerCase();
  return ids.value.filter(id => `${id} ${playerOptionLabel(lookupPlayer(seen.value, id))}`.toLowerCase().includes(query));
});

function who(id) {
  const face = lookupPlayer(seen.value, id);
  return face?.name ? `${face.name}（${id}）` : id;
}

function onPick(id) {
  picked.value = id;
  input.value = id;
}

function apply(data) {
  ids.value = data.ids || [];
  seen.value = data.seen || {};
  file.value = data.file || '';
  needRestart.value = !!data.needRestart;
  slots.value = data.reservedSlots || 0;
}

async function load() {
  const ticket = ++generation;
  const roomId = props.room.id;
  const target = kind.value;
  ids.value = [];
  file.value = '';
  error.value = '';
  loading.value = true;
  try {
    const data = await playerListApi.list(roomId, target);
    if (ticket === generation) apply(data);
  } catch (err) {
    if (ticket === generation) error.value = err.message;
  } finally {
    if (ticket === generation) loading.value = false;
  }
}

watch(() => [props.room.id, kind.value], () => {
  input.value = '';
  picked.value = '';
  search.value = '';
  load();
}, { immediate: true });
onUnmounted(() => { generation++; });

async function change(id, remove = false) {
  if (busy.value || loading.value || error.value) return;
  id = id.trim();
  if (!/^KU_[A-Za-z0-9_-]{1,32}$/i.test(id)) {
    toast('Klei ID 不合法：应为 KU_ 开头、后接字母、数字、下划线或短横线', 'err');
    return;
  }
  const roomId = props.room.id;
  const roomName = props.room.name;
  const target = kind.value;
  const ticket = generation;
  busy.value = true;
  try {
    if (remove && !await confirmAction(`从「${roomName}」的${selected.value.title}移除 ${who(id)}？\n修改将在下次重启房间后生效。`)) return;
    const data = await (remove ? playerListApi.remove : playerListApi.add)(roomId, target, id);
    // Do not overwrite data from a later refresh or a different room/list.
    if (ticket === generation) {
      apply(data);
      input.value = '';
      picked.value = '';
    }
    toast(`「${roomName}」名单已${remove ? '移除' : '添加'}（重启后生效）`);
  } catch (err) {
    toast(err.message, 'err');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <UiCard>
    <template #title>玩家准入名单</template>
    <template #hint>维护黑名单与白名单，修改后需重启房间生效</template>
    <template #actions>
      <IconAction variant="outline" icon="refresh" label="刷新名单" :disabled="loading || busy" @click="load" />
    </template>

    <section :aria-label="selected.title" class="min-w-0 space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <Tabs v-model="kind">
          <TabsList aria-label="名单类型" class="h-7 p-0.5">
            <TabsTrigger
              v-for="item in kinds"
              :key="item.key"
              :value="item.key"
              :disabled="busy"
              class="px-2.5 text-xs"
            >
              {{ item.title }}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <span class="text-xs text-muted-foreground">{{ selected.hint }}</span>
      </div>
      <p v-if="file" class="truncate rounded-md bg-muted/45 px-2.5 py-1.5 font-mono text-[11px] text-muted-foreground" :title="file">
        {{ file }}
      </p>

      <div v-if="loading" role="status" aria-label="读取名单中" class="grid gap-2">
        <div v-for="i in 2" :key="i" class="flex items-center gap-2 rounded-lg border border-border/80 px-3 py-2">
          <Skeleton class="size-8 shrink-0 rounded-md" />
          <div class="grid flex-1 gap-1.5">
            <Skeleton class="h-4 w-32" />
            <Skeleton class="h-3 w-40" />
          </div>
          <Skeleton class="size-7 shrink-0 rounded-md" />
        </div>
      </div>
      <p v-else-if="error" role="alert" class="text-sm text-destructive">读取失败：{{ error }}，请刷新重试。</p>

      <template v-else>
        <p v-if="kind === 'whitelist'" class="text-xs font-medium text-foreground tabular-nums">
          已预留 {{ slots }} / {{ room.maxPlayers }} 个位置（最多预留最大人数对应的位置）
        </p>

        <div v-if="needRestart" role="status" class="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/8 px-3 py-2 text-xs text-amber-800 dark:text-amber-300">
          <PhInfo class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <span>房间正在运行：名单增删将在下次重启房间后生效。</span>
        </div>

        <form class="flex flex-wrap gap-2" @submit.prevent="change(input)">
          <Input
            v-model="input"
            :aria-label="`${selected.title} Klei ID`"
            class="min-w-0 flex-1 basis-48 font-mono"
            placeholder="KU_ 开头的玩家 ID"
            autocomplete="off"
            :disabled="busy"
          />
          <IconAction type="submit" show-label icon="plus" :label="`添加到${selected.title}`" text="添加" :disabled="busy || !input.trim()" />
          <Select v-if="candidates.length" :model-value="picked" :disabled="busy" @update:model-value="onPick">
            <SelectTrigger class="w-full min-w-0 basis-full" aria-label="从进过房间的玩家选择">
              <SelectValue placeholder="从进过房间的玩家选择" />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem v-for="player in candidates" :key="player.id" :value="player.id">{{ playerOptionLabel(player) }}</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </form>

        <Input v-if="ids.length" v-model="search" :aria-label="`搜索${selected.title}`" placeholder="搜索游戏名、角色或 ID" />

        <ul v-if="filtered.length" class="grid gap-2" :aria-label="`${selected.title}玩家`">
          <li v-for="id in filtered" :key="id" class="flex min-w-0 items-center gap-2 rounded-lg border border-border/80 px-3 py-2">
            <PlayerIdentity class="min-w-0 flex-1" :id="id" :seen="seen" />
            <IconAction
              variant="ghost"
              class="text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              icon="trash"
              :label="`从${selected.title}移除 ${who(id)}`"
              :disabled="busy"
              @click="change(id, true)"
            />
          </li>
        </ul>
        <Empty v-else><EmptyDescription>{{ ids.length ? '没有匹配的玩家' : `暂无${selected.title}玩家` }}</EmptyDescription></Empty>
      </template>
    </section>
  </UiCard>
</template>
