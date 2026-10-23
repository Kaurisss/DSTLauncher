<script setup>
// 世界配置：普通文件整表保存并保留未知项，leveldata 仅更新点名的规则。
// 恢复默认也属于修改，必须显式提交；已有存档的生成项始终只读。
//
// 世界规则（settings）与世界生成（worldgen）分开成两个标签页展示；
// 服务器运行中整个页面冻结——游戏只在启动时读这份文件，改了也不生效。

import { computed, onMounted, reactive, ref, watch } from 'vue';

import Icon from '@/components/ui/Icon.vue';
import UiCard from '@/components/ui/UiCard.vue';
import UiEmpty from '@/components/ui/UiEmpty.vue';
import UiField from '@/components/ui/UiField.vue';
import UiSkeleton from '@/components/ui/UiSkeleton.vue';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';
import OptionGroupPanel from '@/components/world/OptionGroupPanel.vue';
import { worldApi } from '@/lib/api.js';
import { iconOf } from '@/lib/worldIcons.js';
import { useRoomsStore } from '@/stores/rooms.js';
import { guard, toast } from '@/lib/toast.js';
import { WORLD_TYPE_LABEL, MOD_LABEL, shardLabel } from '@/lib/format.js';

const rooms = useRoomsStore();

const shard = ref('');
const loadedShard = ref('');
const view = ref(null);
const loading = ref(false);
const saving = ref(false);
const loadError = ref('');

const enabled = ref(true);
const worldgenPreset = ref('');
const settingsPreset = ref('');

/** 待保存增量：相对载入值记录，包含显式恢复的默认值 */
const draft = reactive({});
/** 载入时的原始值，用于判断有没有未保存的改动 */
const original = reactive({ enabled: true, worldgenPreset: '', settingsPreset: '' });
const originalOverrides = ref({});
/** 按分片缓存视图与未保存草稿，切分片不丢改动 */
const shardCache = reactive({});
let loadSeq = 0;

const keyword = ref('');
const onlyChanged = ref(false);

/** 选项标签页：世界规则 / 世界生成 */
const category = ref('settings');

/** 用户手动选过类别后，不再自动跳转 */
const categoryPinned = ref(false);

/** 手动切换类别：记住用户的选择，不再自动回落。 */
function pickCategory(key) {
  categoryPinned.value = true;
  category.value = key;
}

/* ---------- 派生状态 ---------- */

/**
 * 分片标签。取存档里读到的 location；还没读到（切分片的请求未回）时
 * 用房间列表里那一份，避免先闪一下目录名。
 */
const labelOf = name => {
  const locs = (rooms.current && rooms.current.locations) || {};
  return shardLabel(name, locs[name]);
};

/**
 * 当前分片的世界类型。
 * 优先用后端从存档 leveldataoverride.lua 读到的 location：房间配置里的
 * WorldType 可能是旧数据（例如海难分片还写着 forest），按它过滤会掉走整批模组选项。
 */
const shardType = computed(() => {
  if (view.value && view.value.location) return view.value.location;
  const list = (rooms.current && rooms.current.shards) || [];
  const found = list.find(x => x.name === shard.value);
  return (found && found.worldType) || 'forest';
});

const typeLabel = computed(() => WORLD_TYPE_LABEL[shardType.value] || shardType.value);
const modLabel = computed(() => (view.value && view.value.mod ? MOD_LABEL[view.value.mod] : ''));

const shardHint = computed(() =>
  `${labelOf(shard.value)} · ${modLabel.value || typeLabel.value}`);

/** 提示文案用的世界描述：“岛屿冒险 · 海难”比只写模组名更清楚。 */
const worldLabel = computed(() =>
  modLabel.value ? `${modLabel.value} · ${typeLabel.value}` : typeLabel.value);

/** 服务器运行中冻结全部修改：游戏只在启动时读配置，改了也不生效 */
const frozen = computed(() => rooms.running);

/**
 * 世界已生成的房间冻结「世界生成」类配置：
 * 这些选项只在生成世界时被读取，已有地图不会重建，改了纯属误导。
 *
 * 使用 leveldataoverride.lua 的存档（客户端生成过世界）天然就是这种状态：
 * 游戏只认它里面的完整世界定义，面板也只能改世界规则。
 */
const worldGenerated = computed(() => !!(view.value?.hasSave || view.value?.levelDataInUse));
const worldgenFrozen = computed(() => frozen.value || worldGenerated.value);

/** 存档用的是游戏自己的 leveldataoverride.lua：预设与“启用本文件”由游戏固定，不展示开关。 */
const levelDataInUse = computed(() => !!view.value?.levelDataInUse);

/** 候选定义不丢弃，显隐仅影响展示；缺省开关取源码默认值。 */
const displayGroups = computed(() => {
  const groups = view.value?.groups || [];
  const island = groups.flatMap(g => g.options).find(o => o.name === 'volcanoisland');
  if (island && valueOf(island) !== 'none') return groups;
  const hidden = new Set(view.value?.volcanoIslandOnly || []);
  return groups.map(g => ({ ...g, options: g.options.filter(o => !hidden.has(o.name)) }))
    .filter(g => g.options.length > 0);
});

const totalOptions = computed(() =>
  displayGroups.value.reduce((n, g) => n + g.options.length, 0));

const changedCount = computed(() =>
  displayGroups.value.reduce((n, g) => n + g.options.filter(isChanged).length, 0));

const settingsChanged = computed(() =>
  enabled.value !== original.enabled
  || worldgenPreset.value !== original.worldgenPreset
  || settingsPreset.value !== original.settingsPreset);

const currentDirty = computed(() => Object.keys(draft).length > 0 || settingsChanged.value);
const otherShardsDirty = computed(() =>
  Object.entries(shardCache).some(([s, st]) => s !== shard.value && isShardStateDirty(st)));
const dirty = computed(() => currentDirty.value || otherShardsDirty.value);
const canReset = computed(() => settingsChanged.value || displayGroups.value.some(g =>
  !(worldgenFrozen.value && g.category === 'worldgen') && g.options.some(isChanged)));

const worldgenPresets = computed(() => view.value?.presets?.worldgen || []);
const settingsPresets = computed(() => view.value?.presets?.settings || []);

/**
 * 预设与分片类型不一致。
 *
 * 引擎只按预设生成世界，既不看分片叫什么，也不看面板里的类型：
 * 洞穴分片配成 SURVIVAL_TOGETHER 就会长出第二张地表图。
 * 世界生成不可逆，所以这里必须显式提醒，而不是等玩家下去才发现。
 *
 * 模组世界不套这个规则：海难与火山的预设本来就不同名，也不带 CAVE 字样。
 */
const presetMismatch = computed(() => {
  if (view.value?.mod) return false;
  const p = worldgenPreset.value || '';
  if (!p) return false;
  const isCavePreset = p.toUpperCase().includes('CAVE');
  return (shardType.value === 'cave') !== isCavePreset;
});

/** 按类别拆分分组 */
const settingsGroups = computed(() =>
  displayGroups.value.filter(g => g.category !== 'worldgen'));
const worldgenGroups = computed(() =>
  displayGroups.value.filter(g => g.category === 'worldgen'));

const activeGroups = computed(() =>
  category.value === 'worldgen' ? worldgenGroups.value : settingsGroups.value);

const categoryCounts = computed(() => ({
  settings: settingsGroups.value.reduce((n, g) => n + g.options.length, 0),
  worldgen: worldgenGroups.value.reduce((n, g) => n + g.options.length, 0),
}));

/**
 * 默认停在有内容的标签。
 * 存档使用 leveldataoverride.lua 时世界生成全是只读的，若当前标签一篇空白，
 * 用户会以为配置丢了。仅在用户未手动切换时调整。
 */
watch(
  categoryCounts,
  counts => {
    if (categoryPinned.value || !view.value) return;
    if (counts[category.value] === 0) {
      const other = category.value === 'settings' ? 'worldgen' : 'settings';
      if (counts[other] > 0) category.value = other;
    }
  },
  { immediate: true },
);

const visibleGroups = computed(() => {
  const kw = keyword.value.trim().toLowerCase();
  if (!kw && !onlyChanged.value) return activeGroups.value;

  return activeGroups.value
    .map(g => ({
      ...g,
      options: g.options.filter(o => {
        if (onlyChanged.value && !isChanged(o)) return false;
        if (!kw) return true;
        return o.name.toLowerCase().includes(kw)
          || (o.label || '').toLowerCase().includes(kw)
          || (o.values || []).some(v => (v.label || '').toLowerCase().includes(kw));
      }),
    }))
    .filter(g => g.options.length > 0);
});

/**
 * 预热当前可见选项的图标。
 *
 * 跟着 visibleGroups 走，而不是在 applyView 里按整份数据预热：
 * 那样会把另一个分类、被隐藏的火山岛专项、以及搜索时被过滤掉的项全部拉下来，
 * 一次 180+ 请求，既让 <img loading="lazy"> 形同虚设，也白下载几十张永远不显示的图。
 */
watch(visibleGroups, groups => preloadIcons(groups), { immediate: true });

/* ---------- 取值读写 ---------- */

function isChanged(o) {
  return valueOf(o) !== o.default;
}

function valueOf(o) {
  return o.name in draft ? draft[o.name] : (originalOverrides.value[o.name] ?? o.default);
}

function onOptionChange(o, value) {
  if (frozen.value) return;
  // 世界已生成时，世界生成类选项锁死（与控件 disabled 双保险）
  if (worldgenFrozen.value && worldsettingsCategory.value[o.name] === 'worldgen') return;
  // 回到载入值才算撤销；恢复默认仍需点名写回。
  if (value === (originalOverrides.value[o.name] ?? o.default)) delete draft[o.name];
  else draft[o.name] = value;
}

/* ---------- 加载与保存 ---------- */

function isShardStateDirty(st) {
  if (!st) return false;
  return Object.keys(st.draft).length > 0
    || st.enabled !== st.original.enabled
    || st.worldgenPreset !== st.original.worldgenPreset
    || st.settingsPreset !== st.original.settingsPreset;
}

function syncToCache() {
  const cur = loadedShard.value;
  if (!cur || !shardCache[cur]) return;
  shardCache[cur].enabled = enabled.value;
  shardCache[cur].worldgenPreset = worldgenPreset.value;
  shardCache[cur].settingsPreset = settingsPreset.value;
  shardCache[cur].draft = { ...draft };
}

function restoreFromCache(name) {
  const st = shardCache[name];
  if (!st) return false;
  loadedShard.value = name;
  view.value = st.view;
  originalOverrides.value = { ...st.originalOverrides };
  original.enabled = st.original.enabled;
  original.worldgenPreset = st.original.worldgenPreset;
  original.settingsPreset = st.original.settingsPreset;
  enabled.value = st.enabled;
  worldgenPreset.value = st.worldgenPreset;
  settingsPreset.value = st.settingsPreset;
  Object.keys(draft).forEach(k => delete draft[k]);
  Object.assign(draft, st.draft);
  return true;
}

function clearShardCache() {
  loadedShard.value = '';
  Object.keys(shardCache).forEach(k => delete shardCache[k]);
}

function applyView(name, data) {
  // leveldataoverride.lua 里没有 override_enabled / 预设字段：它本身就是完整定义，
  // 游戏恒定按它生效，所以开关与预设固定为启用、无预设。
  const levelData = !!data.levelDataInUse;
  const initEnabled = levelData ? true : data.config.overrideEnabled;
  const initWorldgen = data.config.worldgenPreset || '';
  const initSettings = data.config.settingsPreset || '';
  // 原配置另存快照：未知键与隐藏项不计为待保存改动，也不会丢失。
  shardCache[name] = {
    view: data,
    originalOverrides: { ...data.config.overrides },
    original: {
      enabled: initEnabled,
      worldgenPreset: initWorldgen,
      settingsPreset: initSettings,
    },
    enabled: initEnabled,
    worldgenPreset: initWorldgen,
    settingsPreset: initSettings,
    draft: {},
  };
  if (shard.value === name) restoreFromCache(name);
}

async function load(target = shard.value) {
  const roomId = rooms.currentId;
  if (!roomId || !target) {
    view.value = null;
    loadedShard.value = '';
    return;
  }
  const seq = ++loadSeq;
  loading.value = true;
  loadError.value = '';
  try {
    const data = await worldApi.get(roomId, target);
    if (rooms.currentId !== roomId) return;
    applyView(target, data);
  } catch (err) {
    if (seq === loadSeq) {
      loadError.value = err.message;
      view.value = null;
      loadedShard.value = '';
    }
  } finally {
    if (seq === loadSeq) loading.value = false;
  }
}

/**
 * 切换要编辑的世界。
 *
 * 分片名可以由用户自定义（主分片也可能不叫 Master），所以不能硬编码默认值，
 * 一律以房间当前的启用分片列表为准；列表为空（还没建房间）时什么也不做。
 */
function selectShard(name) {
  if (!name || name === shard.value) return;
  syncToCache();
  loadedShard.value = '';
  shard.value = name;
  if (restoreFromCache(name)) {
    ++loadSeq;
    loading.value = false;
    loadError.value = '';
    return;
  }
  load(name);
}

/**
 * 预解码一组图标。
 * 切分片/分类会重建整批 OptionField，新的 <img> 要重新解码，
 * 表现为切换后第一帧整片图标空白——先把位图暖进解码缓存就没有这块空白。
 *
 * 只预热当前分类真正会渲染的那批：早先这里是全量预加载（含未展开分类、
 * 被隐藏的火山岛专项），一次 180+ 个请求，既把 loading="lazy" 变成摆设，
 * 也多下几十张永远不会显示的图。
 */
function preloadIcons(groups) {
  const urls = new Set();
  for (const g of groups) {
    for (const o of g.options) {
      const url = iconOf(o.name);
      if (url) urls.add(url);
    }
  }
  for (const url of urls) {
    // eslint-disable-next-line no-undef -- 浏览器全局 Image
    const img = new Image();
    img.src = url;
  }
}

function buildSavePayload(roomId, shardName, st) {
  const levelData = !!st.view?.levelDataInUse;
  return {
    room: roomId,
    shard: shardName,
    overrideEnabled: st.enabled,
    worldgenPreset: st.worldgenPreset,
    settingsPreset: st.settingsPreset,
    overrides: onlySettingsFor(st.view, levelData
      ? { ...st.draft }
      : { ...st.originalOverrides, ...st.draft }),
  };
}

async function save() {
  if (frozen.value) {
    toast('服务器运行中，请先关闭服务器再保存', 'warn');
    return;
  }
  syncToCache();
  const roomId = rooms.currentId;
  const currentShard = shard.value;
  const ordered = [
    ...rooms.shards.filter(s => s in shardCache),
    ...Object.keys(shardCache).filter(s => !rooms.shards.includes(s)),
  ];
  const targets = ordered.filter(s => isShardStateDirty(shardCache[s]));
  if (!targets.length) return;

  saving.value = true;
  const { ok, data } = await guard(
    async () => {
      let needRestart = false;
      for (const s of targets) {
        const res = await worldApi.save(buildSavePayload(roomId, s, shardCache[s]));
        if (res && res.needRestart) needRestart = true;
        if (s !== currentShard) delete shardCache[s];
      }
      return { needRestart };
    },
    { success: '世界配置已保存' },
  );
  saving.value = false;
  if (!ok) return;
  // 游戏只在启动时读这个文件，运行中的分片感知不到改动
  if (data && data.needRestart) toast('有分片正在运行，重启后才会生效');
  await load(currentShard);
}

function resetAll() {
  for (const g of displayGroups.value) {
    if (worldgenFrozen.value && g.category === 'worldgen') continue;
    for (const o of g.options) onOptionChange(o, o.default);
  }
  enabled.value = original.enabled;
  worldgenPreset.value = original.worldgenPreset;
  settingsPreset.value = original.settingsPreset;
}

/** leveldata 只提交规则增量；普通文件整表保存，必须保留只读生成原值。 */
function onlySettingsFor(viewData, overrides) {
  if (!viewData?.levelDataInUse) return overrides;
  const categoryOf = {};
  for (const g of viewData.groups || []) {
    for (const o of g.options) categoryOf[o.name] = g.category;
  }
  const out = {};
  for (const [k, v] of Object.entries(overrides)) {
    if (categoryOf[k] !== 'worldgen') out[k] = v;
  }
  return out;
}

const worldsettingsCategory = computed(() => {
  const out = {};
  for (const g of view.value?.groups || []) {
    for (const o of g.options) out[o.name] = g.category;
  }
  return out;
});
/* ---------- 生命周期 ---------- */

watch(() => rooms.currentId, () => {
  clearShardCache();
  shard.value = '';
  selectShard(rooms.shards[0]);
});
// 分片被停用或删除后，当前选中的世界会失效，回落到列表第一个
watch(() => rooms.shards.join(','), () => {
  for (const k of Object.keys(shardCache)) {
    if (!rooms.shards.includes(k)) delete shardCache[k];
  }
  if (rooms.shards.includes(shard.value)) return;
  loadedShard.value = '';
  shard.value = '';
  selectShard(rooms.shards[0]);
});

onMounted(() => selectShard(rooms.shards[0]));
</script>

<template>
  <!-- 必须保持单一根节点：App.vue 用 <Transition> 包裹视图组件 -->
  <div class="stack">
    <UiCard>
      <template #title>世界配置</template>
      <template #hint>{{ rooms.current ? rooms.current.name + ' · ' + shardHint : '' }}</template>
      <template #actions>
        <button
          v-for="s in rooms.shards"
          :key="s"
          class="btn btn-sm shard-tab transition-none"
          :class="{ 'btn-primary': s === shard }"
          :aria-pressed="s === shard"
          @click="selectShard(s)"
        >
          {{ labelOf(s) }}
        </button>
      </template>

      <UiSkeleton v-if="!rooms.loaded" variant="form" :rows="2" />
      <UiEmpty v-else-if="!rooms.current" icon="globe" text="请先在「房间」页新建一个房间" />
      <!-- 只在首次加载时铺骨架；切分片保留旧内容到新数据回来，避免整页重建闪一下 -->
      <UiSkeleton v-else-if="loading && !view" variant="form" :rows="2" />
      <p v-else-if="loadError" class="field-tip">读取失败：{{ loadError }}</p>

      <div v-else-if="view" class="world-config-body">
        <!-- 游戏只在启动时读配置，运行中修改不会生效 -->
        <div v-if="frozen" class="notice">
          <Icon name="info" size="16" />
          <span>
            房间正在运行中，配置已冻结。现在修改不会生效，
            请先在「总览」页<strong>关闭服务器</strong>后再修改配置。
          </span>
        </div>

        <div v-if="!levelDataInUse" class="world-presets">
          <UiField block label="世界生成预设" for-id="w-worldgen" tip="决定地图长什么样">
            <Select v-model="worldgenPreset" :disabled="worldgenFrozen">
              <SelectTrigger id="w-worldgen" class="w-full">
                <SelectValue :placeholder="worldgenPreset || '选择预设'" />
              </SelectTrigger>
              <SelectContent position="popper">
                <SelectGroup>
                  <SelectItem v-for="p in worldgenPresets" :key="p.data" :value="p.data">
                    {{ p.label ? p.label + '（' + p.data + '）' : p.data }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </UiField>

          <UiField block label="世界规则预设" for-id="w-settings" tip="决定游戏规则">
            <Select v-model="settingsPreset" :disabled="frozen">
              <SelectTrigger id="w-settings" class="w-full">
                <SelectValue :placeholder="settingsPreset || '选择预设'" />
              </SelectTrigger>
              <SelectContent position="popper">
                <SelectGroup>
                  <SelectItem v-for="p in settingsPresets" :key="p.data" :value="p.data">
                    {{ p.label ? p.label + '（' + p.data + '）' : p.data }}
                  </SelectItem>
                </SelectGroup>
              </SelectContent>
            </Select>
          </UiField>
        </div>

        <label v-if="!levelDataInUse" class="world-enabled">
          <Switch v-model="enabled" aria-label="启用本文件" :disabled="frozen" />
          <span>启用本文件</span>
        </label>

        <div v-if="!enabled && !levelDataInUse" class="notice">
          <span>已关闭 <code>override_enabled</code>：游戏仍会读取本文件，但不会应用里面的设置。</span>
        </div>
        <div v-if="worldGenerated" class="notice">
          <Icon name="info" size="16" />
          <span v-if="levelDataInUse">
            该存档的配置由游戏自己的 <code>leveldataoverride.lua</code> 保存，
            当前分片是 <strong>{{ worldLabel }}</strong>。
            <strong>世界规则</strong>可直接在这里改；<strong>世界生成</strong>只在建图时生效，
            地图已生成后不可修改，只做展示。
          </span>
          <span v-else>
            该房间<strong>已生成过世界</strong>：世界生成类配置已冻结——
            这些选项只在生成世界时读取，改了也不会重建已有地图。
            如需调整地形、资源分布等，请新建房间重新生成，或另行导入存档。
          </span>
        </div>

        <div v-if="presetMismatch" class="notice">
          <Icon name="alert" size="16" />
          <span>
            世界预设与分片类型不一致：当前预设是 <strong>{{ worldgenPreset || '（未指定）' }}</strong>，
            而这个分片是 <strong>{{ shardHint }}</strong>。
            <template v-if="worldGenerated">
              世界已经生成，改预设不会重建地图——要纠正只能删除该分片的存档后重新生成。
            </template>
            <template v-else>
              现在启动会严格按预设生成，请先把预设改成与该类型匹配的值。
            </template>
          </span>
        </div>

        <p class="path-line">{{ view.file }}</p>
      </div>
    </UiCard>

    <UiCard v-if="view && !loadError" class="world-options-card" :aria-busy="loading">
      <template #title>选项</template>
      <template #hint>共 {{ totalOptions }} 项</template>
      <template #actions>
        <span class="world-count">
          {{ changedCount ? '已改 ' + changedCount + ' 项' : '未做改动' }}
        </span>
        <button class="btn btn-sm" :disabled="!canReset || frozen" @click="resetAll">重置</button>
        <button class="btn btn-sm btn-primary" :disabled="!dirty || frozen || saving" @click="save">
          {{ saving ? '保存中…' : '保存' }}
        </button>
      </template>

      <div class="world-toolbar">
        <div class="seg">
          <button
            class="seg-btn"
            :class="{ 'is-active': category === 'settings' }"
            :aria-pressed="category === 'settings'"
            type="button"
            @click="pickCategory('settings')"
          >
            世界规则 <span class="seg-count">{{ categoryCounts.settings }}</span>
          </button>
          <button
            class="seg-btn"
            :class="{ 'is-active': category === 'worldgen' }"
            :aria-pressed="category === 'worldgen'"
            type="button"
            @click="pickCategory('worldgen')"
          >
            世界生成 <span class="seg-count">{{ categoryCounts.worldgen }}</span>
          </button>
        </div>
      </div>

      <div class="world-toolbar world-toolbar-sub">
        <Input v-model="keyword" placeholder="搜索选项（中英文均可）" class="world-search" />
        <label class="inline-flex items-center gap-2 cursor-pointer select-none text-sm text-foreground">
          <Switch v-model="onlyChanged" />
          <span>只看已改动</span>
        </label>
      </div>

      <UiEmpty
        v-if="!visibleGroups.length"
        icon="inbox"
        :text="onlyChanged ? '还没有改动过任何选项' : '没有匹配的选项'"
      />

      <template v-else>
        <!-- 服务器运行中冻结：改了也不生效，引导用户先关服 -->
        <div v-if="frozen" class="notice" style="margin-bottom: 12px">
          <Icon name="info" size="16" />
          <span>
            服务器运行中，选项已冻结。请先<strong>关闭服务器</strong>再修改。
          </span>
        </div>

        <OptionGroupPanel
          v-for="g in visibleGroups"
          :key="g.category + '/' + g.id"
          :group="g"
          :value-of="valueOf"
          :is-changed="isChanged"
          :disabled="frozen || (worldGenerated && g.category === 'worldgen')"
          @change="onOptionChange"
        />
      </template>
    </UiCard>
  </div>
</template>
