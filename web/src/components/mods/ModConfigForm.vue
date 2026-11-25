<script setup>
// 模组配置表单：按 modinfo.lua 声明的 schema 动态渲染。
//
// 每个配置项都是独立接口调用（后端一次只接受一个键），
// 因此保存是逐个提交；某项失败会立即中断并提示，避免部分写入却显示成功。

import { computed, onMounted, reactive, ref } from 'vue';

import { Skeleton } from '@/components/ui/skeleton';
import IconAction from '@/components/ui/IconAction.vue';
import { Input } from '@/components/ui/input';
import {
  Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select';
import { modApi } from '@/lib/api.js';
import { toast } from '@/lib/toast.js';

const props = defineProps({
  roomId: { type: Number, required: true },
  shard: { type: String, required: true },
  mod: { type: Object, required: true },
  /** 服务器运行中冻结：控件只读、禁止保存 */
  disabled: { type: Boolean, default: false },
});
const emit = defineEmits(['saved']);

const schema = ref([]);
const values = reactive({});
const initialValues = reactive({});
const loading = ref(true);
const loadError = ref('');
const saving = ref(false);
const dirty = computed(() => schema.value.some(o => values[o.name] !== initialValues[o.name]));
defineExpose({ dirty, saving });

function reset() {
  for (const o of schema.value) values[o.name] = initialValues[o.name];
}

/** 配置项的类型：有 options 就是下拉框，否则看 type */
function fieldType(option) {
  if ((option.choices && option.choices.length) || (option.options && option.options.length)) return 'select';
  if (option.type === 'boolean') return 'boolean';
  if (option.type === 'number') return 'number';
  return 'string';
}

function choiceList(option) {
  if (option.choices && option.choices.length) {
    return option.choices.map(c => ({
      data: String(c.data),
      label: c.description || String(c.data),
    }));
  }
  return (option.options || []).map(v => ({ data: String(v), label: String(v) }));
}

/**
 * 把后端解析出的值归一成表单期望的类型。
 * Lua 里 true 可能被解析成字符串 "true"，直接丢给 <select> 会选不中任何选项。
 */
function normalize(option, raw) {
  const type = fieldType(option);
  if (type === 'boolean') {
    if (typeof raw === 'string') return raw.toLowerCase() === 'true';
    return !!raw;
  }
  if (type === 'number') {
    const n = Number(raw);
    return Number.isFinite(n) ? n : 0;
  }
  return raw == null ? '' : String(raw);
}

onMounted(async () => {
  try {
    const list = await modApi.schema(props.roomId, props.mod.id);
    schema.value = list || [];
    const current = props.mod.config || {};
    schema.value.forEach(o => {
      if (!(o.name in current) && o.defaultUnresolved) return;
      const raw = (o.name in current) ? current[o.name] : o.default;
      values[o.name] = normalize(o, raw);
      initialValues[o.name] = values[o.name];
    });
  } catch (err) {
    loadError.value = err.message;
  } finally {
    loading.value = false;
  }
});

const hasOptions = computed(() => schema.value.length > 0);

async function save() {
  if (saving.value) return;
  if (props.disabled) {
    toast('服务器运行中，请先关闭服务器再修改配置', 'warn');
    return;
  }
  saving.value = true;
  const { roomId, shard, mod } = props;
  try {
    for (const o of schema.value) {
      const type = fieldType(o);
      const raw = values[o.name];
      if (raw === undefined || (o.defaultUnresolved && raw === initialValues[o.name])) continue;
      // 后端按 JSON 字面量解析 value 参数，数字/布尔要原样传
      let value;
      if (type === 'number') value = JSON.stringify(Number(raw) || 0);
      else if (type === 'boolean') value = raw === true || raw === 'true' ? 'true' : 'false';
      else value = JSON.stringify(raw == null ? '' : String(raw));

      await modApi.setConfig(roomId, mod.id, o.name, value, shard);
      initialValues[o.name] = values[o.name];
    }
    toast('模组配置已保存，重启后生效');
    emit('saved');
  } catch (err) {
    toast(err.message, 'err');
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <form class="flex min-h-0 flex-1 flex-col" @submit.prevent="save">
    <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 sm:p-6">
      <div v-if="loading" role="status" aria-label="加载中" class="grid gap-3">
        <div v-for="i in 5" :key="i" class="opt mb-0">
          <Skeleton class="ml-auto h-4 w-28 max-w-full" />
          <Skeleton class="h-9 w-full" />
        </div>
      </div>

      <p v-else-if="loadError" class="text-sm text-destructive" role="alert">读取配置项失败：{{ loadError }}</p>

      <p v-else-if="!hasOptions" class="text-xs text-muted-foreground">该模组没有可配置项。</p>

      <template v-else>
        <div v-for="o in schema" :key="o.name" class="opt">
          <label :for="`opt_${mod.id}_${o.name}`">{{ o.label || o.name }}</label>

          <Select
            v-if="fieldType(o) === 'select'"
            :model-value="values[o.name] !== undefined ? String(values[o.name]) : ''"
            :disabled="disabled || saving"
            @update:model-value="(v) => values[o.name] = v"
          >
            <SelectTrigger :id="`opt_${mod.id}_${o.name}`" class="min-w-36">
              <SelectValue :placeholder="String(values[o.name] ?? '请选择')" />
            </SelectTrigger>
            <SelectContent position="popper">
              <SelectGroup>
                <SelectItem v-for="op in choiceList(o)" :key="op.data" :value="op.data">
                  {{ op.label }}
                </SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>

          <Select
            v-else-if="fieldType(o) === 'boolean'"
            :model-value="values[o.name] === true ? 'true' : (values[o.name] === false ? 'false' : '')"
            :disabled="disabled || saving"
            @update:model-value="(v) => values[o.name] = (v === 'true')"
          >
            <SelectTrigger :id="`opt_${mod.id}_${o.name}`" class="min-w-36">
              <SelectValue :placeholder="String(values[o.name] ?? '请选择')" />
            </SelectTrigger>
            <SelectContent position="popper">
              <SelectGroup>
                <SelectItem value="true">开启</SelectItem>
                <SelectItem value="false">关闭</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>

          <Input
            v-else-if="fieldType(o) === 'number'"
            :id="`opt_${mod.id}_${o.name}`"
            v-model="values[o.name]"
            type="number"
            :disabled="disabled || saving"
            :min="o.min !== undefined && o.min !== '' ? o.min : undefined"
            :max="o.max !== undefined && o.max !== '' ? o.max : undefined"
          />

          <Input
            v-else
            :id="`opt_${mod.id}_${o.name}`"
            v-model="values[o.name]"
            type="text"
            :disabled="disabled || saving"
          />

          <div v-if="o.description" class="opt-desc">{{ o.description }}</div>
        </div>
      </template>
    </div>
    <div v-if="!loading && !loadError && hasOptions" class="flex shrink-0 items-center justify-end gap-2 border-t bg-background p-4">
      <span v-if="dirty" role="status" class="mr-auto text-sm text-amber-700 dark:text-amber-400">未保存</span>
      <IconAction type="button" variant="ghost" show-label icon="undo" label="撤销修改" :disabled="!dirty || saving || disabled" @click="reset" />
      <IconAction type="submit" show-label icon="save" :label="saving ? '保存中…' : '保存配置'" :disabled="saving || disabled" />
    </div>
  </form>
</template>
