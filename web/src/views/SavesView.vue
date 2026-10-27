<script setup>
// 存档管理：列出磁盘上的全部存档目录（Cluster_*）。
//
// 删除房间只删定义、不删存档，所以这里会看到一批「房间已删除」的孤儿存档。
// 只有孤儿能在这里删——房间还在的存档必须先到「房间」页删房间，
// 否则就太容易把正在玩的存档误删了。

import { computed, h, onMounted, ref } from 'vue';
import {
  createFilteredRowModel, createPaginatedRowModel, createSortedRowModel,
  filterFn_includesString, FlexRender, globalFilteringFeature,
  rowPaginationFeature, rowSortingFeature, sortFn_alphanumeric, sortFn_basic,
  tableFeatures, useTable,
} from '@tanstack/vue-table';

import UiCard from '@/components/ui/UiCard.vue';
import UiToolbar from '@/components/ui/UiToolbar.vue';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import Icon from '@/components/ui/Icon.vue';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import UiLoading from '@/components/ui/UiLoading.vue';
import { Spinner } from '@/components/ui/spinner';
import {
  Table, TableBody, TableCaption, TableCell, TableEmpty,
  TableHead, TableHeader, TableRow,
} from '@/components/ui/table';
import { savesApi } from '@/lib/api.js';
import { guard, toast } from '@/lib/toast.js';
import { confirmAction } from '@/lib/confirm.js';
import { fmtSize, fmtTime } from '@/lib/format.js';

const items = ref([]);
const root = ref('');
const loading = ref(false);
const loadError = ref('');

const totalSize = computed(() => items.value.reduce((n, x) => n + (x.size || 0), 0));
const orphans = computed(() => items.value.filter(x => !x.roomExists));
const orphanSize = computed(() => orphans.value.reduce((n, x) => n + (x.size || 0), 0));

const columns = [
  {
    accessorKey: 'cluster', header: '存档目录', sortFn: 'alphanumeric',
    cell: ({ row }) => h('div', { class: 'flex items-center gap-2' }, [
      h('code', {}, row.original.cluster),
      row.original.running ? h(Badge, { variant: 'secondary' }, () => '运行中') : null,
    ]),
  },
  {
    id: 'room', header: '房间',
    accessorFn: item => item.roomExists ? item.roomName : '房间已删除',
    cell: ({ row, getValue }) => h('span', {
      class: row.original.roomExists ? '' : 'text-amber-700 dark:text-amber-400',
    }, getValue()),
  },
  { id: 'shards', header: '分片', accessorFn: item => item.hasSave ? shardNames(item) : '未生成世界' },
  { accessorKey: 'size', header: '大小', enableGlobalFilter: false, cell: ({ getValue }) => fmtSize(getValue()) },
  {
    id: 'modTime', header: '最后修改', enableGlobalFilter: false,
    accessorFn: item => new Date(item.modTime).getTime(),
    cell: ({ row }) => fmtTime(row.original.modTime),
  },
  {
    id: 'actions', header: '操作', enableSorting: false,
    cell: ({ row }) => h(Button, {
      variant: 'destructive', size: 'sm',
      disabled: row.original.roomExists || row.original.running,
      title: row.original.roomExists ? '房间还在，请先在房间页删除房间'
        : (row.original.running ? '正在运行，请先停服' : '永久删除这份存档'),
      onClick: () => remove(row.original),
    }, () => '删除'),
  },
];

const table = useTable({
  features: tableFeatures({
    globalFilteringFeature, rowSortingFeature, rowPaginationFeature,
    filteredRowModel: createFilteredRowModel(),
    sortedRowModel: createSortedRowModel(),
    paginatedRowModel: createPaginatedRowModel(),
    filterFns: { includesString: filterFn_includesString },
    sortFns: { basic: sortFn_basic, alphanumeric: sortFn_alphanumeric },
  }),
  get data() { return items.value; },
  columns,
  getRowId: item => item.cluster,
  defaultColumn: { sortFn: 'basic' },
  globalFilterFn: 'includesString',
  initialState: { pagination: { pageIndex: 0, pageSize: 10 } },
});

async function load() {
  loading.value = true;
  loadError.value = '';
  try {
    const data = (await savesApi.list()) || {};
    items.value = data.items || [];
    root.value = data.root || '';
  } catch (err) {
    loadError.value = err.message;
    items.value = [];
  } finally {
    loading.value = false;
  }
}

onMounted(load);

async function copyRoot() {
  if (!window.navigator.clipboard) {
    toast('当前连接不支持剪贴板访问，请手动复制目录路径', 'err');
    return;
  }
  await guard(() => window.navigator.clipboard.writeText(root.value), { success: '目录路径已复制' });
}

function shardNames(item) {
  return (item.shards || []).map(s => s.name).join('、') || '—';
}

async function remove(item) {
  if (item.roomExists) {
    toast('该房间还在面板里，请先在「房间」页删除房间', 'warn');
    return;
  }
  if (item.running) {
    toast('该存档仍被运行中的进程占用，请先停服', 'warn');
    return;
  }

  const ok = await confirmAction(
    `永久删除 ${item.cluster}？\n\n`
    + `分片：${shardNames(item)}\n`
    + `占用：${fmtSize(item.size)}\n`
    + `最后修改：${fmtTime(item.modTime)}\n\n`
    + '删除后无法恢复。想保留的话请先取消，把上面显示的目录整个复制到别处。',
  );
  if (!ok) return;

  const { ok: done } = await guard(
    () => savesApi.remove(item.cluster),
    { success: '存档已删除' },
  );
  if (done) load();
}
</script>

<template>
  <UiCard flush fill>
    <UiToolbar aria-label="存档工具栏" leading-class="min-w-full flex-wrap lg:min-w-0">
      <Input class="min-w-32 max-w-64 flex-1" aria-label="搜索存档" placeholder="搜索存档、房间或分片" :model-value="table.atoms.globalFilter.get() ?? ''" @update:model-value="table.setGlobalFilter" />
      <Badge variant="secondary" class="shrink-0 whitespace-nowrap tabular-nums">共 {{ items.length }} 个存档（{{ fmtSize(totalSize) }}）</Badge>
      <Badge v-if="orphans.length" variant="outline" class="shrink-0 whitespace-nowrap text-muted-foreground">孤儿存档 {{ orphans.length }} 个 · {{ fmtSize(orphanSize) }}</Badge>
      <Tooltip v-if="root">
        <TooltipTrigger as-child>
          <Button variant="ghost" size="sm" class="text-muted-foreground" aria-label="复制存档根目录" @click="copyRoot"><Icon name="folder" size="14" />目录路径</Button>
        </TooltipTrigger>
        <TooltipContent class="max-w-sm break-all"><code class="bg-transparent p-0">{{ root }}</code></TooltipContent>
      </Tooltip>
      <template #actions>
        <Button variant="outline" size="sm" :disabled="loading" @click="load"><Spinner v-if="loading && items.length" aria-hidden="true" />刷新</Button>
      </template>
    </UiToolbar>

    <div data-slot="page-scroll" class="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain p-4 pt-1 sm:px-6 sm:pb-6" tabindex="0" role="region" aria-label="存档列表">
      <div class="grid min-w-0 gap-4">
        <div class="min-w-0 overflow-hidden rounded-md border" :aria-busy="loading">
          <Table class="min-w-[760px]">
            <TableCaption class="sr-only">存档列表</TableCaption>
            <TableHeader>
              <TableRow v-for="group in table.getHeaderGroups()" :key="group.id">
                <TableHead
                  v-for="header in group.headers"
                  :key="header.id"
                  :class="header.column.id === 'actions' ? 'text-right' : ''"
                  :aria-sort="header.column.getIsSorted() === 'asc' ? 'ascending'
                    : header.column.getIsSorted() === 'desc' ? 'descending' : undefined"
                >
                  <Button
                    v-if="header.column.getCanSort()"
                    variant="ghost" size="sm" class="-ml-2"
                    @click="header.column.toggleSorting()"
                  >
                    <FlexRender :header="header" />
                    <span aria-hidden="true">{{ header.column.getIsSorted() === 'asc' ? '↑' : header.column.getIsSorted() === 'desc' ? '↓' : '↕' }}</span>
                  </Button>
                  <FlexRender v-else :header="header" />
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-if="loading && !items.length">
                <TableCell :colspan="columns.length"><UiLoading /></TableCell>
              </TableRow>
              <TableEmpty v-else-if="loadError" :colspan="columns.length">
                <span role="alert" class="text-destructive">读取失败：{{ loadError }}</span>
              </TableEmpty>
              <template v-else-if="table.getRowModel().rows.length">
                <TableRow v-for="row in table.getRowModel().rows" :key="row.id">
                  <TableCell
                    v-for="cell in row.getAllCells()" :key="cell.id"
                    :class="cell.column.id === 'actions' ? 'text-right' : cell.column.id === 'size' ? 'font-mono tabular-nums' : ''"
                  >
                    <FlexRender :cell="cell" />
                  </TableCell>
                </TableRow>
              </template>
              <TableEmpty v-else :colspan="columns.length">
                {{ items.length ? '没有匹配的存档' : '磁盘上还没有任何存档' }}
              </TableEmpty>
            </TableBody>
          </Table>
        </div>

        <nav v-if="!loadError && items.length" aria-label="存档分页" class="flex flex-wrap items-center justify-end gap-2">
          <span class="mr-2 text-sm text-muted-foreground" aria-live="polite">第 {{ table.atoms.pagination.get().pageIndex + 1 }} / {{ Math.max(1, table.getPageCount()) }} 页</span>
          <Button variant="outline" size="sm" :disabled="!table.getCanPreviousPage()" @click="table.previousPage()">
            <Icon name="chevron" class="rotate-180" :size="14" />上一页
          </Button>
          <Button variant="outline" size="sm" :disabled="!table.getCanNextPage()" @click="table.nextPage()">
            下一页<Icon name="chevron" :size="14" />
          </Button>
        </nav>
      </div>
    </div>
  </UiCard>
</template>
