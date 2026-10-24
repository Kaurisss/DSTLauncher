<script setup>
// 房间编辑表单。
//
// 房间配置是全部 ini 的唯一事实来源，所以这里覆盖了 cluster.ini 与 server.ini
// 的所有可配置项。表单状态是本地副本，只有点保存才写回后端。
// 切换编辑对象时由父组件用 :key 重建本组件，因此不需要 watch 同步。

import { computed, reactive, ref } from 'vue';

import UiCard from '@/components/ui/UiCard.vue';
import UiField from '@/components/ui/UiField.vue';
import UiSwitch from '@/components/ui/UiSwitch.vue';
import Icon from '@/components/ui/Icon.vue';
import ShardEditor from '@/components/rooms/ShardEditor.vue';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
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
  <UiCard class="room-editor">
    <template #title>{{ isNew ? '新建房间' : '编辑房间 · ' + form.name }}</template>
    <template v-if="!isNew" #hint>Cluster_{{ form.id }}</template>
    <template #actions>
      <Button :disabled="saving" @click="save">
        <Icon name="check" size="14" />{{ saving ? '保存中…' : '保存' }}
      </Button>
      <Button variant="outline" :disabled="saving" @click="emit('close')">取消</Button>
    </template>

    <Alert v-if="saveError" variant="destructive" class="mb-4">
      <AlertTitle>保存失败</AlertTitle>
      <AlertDescription>{{ saveError }}</AlertDescription>
    </Alert>

    <section class="form-section">
      <h3>基本信息</h3>
      <div class="form-grid">
        <UiField label="房间名" for-id="f_name">
          <Input id="f_name" v-model="form.name" type="text" placeholder="显示在服务器列表里的名字" />
        </UiField>
        <UiField label="描述" for-id="f_description">
          <Input id="f_description" v-model="form.description" type="text" placeholder="一句话介绍" />
        </UiField>
        <UiField label="游戏模式" for-id="f_gameMode">
          <Select v-model="form.gameMode">
            <SelectTrigger id="f_gameMode">
              <SelectValue :placeholder="GAME_MODE_LABEL[form.gameMode] || form.gameMode" />
            </SelectTrigger>
            <SelectContent position="popper">
              <SelectGroup>
                <SelectItem v-for="m in gameModes" :key="m" :value="m">
                  {{ GAME_MODE_LABEL[m] || m }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </UiField>
        <UiField label="最大人数" for-id="f_maxPlayers">
          <Input id="f_maxPlayers" v-model.number="form.maxPlayers" type="number" min="1" max="64" />
        </UiField>
        <UiField label="tick 频率" for-id="f_tickRate">
          <Select :model-value="String(form.tickRate)" @update:model-value="(v) => form.tickRate = Number(v)">
            <SelectTrigger id="f_tickRate">
              <SelectValue :placeholder="TICK_RATE_LABEL[form.tickRate] || String(form.tickRate)" />
            </SelectTrigger>
            <SelectContent position="popper">
              <SelectGroup>
                <SelectItem v-for="t in tickRates" :key="t" :value="String(t)">
                  {{ TICK_RATE_LABEL[t] || t }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </UiField>
        <UiField label="语言" for-id="f_language">
          <Select v-model="form.language">
            <SelectTrigger id="f_language">
              <SelectValue :placeholder="LANGUAGE_LABEL[form.language] || form.language" />
            </SelectTrigger>
            <SelectContent position="popper">
              <SelectGroup>
                <SelectItem v-for="l in languages" :key="l" :value="l">
                  {{ LANGUAGE_LABEL[l] || l }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </UiField>
        <UiField label="房间密码" for-id="f_password">
          <Input id="f_password" v-model="form.password" type="text" placeholder="留空表示无密码" />
        </UiField>
        <UiField label="回档点数" for-id="f_maxSnapshots">
          <Input id="f_maxSnapshots" v-model.number="form.maxSnapshots" type="number" min="1" max="100" />
        </UiField>
      </div>
    </section>

    <section class="form-section">
      <h3>Klei 服务器令牌</h3>
      <UiField
        for-id="f_token"
        block
        tip="在线开服必需，会写入存档目录下的 cluster_token.txt；勾选「仅局域网」或「离线模式」后可以留空。"
      >
        <Textarea
          id="f_token"
          v-model="form.token"
          rows="2"
          spellcheck="false"
          placeholder="pds-g^... 从 Klei 账号页复制"
        />
      </UiField>
    </section>

    <section class="form-section">
      <h3>行为开关</h3>
      <div class="switches">
        <UiSwitch v-model="form.pvp" label="PVP" />
        <UiSwitch v-model="form.pauseEmpty" label="无人时暂停" />
        <UiSwitch v-model="form.voteEnabled" label="投票" />
        <UiSwitch v-model="form.voteKick" label="投票踢人" />
        <UiSwitch v-model="form.lanOnly" label="仅局域网" />
        <UiSwitch v-model="form.offline" label="离线模式" />
      </div>
    </section>

    <section class="form-section">
      <h3>世界（分片）</h3>
      <ShardEditor v-model:shards="form.shards" :room-is-new="isNew" />
    </section>

    <details class="form-section details">
      <summary>高级设置</summary>
      <div class="form-grid form-grid-top">
        <UiField label="主分片 IP" for-id="f_masterIp">
          <Input id="f_masterIp" v-model="form.masterIp" type="text" />
        </UiField>
        <UiField label="分片协调端口" for-id="f_masterPort">
          <Input id="f_masterPort" v-model.number="form.masterPort" type="number" min="1024" max="65535" />
        </UiField>
        <UiField
          label="通信密钥"
          for-id="f_clusterKey"
          wide
          tip="各分片之间的认证密钥，全部启用分片必须一致；改动后需重启全部分片。"
        >
          <Input id="f_clusterKey" v-model="form.clusterKey" type="text" />
        </UiField>
      </div>
    </details>
  </UiCard>
</template>

<style scoped>
.room-editor :deep([data-slot="card-header"]) {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--card);
  border-top-left-radius: inherit;
  border-top-right-radius: inherit;
}
</style>
