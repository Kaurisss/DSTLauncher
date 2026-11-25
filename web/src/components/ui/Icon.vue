<script setup>
import { computed } from 'vue';
import {
  PhArchive, PhArrowClockwise, PhArrowCounterClockwise, PhArrowSquareOut, PhArrowsClockwise,
  PhArrowsLeftRight, PhBroom, PhCaretRight, PhCheck, PhClipboard, PhClock, PhClockCounterClockwise,
  PhCloudCheck, PhCpu, PhDownloadSimple, PhFastForward, PhFileCode, PhFloppyDisk, PhFolder,
  PhGlobe, PhHardDrives, PhHeart, PhInfo, PhKey, PhMagnifyingGlass, PhMapPin, PhMapTrifold,
  PhMegaphone, PhMemory, PhPackage, PhPaperPlaneTilt, PhPencilSimple, PhPlay, PhPlug, PhPlus,
  PhProhibit, PhPulse, PhSignOut, PhSliders, PhStack, PhStethoscope, PhStop, PhTerminalWindow,
  PhTrash, PhUploadSimple, PhUsers, PhWarning, PhX,
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
  diagnostics: PhStethoscope,
  cpu: PhCpu,
  key: PhKey,
  globe: PhGlobe,
  users: PhUsers,
  clock: PhClock,
  rollback: PhClockCounterClockwise,
  clipboard: PhClipboard,
  search: PhMagnifyingGlass,
  undo: PhArrowCounterClockwise,
  ban: PhProhibit,
  heart: PhHeart,
  send: PhPaperPlaneTilt,
  megaphone: PhMegaphone,
  sliders: PhSliders,
  external: PhArrowSquareOut,
  skip: PhFastForward,
  logout: PhSignOut,
  pin: PhMapPin,
  map: PhMapTrifold,
  migrate: PhArrowsLeftRight,
  templates: PhStack,
  cloudCheck: PhCloudCheck,
  plug: PhPlug,
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
