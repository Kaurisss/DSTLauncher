<script setup>
import { computed, onUnmounted, ref, useTemplateRef, watch } from 'vue';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import IconAction from '@/components/ui/IconAction.vue';
import { Skeleton } from '@/components/ui/skeleton';
import { migrationApi } from '@/lib/api.js';
import { fmtSize, fmtTime } from '@/lib/format.js';
import { confirmAction } from '@/lib/confirm.js';
import { useRoomsStore } from '@/stores/rooms.js';

const rooms = useRoomsStore();
const open = ref(false);
const busy = ref(false);
const uploading = ref(false);
const error = ref('');
const job = ref(null);
const applyPlan = ref(false);
const fileInput = useTemplateRef('fileInput');
let jobId = window.sessionStorage.getItem('dst-migration-job') || '';
let timer;
let disposed = false;
const working = computed(() => busy.value || job.value?.state === 'working');
const allStopped = computed(() => rooms.list.every(room => !room.running));
const importing = computed(() => job.value?.kind === 'import');
const stage = computed(() => uploading.value ? '正在上传迁移包…' : job.value?.kind === 'upload' ? '正在校验迁移包…' : importing.value ? '正在创建新房间…' : '正在生成迁移包…');

watch(open, value => { clearTimeout(timer); if (value && jobId) poll(); });
onUnmounted(() => { disposed = true; clearTimeout(timer); });

async function poll() {
  clearTimeout(timer);
  const id = jobId;
  try {
    const data = await migrationApi.status(id);
    if (disposed || id !== jobId) return;
    job.value = data;
    error.value = '';
    if (data.kind === 'import' && data.state === 'ready') await rooms.load();
    if (data.state === 'working' && open.value) timer = setTimeout(poll, 750);
  } catch (err) { if (!disposed && id === jobId) error.value = err.message; }
}

async function begin(action) {
  busy.value = true;
  error.value = '';
  try {
    const result = await action();
    jobId = result.id;
    window.sessionStorage.setItem('dst-migration-job', jobId);
    await poll();
  } catch (err) { error.value = err.message; }
  finally { busy.value = false; uploading.value = false; }
}

async function upload(event) {
  const file = event.target.files[0];
  event.target.value = '';
  if (!file) return;
  if (file.size > 8 * 1024 ** 3) { error.value = '迁移包不能超过 8 GiB'; return; }
  uploading.value = true;
  await begin(() => migrationApi.upload(file));
}

async function apply() {
  if (!await confirmAction(`导入为 ${job.value.preview.rooms.length} 个新房间？${applyPlan.value ? '\n\n同时替换当前自动维护计划。' : ''}\n\n导入后请检查房间设置并下载所需模组。`)) return;
  await begin(() => migrationApi.apply(jobId, applyPlan.value));
}

async function reset() {
  busy.value = true;
  try {
    if (jobId) await migrationApi.remove(jobId);
    jobId = '';
    window.sessionStorage.removeItem('dst-migration-job');
    job.value = null;
    applyPlan.value = false;
    error.value = '';
  } catch (err) { error.value = err.message; }
  finally { busy.value = false; }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child><IconAction icon="migrate" variant="outline" show-label="sm" label="完整迁移" /></DialogTrigger>
    <DialogContent class="max-h-[85dvh] overflow-y-auto sm:max-w-xl" :close-disabled="uploading" @escape-key-down="uploading && $event.preventDefault()" @interact-outside="uploading && $event.preventDefault()">
      <DialogHeader>
        <DialogTitle>完整迁移</DialogTitle>
        <DialogDescription>迁移全部房间的存档、配置、玩家名单和配置方案，保留游戏令牌与房间密码。导出和导入前需停止全部房间。</DialogDescription>
      </DialogHeader>
      <p class="text-sm text-muted-foreground">游戏程序、模组文件和面板登录设置需在目标机器单独配置。</p>
      <p v-if="!allStopped" role="status" class="text-sm text-muted-foreground">仍有房间运行，请先停止。</p>
      <p v-if="error || job?.error" role="alert" class="break-words text-sm text-destructive">{{ error || job.error }}</p>
      <div v-if="working" class="grid gap-3" role="status">
        <p class="text-sm">{{ stage }}</p>
        <p v-if="job?.total" class="text-sm text-muted-foreground tabular-nums">{{ job.done }} / {{ job.total }}</p>
        <div class="grid gap-2">
          <Skeleton class="h-4 w-48" />
          <Skeleton class="h-4 w-36" />
        </div>
      </div>
      <template v-if="job?.preview">
        <div class="flex flex-wrap gap-x-4 gap-y-2 text-sm text-muted-foreground">
          <time>{{ fmtTime(job.preview.createdAt) }}</time><span>{{ job.preview.files }} 个文件</span><span>{{ fmtSize(job.preview.bytes) }}</span><span>{{ job.preview.profiles }} 个配置方案</span>
        </div>
        <ul class="grid gap-3" aria-label="迁移房间清单">
          <li v-for="(room, index) in job.preview.rooms" :key="index" class="grid gap-1 text-sm">
            <span class="break-words font-medium">{{ room.name }}</span>
            <span class="break-words text-muted-foreground">{{ room.shards.map(shard => shard.name + (shard.disabled ? '（停用）' : '')).join('、') }}</span>
          </li>
        </ul>
        <label v-if="job.kind === 'upload' && job.state === 'ready' && job.preview.hasPlan" class="flex items-center gap-2 text-sm"><Checkbox v-model="applyPlan" :disabled="working" />应用迁移包中的自动维护计划</label>
        <p v-if="importing && job.state === 'ready'" role="status" class="text-sm">已创建 {{ job.roomIds.length }} 个房间。请检查设置并下载所需模组，再启动游戏。</p>
      </template>
      <div class="flex flex-wrap gap-2">
        <template v-if="!job && !jobId">
          <Button :disabled="working || !allStopped || !rooms.list.length" @click="begin(migrationApi.export)">导出全部房间</Button>
          <Button variant="outline" :disabled="working" @click="fileInput.click()">上传迁移包</Button>
        </template>
        <Button v-if="job?.kind === 'upload' && job.state === 'ready'" :disabled="working || !allStopped" @click="apply">导入为新房间</Button>
        <Button v-if="job?.kind === 'export' && job.state === 'ready'" as-child><a :href="migrationApi.downloadUrl(job.id)" download>下载迁移包</a></Button>
        <Button v-if="error && jobId" variant="outline" :disabled="busy" @click="poll">重新查询</Button>
        <Button v-if="jobId && job?.state !== 'working'" variant="outline" :disabled="busy" @click="reset">清除临时包</Button>
      </div>
      <input ref="fileInput" type="file" class="hidden" accept=".zip,application/zip" aria-label="上传完整迁移包" @change="upload">
    </DialogContent>
  </Dialog>
</template>
