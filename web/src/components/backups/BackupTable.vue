<script setup>
import { h } from 'vue';
import {
  createPaginatedRowModel, createSortedRowModel, FlexRender,
  rowPaginationFeature, rowSortingFeature, sortFn_basic,
  tableFeatures, useTable,
} from '@tanstack/vue-table';

import Icon from '@/components/ui/Icon.vue';
import { Button } from '@/components/ui/button';
import {
  Table, TableBody, TableCaption, TableCell,
  TableHead, TableHeader, TableRow,
} from '@/components/ui/table';
import { fmtSize, fmtTime } from '@/lib/format.js';

const props = defineProps({
  items: { type: Array, required: true },
});
const emit = defineEmits(['restore', 'remove', 'export']);

const columns = [
  {
    id: 'createdAt', header: '时间',
    accessorFn: item => new Date(item.createdAt).getTime(),
    cell: ({ row }) => fmtTime(row.original.createdAt),
  },
  { accessorKey: 'size', header: '大小', cell: ({ getValue }) => fmtSize(getValue()) },
  {
    id: 'actions', header: '操作', enableSorting: false,
    cell: ({ row }) => h('div', { class: 'flex justify-end gap-2' }, [
      h(Button, { variant: 'outline', size: 'sm', onClick: () => emit('restore', row.original.name) }, () => '回滚'),
      h(Button, { variant: 'outline', size: 'sm', onClick: () => emit('export', row.original.name) }, () => '导出'),
      h(Button, { variant: 'destructive', size: 'sm', onClick: () => emit('remove', row.original.name) }, () => '删除'),
    ]),
  },
];

const table = useTable({
  features: tableFeatures({
    rowSortingFeature, rowPaginationFeature,
    sortedRowModel: createSortedRowModel(),
    paginatedRowModel: createPaginatedRowModel(),
    sortFns: { basic: sortFn_basic },
  }),
  get data() { return props.items; },
  columns,
  getRowId: item => item.name,
  defaultColumn: { sortFn: 'basic' },
  initialState: {
    sorting: [{ id: 'createdAt', desc: true }],
    pagination: { pageIndex: 0, pageSize: 10 },
  },
});
</script>

<template>
  <div class="grid min-w-0 gap-4">
    <div class="min-w-0 overflow-hidden rounded-md border">
      <Table class="min-w-[560px]">
        <TableCaption class="sr-only">备份列表</TableCaption>
        <TableHeader>
          <TableRow v-for="group in table.getHeaderGroups()" :key="group.id">
            <TableHead
              v-for="header in group.headers" :key="header.id"
              :class="header.column.id === 'actions' ? 'text-right' : ''"
              :aria-sort="header.column.getIsSorted() === 'asc' ? 'ascending'
                : header.column.getIsSorted() === 'desc' ? 'descending' : undefined"
            >
              <Button
                v-if="header.column.getCanSort()"
                variant="ghost" size="sm" class="-ml-2"
                @click="header.column.toggleSorting(header.column.getIsSorted() === 'asc')"
              >
                <FlexRender :header="header" />
                <span aria-hidden="true">{{ header.column.getIsSorted() === 'asc' ? '↑' : header.column.getIsSorted() === 'desc' ? '↓' : '↕' }}</span>
              </Button>
              <FlexRender v-else :header="header" />
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in table.getRowModel().rows" :key="row.id">
            <TableCell
              v-for="cell in row.getAllCells()" :key="cell.id"
              :class="cell.column.id === 'size' ? 'font-mono tabular-nums' : ''"
            >
              <FlexRender :cell="cell" />
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>

    <nav aria-label="备份分页" class="flex flex-wrap items-center justify-end gap-2">
      <span class="mr-2 text-sm text-muted-foreground" aria-live="polite">第 {{ table.atoms.pagination.get().pageIndex + 1 }} / {{ Math.max(1, table.getPageCount()) }} 页</span>
      <Button variant="outline" size="sm" :disabled="!table.getCanPreviousPage()" @click="table.previousPage()">
        <Icon name="chevron" class="rotate-180" :size="14" />上一页
      </Button>
      <Button variant="outline" size="sm" :disabled="!table.getCanNextPage()" @click="table.nextPage()">
        下一页<Icon name="chevron" :size="14" />
      </Button>
    </nav>
  </div>
</template>
