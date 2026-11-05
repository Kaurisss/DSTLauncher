<script setup>
import { computed } from 'vue';
import {
  PhArchive, PhArrowClockwise, PhArrowCounterClockwise, PhArrowSquareOut, PhArrowsClockwise,
  PhBroom, PhCaretRight, PhCheck, PhClipboard, PhClock, PhCpu, PhDownloadSimple, PhFastForward,
  PhFileCode, PhFloppyDisk, PhFolder, PhGlobe, PhHardDrives, PhHeart, PhInfo, PhKey,
  PhMagnifyingGlass, PhMapPin, PhMemory, PhPackage, PhPaperPlaneTilt, PhPencilSimple, PhPlay, PhPlus,
  PhProhibit, PhPulse, PhSignOut, PhSliders, PhStop, PhTerminalWindow, PhTrash, PhUploadSimple,
  PhUsers, PhWarning, PhX,
} from '@phosphor-icons/vue';

const ICONS = {
  server: PhHardDrives,
  package: PhPackage,
  archive: PhArchive,
  folder: PhFolder,
  fileCode: PhFileCode,
  play: PhPlay,
  stop: PhStop,
  restart: PhArrowsClockwise,
  refresh: PhArrowClockwise,
  plus: PhPlus,
  pencil: PhPencilSimple,
  trash: PhTrash,
  check: PhCheck,
  download: PhDownloadSimple,
  upload: PhUploadSimple,
  chevron: PhCaretRight,
  x: PhX,
  alert: PhWarning,
  info: PhInfo,
  terminal: PhTerminalWindow,
  activity: PhPulse,
  cpu: PhCpu,
  key: PhKey,
  globe: PhGlobe,
  users: PhUsers,
  clock: PhClock,
  clipboard: PhClipboard,
  search: PhMagnifyingGlass,
  undo: PhArrowCounterClockwise,
  ban: PhProhibit,
  heart: PhHeart,
  send: PhPaperPlaneTilt,
  sliders: PhSliders,
  external: PhArrowSquareOut,
  skip: PhFastForward,
  logout: PhSignOut,
  pin: PhMapPin,
  memory: PhMemory,
  save: PhFloppyDisk,
  clean: PhBroom,
};

const props = defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 18 },
});

const icon = computed(() => ICONS[props.name]);
</script>

<template>
  <component :is="icon" v-if="icon" :size="size" aria-hidden="true" />
</template>
