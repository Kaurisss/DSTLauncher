<script setup>
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import { confirmState, handleCancel, handleConfirm } from '@/lib/confirm.js';
</script>

<template>
  <AlertDialog :open="confirmState.open" @update:open="(val) => { if (!val) handleCancel(); }">
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{{ confirmState.title }}</AlertDialogTitle>
        <AlertDialogDescription v-if="confirmState.description" class="whitespace-pre-line text-sm text-muted-foreground">
          {{ confirmState.description }}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel @click="handleCancel">
          {{ confirmState.cancelText }}
        </AlertDialogCancel>
        <!-- 先解析确认结果，再由组件内部关闭事件处理弹窗，避免被误判为取消。 -->
        <AlertDialogAction
          :class="confirmState.destructive ? 'bg-destructive text-destructive-foreground hover:bg-destructive/90' : ''"
          @click.capture="handleConfirm"
        >
          {{ confirmState.confirmText }}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  </AlertDialog>
</template>
