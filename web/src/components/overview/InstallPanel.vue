<script setup>
// 安装与位置配置沿用现有 store/API；这里只替换展示与确认交互。
import { computed, onMounted, ref, watch } from 'vue';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader,
  AlertDialogTitle, AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Field, FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select';
import { Separator } from '@/components/ui/separator';
import UiProgress from '@/components/ui/UiProgress.vue';
import { useRoomsStore } from '@/stores/rooms.js';
import { useSystemStore } from '@/stores/system.js';
import { guard, toast } from '@/lib/toast.js';

const system = useSystemStore();
const rooms = useRoomsStore();
const busy = computed(() => rooms.list.some(r => r.running));
const installRequested = ref(false);
const percent = computed(() => {
  const match = /([\d.]+)\s*%/.exec(system.progress || '');
  if (!match) return -1;
  const value = Number(match[1]);
  return Number.isFinite(value) && value >= 0 && value <= 100 ? value : -1;
});
const stage = computed(() => (system.progress || '准备中…').replace(/\s*[\d.]+\s*%/, ''));
const server = computed(() => system.server);
const locked = computed(() => !!server.value?.locked || system.installing || rooms.starting);
const draftDir = ref('');
const draftArch = ref('auto');
const ARCH_LABEL = { auto: '自动（优先 64 位）', 64: '64 位', 32: '32 位' };

watch(server, value => {
  if (!value) return;
  draftArch.value = value.archPref || 'auto';
  draftDir.value = value.pinned ? value.root : '';
}, { immediate: true });

async function apply(dir) {
  const { ok } = await guard(
    () => system.saveServer(dir, draftArch.value),
    { success: '服务器文件位置已更新' },
  );
  if (ok) toast('新设置会在下次启动分片时生效');
}

function applyArch(event) {
  draftArch.value = event.target.value;
  return apply(draftDir.value);
}

function useCandidate(root) {
  draftDir.value = root;
  return apply(root);
}

function resetAuto() {
  draftDir.value = '';
  return apply('');
}

async function install() {
  if (installRequested.value) return;
  installRequested.value = true;
  try {
    await guard(() => system.startInstall(), { success: '安装任务已启动' });
  } finally {
    installRequested.value = false;
  }
}

function update() {
  guard(() => system.update(), { success: '更新任务已启动' });
}

onMounted(async () => {
  await guard(() => system.refresh());
  await guard(() => system.loadServer());
});
</script>

<template>
  <div class="flex flex-col gap-4">
    <template v-if="system.installing">
      <div class="flex items-center justify-between gap-4">
        <Badge variant="secondary">安装中</Badge>
        <span v-if="percent >= 0" class="text-sm font-semibold tabular-nums" aria-live="polite">{{ percent }}%</span>
      </div>
      <div class="flex flex-col gap-2">
        <p class="text-sm font-medium">{{ stage }}</p>
        <UiProgress :value="percent" :aria-label="stage" />
        <p v-if="percent >= 0" class="text-sm text-muted-foreground">当前阶段进度，切换阶段时重新计算。</p>
      </div>
      <p class="text-sm text-muted-foreground">
        {{ rooms.starting ? '安装成功后自动开服，请保持此页面打开' : '完成后会自动刷新，无需手动操作' }}
      </p>
    </template>

    <template v-else-if="system.installed">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="text-sm font-medium">{{ system.external ? '已复用已安装的服务器' : '服务器已安装' }}</p>
        <Badge variant="outline">{{ system.arch }} 位</Badge>
      </div>
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
        <dt class="text-muted-foreground">来源</dt>
        <dd>{{ system.source || '未知' }}</dd>
        <dt class="text-muted-foreground">游戏版本</dt>
        <dd>{{ system.version || '未知' }}</dd>
      </dl>
      <p class="break-all text-sm text-muted-foreground">{{ system.root }}</p>
      <p v-if="system.external" class="text-sm text-muted-foreground">
        更新请在 Steam 中更新「Don't Starve Together Dedicated Server」工具，面板会自动使用最新版本。
      </p>
      <Button v-else variant="outline" size="sm" class="self-start" :disabled="busy" @click="update">检查更新</Button>
      <Alert v-if="busy">
        <AlertTitle>更新前请先停止所有分片</AlertTitle>
        <AlertDescription>运行中的服务器会占用文件，更新可能失败或只更新一半。</AlertDescription>
      </Alert>
    </template>

    <template v-else>
      <div class="flex items-center gap-2">
        <Badge variant="outline">未安装</Badge>
        <p class="text-sm font-medium">未找到专用服务器</p>
      </div>
      <p class="text-sm text-muted-foreground">
        点击房间或分片的「安装并启动」会自动下载安装服务器（约 4.5GB），完成后继续开服。
        已有安装也可在下方指定目录，无需重复下载。
      </p>
      <AlertDialog>
        <AlertDialogTrigger as-child>
          <Button variant="outline" size="sm" class="self-start" :disabled="busy || rooms.starting || installRequested">
            {{ system.error ? '重试安装（支持续传）' : '仅安装服务器（约 4.5GB）' }}
          </Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>安装 DST 专用服务器？</AlertDialogTitle>
            <AlertDialogDescription>约需下载 4.5GB，耗时取决于网络和磁盘速度。本次仅安装服务器，不会自动开服；已有下载文件会保留并用于续传。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction :disabled="installRequested" @click="install">开始安装</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </template>

    <Alert v-if="system.error" variant="destructive">
      <AlertTitle>安装或更新失败</AlertTitle>
      <AlertDescription>{{ system.error }}</AlertDescription>
    </Alert>

    <template v-if="server">
      <Separator />
      <dl v-if="server.ugcDir" class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 text-sm">
        <dt class="text-muted-foreground">模组目录</dt>
        <dd class="break-all font-mono text-xs leading-5">{{ server.ugcDir }}</dd>
      </dl>
      <details>
        <summary class="cursor-pointer text-sm font-medium">更改服务器文件位置</summary>
        <div class="mt-4 flex flex-col gap-4">
          <p v-if="locked" class="text-sm text-muted-foreground">开服、安装/更新、模组下载或分片运行期间不能更改位置。</p>
          <FieldGroup>
            <Field :data-disabled="locked || undefined">
              <FieldLabel for="srv_dir">安装目录</FieldLabel>
              <Input id="srv_dir" v-model="draftDir" :disabled="locked" placeholder="留空 = 自动探测 Steam 库" />
              <FieldDescription>填「Don't Starve Together Dedicated Server」那一层，也可以粘贴里面的 bin 目录。</FieldDescription>
              <Button size="sm" class="self-start" :disabled="locked" @click="apply(draftDir)">应用</Button>
            </Field>
            <Field :data-disabled="locked || undefined">
              <FieldLabel for="srv_arch">架构</FieldLabel>
              <NativeSelect id="srv_arch" v-model="draftArch" :disabled="locked" @change="applyArch">
                <NativeSelectOption v-for="arch in server.archOptions" :key="arch" :value="arch">{{ ARCH_LABEL[arch] || arch }}</NativeSelectOption>
              </NativeSelect>
              <FieldDescription>64 位可突破 32 位进程约 2GB 的内存上限，模组较多时更稳。</FieldDescription>
            </Field>
          </FieldGroup>
          <template v-if="server.candidates?.length">
            <p class="text-sm text-muted-foreground">自动探测到的安装：</p>
            <div v-for="candidate in server.candidates" :key="candidate.root" class="flex flex-wrap items-center gap-2">
              <span class="min-w-0 flex-1 break-all text-sm">{{ candidate.root }}</span>
              <Badge variant="secondary">{{ candidate.source }}</Badge>
              <Button variant="outline" size="sm" :disabled="locked || candidate.root === server.root" @click="useCandidate(candidate.root)">
                {{ candidate.root === server.root ? '使用中' : '使用' }}
              </Button>
            </div>
          </template>
          <div class="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" :disabled="locked || !server.pinned" @click="resetAuto">恢复自动探测</Button>
            <span class="text-sm text-muted-foreground">{{ server.pinned ? '当前为手动指定' : '当前为自动探测' }}</span>
          </div>
          <p class="break-all text-sm text-muted-foreground">配置保存在 {{ server.configPath }}，重启面板后依然有效。</p>
        </div>
      </details>
    </template>
  </div>
</template>
