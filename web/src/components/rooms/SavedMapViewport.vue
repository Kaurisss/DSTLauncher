<script setup>
import { computed, ref, useTemplateRef, watch } from 'vue';
import { useElementSize } from '@vueuse/core';
import { mapApi } from '@/lib/api.js';
import { landmarkCoordinates, landmarkStyle } from '@/lib/mapLandmarks.js';

const props = defineProps({
  map: { type: Object, required: true },
  zoom: { type: Number, required: true },
  selectedId: { type: String, default: '' },
  showLandmarks: { type: Boolean, default: true },
});
const emit = defineEmits(['update:zoom', 'select']);
const svg = useTemplateRef('svg');
const viewport = useTemplateRef('viewport');
const { width: viewportWidth, height: viewportHeight } = useElementSize(viewport, { width: 600, height: 450 });
const viewportSize = computed(() => ({
  width: viewportWidth.value || 600,
  height: viewportHeight.value || 450,
}));
const center = ref({ x: props.map.width / 2, y: props.map.height / 2 });
const dragging = ref(false);
let drag;
const scale = computed(() => Math.min(viewportSize.value.width / (props.map.width / props.zoom), viewportSize.value.height / (props.map.height / props.zoom)));
const view = computed(() => ({ x: center.value.x - props.map.width / props.zoom / 2, y: center.value.y - props.map.height / props.zoom / 2, width: props.map.width / props.zoom, height: props.map.height / props.zoom }));
const selected = computed(() => props.map.landmarks.find(point => point.id === props.selectedId));
const paired = computed(() => selected.value?.target ? props.map.landmarks.find(point => point.id === selected.value.target) : null);
const markers = computed(() => props.map.landmarks.filter(point => point.x >= view.value.x && point.x <= view.value.x + view.value.width && point.y >= view.value.y && point.y <= view.value.y + view.value.height));
const iconKinds = computed(() => new Set(props.map.icons || []));
const hasGameIcon = kind => iconKinds.value.has(kind);

// As in DMP's map renderer, choose labels in screen space and keep them apart.
// Important landmarks remain named at fit zoom; selection always takes priority.
const labels = computed(() => {
  const boxes = [];
  const visible = new Map();
  const sorted = [...markers.value].sort((a, b) => (a.id === props.selectedId ? -1 : b.id === props.selectedId ? 1 : landmarkStyle(a.kind).priority - landmarkStyle(b.kind).priority));
  for (const point of sorted) {
    if (point.id !== props.selectedId && landmarkStyle(point.kind).priority > 1 && props.zoom < 2) continue;
    const x = (point.x - view.value.x) * scale.value + (viewportSize.value.width - view.value.width * scale.value) / 2;
    const y = point.y * scale.value;
    const width = point.name.length * 12;
    const offset = hasGameIcon(point.kind) ? 18 : 16;
    const left = x + offset + 8 + width > viewportSize.value.width;
    const box = { x: left ? x - offset - 8 - width : x + offset, y: y - 10, right: left ? x - offset : x + offset + 8 + width, bottom: y + 10 };
    if (point.id !== props.selectedId && boxes.some(other => box.x < other.right && box.right > other.x && box.y < other.bottom && box.bottom > other.y)) continue;
    boxes.push(box);
    visible.set(point.id, { x: left ? -offset : offset, anchor: left ? 'end' : 'start' });
  }
  return visible;
});

function clampCenter(zoom = props.zoom) {
  const halfX = props.map.width / zoom / 2, halfY = props.map.height / zoom / 2;
  center.value = { x: Math.max(halfX, Math.min(props.map.width - halfX, center.value.x)), y: Math.max(halfY, Math.min(props.map.height - halfY, center.value.y)) };
}
watch(() => props.zoom, value => {
  if (value === 1) center.value = { x: props.map.width / 2, y: props.map.height / 2 };
  clampCenter(value);
});
watch(() => props.selectedId, () => {
  if (!selected.value) return;
  const zoom = Math.max(props.zoom, 2);
  center.value = { x: selected.value.x, y: selected.value.y };
  clampCenter(zoom);
  emit('update:zoom', zoom);
});

function down(event) {
  if (event.button !== 0 || event.target.closest('[data-landmark]')) return;
  drag = { pointer: event.pointerId, x: event.clientX, y: event.clientY, center: { ...center.value } };
  dragging.value = true;
  svg.value.setPointerCapture(event.pointerId);
}
function move(event) {
  if (!drag || event.pointerId !== drag.pointer) return;
  center.value = { x: drag.center.x - (event.clientX - drag.x) / scale.value, y: drag.center.y - (event.clientY - drag.y) / scale.value };
  clampCenter();
}
function up() { drag = null; dragging.value = false; }
function wheel(event) {
  const next = Math.max(1, Math.min(8, props.zoom * (event.deltaY < 0 ? 1.2 : 1 / 1.2)));
  const point = svg.value.createSVGPoint();
  point.x = event.clientX; point.y = event.clientY;
  const anchor = point.matrixTransform(svg.value.getScreenCTM().inverse());
  center.value = { x: anchor.x - (anchor.x - center.value.x) * props.zoom / next, y: anchor.y - (anchor.y - center.value.y) * props.zoom / next };
  clampCenter(next);
  emit('update:zoom', next);
}
function keydown(event) {
  const offsets = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
  if (offsets[event.key]) {
    event.preventDefault();
    center.value = { x: center.value.x + offsets[event.key][0] * 40 / scale.value, y: center.value.y + offsets[event.key][1] * 40 / scale.value };
    clampCenter();
  } else if (event.key === 'Home') { event.preventDefault(); emit('update:zoom', 1); }
}
</script>

<template>
  <div ref="viewport" class="map-viewport min-h-0 min-w-0 overflow-hidden" :data-textured="map.textured ? 'true' : 'false'" tabindex="0" role="region" aria-label="存档地形地图" @keydown="keydown">
    <svg ref="svg" class="block size-full cursor-grab touch-none select-none data-[dragging=true]:cursor-grabbing" :data-dragging="dragging" :viewBox="`${view.x} ${view.y} ${view.width} ${view.height}`" role="group" aria-label="地图地形与地标" @pointerdown="down" @pointermove="move" @pointerup="up" @pointercancel="up" @lostpointercapture="up" @wheel.prevent="wheel">
      <image :href="map.terrainImage" :width="map.width" :height="map.height" :style="map.textured ? undefined : { imageRendering: 'pixelated' }" role="img" aria-label="地形底图" />
      <line v-if="showLandmarks && selected && paired" :x1="selected.x" :y1="selected.y" :x2="paired.x" :y2="paired.y" stroke="#fff4cd" :stroke-width="2 / scale" :stroke-dasharray="`${5 / scale} ${4 / scale}`" aria-label="虫洞连接" />
      <g v-if="showLandmarks">
        <g v-for="point in markers" :key="point.id" :transform="`translate(${point.x} ${point.y}) scale(${1 / scale})`" class="map-marker cursor-pointer" data-landmark tabindex="0" role="button" :aria-label="`${point.name}（${landmarkCoordinates(point, map)}）`" :aria-pressed="point.id === selectedId" @click.stop="emit('select', point.id)" @keydown.enter.stop.prevent="emit('select', point.id)" @keydown.space.stop.prevent="emit('select', point.id)">
          <title>{{ point.name }}（{{ landmarkCoordinates(point, map) }}）</title>
          <template v-if="hasGameIcon(point.kind)">
            <circle v-if="point.id === selectedId || point.id === paired?.id" r="18" fill="rgba(255, 244, 205, 0.16)" stroke="#fff4cd" stroke-width="2" />
            <circle class="marker-frame" r="16" fill="transparent" stroke="transparent" stroke-width="2" />
            <image :href="mapApi.iconUrl(point.kind)" x="-15" y="-15" width="30" height="30" preserveAspectRatio="xMidYMid meet" class="map-game-icon" aria-hidden="true" />
          </template>
          <template v-else>
            <circle v-if="point.id === selectedId || point.id === paired?.id" r="15" fill="none" stroke="#fff4cd" stroke-width="2" />
            <circle class="marker-frame" r="11" :fill="landmarkStyle(point.kind).color" stroke="#f5f1e8" stroke-width="1.5" />
            <component :is="landmarkStyle(point.kind).icon" x="-8" y="-8" :width="16" :height="16" weight="bold" color="#ffffff" aria-hidden="true" />
          </template>
          <text v-if="labels.has(point.id)" :x="labels.get(point.id).x" :text-anchor="labels.get(point.id).anchor" y="4" font-size="12" font-weight="600" fill="#ffffff" stroke="#18232c" stroke-width="3" paint-order="stroke" stroke-linejoin="round">{{ point.name }}</text>
        </g>
      </g>
    </svg>
  </div>
</template>

<style scoped>
.map-viewport { background: #152632; }
.map-viewport[data-textured="true"] { background: #16181d; }
.map-marker:focus-visible { outline: none; }
.map-marker:focus-visible .marker-frame { stroke: #fff4cd; stroke-width: 4; }
.map-game-icon { filter: drop-shadow(0 1px 2px rgba(0, 0, 0, 0.72)); }
</style>
