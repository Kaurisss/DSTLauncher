<script setup>
// 快捷控制台指令弹窗：预设世界/玩家常用 Lua 指令，支持填入输入框或直接执行。

import { computed, ref, watch } from 'vue';
import { PhLightning, PhPlay, PhTerminalWindow } from '@phosphor-icons/vue';

import { Button } from '@/components/ui/button';
import {
  Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { playerApi } from '@/lib/api.js';
import { shardLabel } from '@/lib/format.js';
import { useRoomsStore } from '@/stores/rooms.js';

const props = defineProps({
  shard: { type: String, required: true },
  disabled: { type: Boolean, default: false },
});

const emit = defineEmits(['fill', 'execute']);

const rooms = useRoomsStore();
const open = ref(false);
const scope = ref('world');
const selectedCmd = ref('save');
const announceText = ref('');
const playerId = ref('');
const onlinePlayers = ref([]);
const loadingPlayers = ref(false);

const label = computed(() => shardLabel(props.shard, rooms.current?.locations?.[props.shard]));

function luaQuote(text) {
  return `"${String(text).replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

const WORLD_PRESETS = [
  { key: 'save', label: '立即保存当前分片', desc: '触发当前世界落盘保存', build: () => 'c_save()' },
  { key: 'nextphase', label: '跳至下一时段（黄昏/夜晚/白天）', desc: '推进当前世界昼夜阶段', build: () => 'TheWorld:PushEvent("ms_nextphase")' },
  { key: 'skip1', label: '快进 1 天', desc: '跳过一整天时间循环', build: () => 'c_skip(1)' },
  { key: 'rain_stop', label: '停止下雨 / 降雨切换', desc: '立刻停止当前世界的降雨', build: () => 'TheWorld:PushEvent("ms_forceprecipitation", false)' },
  { key: 'listplayers', label: '在日志列出在线玩家', desc: '打印所有在线玩家名称与 ID', build: () => 'c_listallplayers()' },
  { key: 'announce', label: '发送全服公告', desc: '向所有在线玩家广播文本', needsText: true, build: text => (text.trim() ? `c_announce(${luaQuote(text.trim())})` : '') },
];

const PLAYER_PRESETS = [
  { key: 'revive', label: '复活玩家（幽灵复活）', desc: '将死亡的幽灵状态玩家复活', build: id => `for _,p in ipairs(AllPlayers) do if p.userid==${luaQuote(id)} then p:PushEvent("respawnfromghost") end end` },
  { key: 'heal', label: '恢复生命 / 饥饿 / 理智', desc: '将目标玩家三围补满', build: id => `for _,p in ipairs(AllPlayers) do if p.userid==${luaQuote(id)} then if p.components.health then p.components.health:SetPercent(1) end; if p.components.hunger then p.components.hunger:SetPercent(1) end; if p.components.sanity then p.components.sanity:SetPercent(1) end end end` },
  { key: 'portal', label: '传送回出生门', desc: '将卡住的玩家传送回绚丽之门', build: id => `local p=nil; for _,v in ipairs(AllPlayers) do if v.userid==${luaQuote(id)} then p=v end end; local m=c_findnext("multiplayer_portal") or c_findnext("multiplayer_portal_moonrock"); if p and m then p.Transform:SetPosition(m.Transform:GetWorldPosition()) end` },
  { key: 'godmode', label: '切换无敌模式', desc: '开关目标玩家的无敌状态', build: id => `for _,p in ipairs(AllPlayers) do if p.userid==${luaQuote(id)} then c_godmode(p) end end` },
];

const currentPresets = computed(() => (scope.value === 'world' ? WORLD_PRESETS : PLAYER_PRESETS));
const activePreset = computed(() => currentPresets.value.find(p => p.key === selectedCmd.value) || currentPresets.value[0]);

const validPlayerId = computed(() => /^KU_[A-Za-z0-9_-]{1,32}$/.test(playerId.value.trim()));

const generatedCommand = computed(() => {
  const preset = activePreset.value;
  if (!preset) return '';
  if (scope.value === 'world') return preset.build(announceText.value);
  if (!validPlayerId.value) return '';
  return preset.build(playerId.value.trim());
});

watch(scope, val => {
  selectedCmd.value = val === 'world' ? WORLD_PRESETS[0].key : PLAYER_PRESETS[0].key;
});

watch(open, async val => {
  if (!val || !rooms.currentId) return;
  loadingPlayers.value = true;
  try {
    const data = await playerApi.list(rooms.currentId);
    onlinePlayers.value = (data?.players || []).filter(p => p.shard === props.shard);
    if (!playerId.value && onlinePlayers.value.length) {
      playerId.value = onlinePlayers.value[0].userid;
    }
  } catch (_) {
    onlinePlayers.value = [];
  } finally {
    loadingPlayers.value = false;
  }
});

function onFill() {
  if (!generatedCommand.value) return;
  emit('fill', generatedCommand.value);
  open.value = false;
}

function onExecute() {
  if (!generatedCommand.value || props.disabled) return;
  emit('execute', generatedCommand.value);
  open.value = false;
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <Button type="button" variant="outline" size="sm" class="h-8 shrink-0 gap-1.5 ps-2 pe-2.5 text-xs" :disabled="disabled">
        <PhLightning class="size-3.5" aria-hidden="true" />
        <span class="hidden sm:inline">快捷指令</span>
      </Button>
    </DialogTrigger>
    <DialogContent class="sm:max-w-lg">
      <DialogHeader>
        <DialogTitle>快捷指令（{{ label }}）</DialogTitle>
        <DialogDescription>选择常用 Lua 指令模板，可填入命令栏继续编辑，或直接下发到当前分片。</DialogDescription>
      </DialogHeader>

      <div class="grid gap-4">
        <Tabs v-model="scope">
          <TabsList aria-label="指令分类" class="h-7 p-0.5">
            <TabsTrigger value="world" class="px-3 text-xs">世界指令</TabsTrigger>
            <TabsTrigger value="player" class="px-3 text-xs">玩家指令</TabsTrigger>
          </TabsList>
        </Tabs>

        <Field v-if="scope === 'player'">
          <FieldLabel for="quick-cmd-player">目标玩家（Klei ID）</FieldLabel>
          <div class="grid gap-2">
            <Select v-if="onlinePlayers.length" :model-value="playerId" @update:model-value="playerId = $event">
              <SelectTrigger aria-label="选择在线玩家" class="w-full"><SelectValue placeholder="从在线玩家中选择" /></SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  <SelectItem v-for="p in onlinePlayers" :key="p.userid" :value="p.userid">
                    {{ p.name || p.userid }}（{{ p.userid }}）
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
            <Input id="quick-cmd-player" v-model="playerId" autocomplete="off" placeholder="例如 KU_abc12345" class="font-mono text-xs" />
          </div>
          <FieldDescription class="text-xs">
            {{ loadingPlayers ? '正在读取本分片在线玩家…' : onlinePlayers.length ? '可直接从下方在线列表选择或手动输入 KU_ 开头 ID' : '当前分片暂无在线玩家，可手动输入 KU_ 开头 ID' }}
          </FieldDescription>
        </Field>

        <Field>
          <FieldLabel for="quick-cmd-preset">指令模板</FieldLabel>
          <Select v-model="selectedCmd">
            <SelectTrigger id="quick-cmd-preset" class="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem v-for="item in currentPresets" :key="item.key" :value="item.key">
                  {{ item.label }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
          <FieldDescription v-if="activePreset?.desc" class="text-xs">{{ activePreset.desc }}</FieldDescription>
        </Field>

        <Field v-if="scope === 'world' && activePreset?.needsText">
          <FieldLabel for="quick-cmd-announce">公告内容</FieldLabel>
          <Input id="quick-cmd-announce" v-model="announceText" autocomplete="off" maxlength="200" placeholder="输入要全服广播的内容…" />
        </Field>

        <div class="grid gap-1.5">
          <span class="text-xs font-medium text-muted-foreground">将生成的指令</span>
          <pre class="max-h-24 overflow-auto rounded-lg border border-border bg-muted/35 p-2.5 font-mono text-xs break-all whitespace-pre-wrap text-foreground">{{ generatedCommand || '请先补全上方必填参数' }}</pre>
        </div>
      </div>

      <DialogFooter class="gap-2">
        <DialogClose as-child>
          <Button type="button" variant="outline" size="sm">取消</Button>
        </DialogClose>
        <Button type="button" variant="outline" size="sm" class="gap-1.5" :disabled="!generatedCommand" @click="onFill">
          <PhTerminalWindow class="size-3.5" aria-hidden="true" />
          <span>填入命令栏</span>
        </Button>
        <Button type="button" size="sm" class="gap-1.5" :disabled="!generatedCommand || disabled" @click="onExecute">
          <PhPlay class="size-3.5" aria-hidden="true" />
          <span>立即执行</span>
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
