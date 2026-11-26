// 导航与分段控件的滑动高亮指示器。
// 首次测量直接落位（跳过首屏入场飞行），后续切换用 CSS transition 平滑滑动并支持中途打断。

import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { unrefElement } from '@vueuse/core';

/**
 * 追踪容器内当前激活项的位置与尺寸，驱动绝对定位的高亮胶囊。
 *
 * @param {import('vue').Ref<HTMLElement | import('vue').ComponentPublicInstance | null>} containerRef
 * @param {string} activeSelector - 匹配当前激活按钮的 CSS 选择器
 * @param {() => unknown} watchSource - 触发重新测量的响应式依赖
 */
export function useSlidingIndicator(containerRef, activeSelector, watchSource) {
  const ready = ref(false);
  const visible = ref(false);
  const animated = ref(false);
  const rect = ref({ left: 0, top: 0, width: 0, height: 0 });

  let containerObserver = null;
  let activeObserver = null;
  let observedContainer = null;
  let observedActive = null;
  let enableAnimFrame = 0;

  function cancelAnimFrame() {
    if (enableAnimFrame && typeof window !== 'undefined' && typeof window.cancelAnimationFrame === 'function') {
      window.cancelAnimationFrame(enableAnimFrame);
    }
    enableAnimFrame = 0;
  }

  function disconnectActiveObserver() {
    activeObserver?.disconnect();
    observedActive = null;
  }

  function disconnectAll() {
    cancelAnimFrame();
    containerObserver?.disconnect();
    containerObserver = null;
    observedContainer = null;
    disconnectActiveObserver();
  }

  function observeActive(el) {
    if (observedActive === el || typeof window === 'undefined' || typeof window.ResizeObserver === 'undefined') return;
    disconnectActiveObserver();
    if (!el) return;
    activeObserver = new window.ResizeObserver(() => measure());
    activeObserver.observe(el);
    observedActive = el;
  }

  function ensureContainerObserver(container) {
    if (observedContainer === container || typeof window === 'undefined' || typeof window.ResizeObserver === 'undefined') return;
    containerObserver?.disconnect();
    containerObserver = null;
    observedContainer = null;
    if (!container) return;
    containerObserver = new window.ResizeObserver(() => measure());
    containerObserver.observe(container);
    observedContainer = container;
  }

  function scheduleEnableAnimation() {
    if (animated.value || enableAnimFrame) return;
    if (typeof window === 'undefined' || typeof window.requestAnimationFrame !== 'function') return;
    enableAnimFrame = window.requestAnimationFrame(() => {
      enableAnimFrame = window.requestAnimationFrame(() => {
        enableAnimFrame = 0;
        if (ready.value) animated.value = true;
      });
    });
  }

  function measure() {
    const container = unrefElement(containerRef);
    if (!container || typeof container.getBoundingClientRect !== 'function') {
      ready.value = false;
      visible.value = false;
      animated.value = false;
      disconnectAll();
      return;
    }

    ensureContainerObserver(container);
    const active = container.querySelector(activeSelector);
    observeActive(active);

    if (!active) {
      visible.value = false;
      return;
    }

    // 优先使用布局坐标（offsetParent === container），完全不受父级入场 scale/translate 变换影响。
    if (active.offsetParent === container) {
      if (active.offsetWidth === 0 || active.offsetHeight === 0) {
        visible.value = false;
        return;
      }
      rect.value = {
        left: active.offsetLeft,
        top: active.offsetTop,
        width: active.offsetWidth,
        height: active.offsetHeight,
      };
    } else {
      const cRect = container.getBoundingClientRect();
      const eRect = active.getBoundingClientRect();
      if (cRect.width === 0 || eRect.width === 0 || eRect.height === 0) {
        visible.value = false;
        return;
      }
      rect.value = {
        left: eRect.left - cRect.left - container.clientLeft + container.scrollLeft,
        top: eRect.top - cRect.top - container.clientTop + container.scrollTop,
        width: eRect.width,
        height: eRect.height,
      };
    }

    ready.value = true;
    visible.value = true;
    scheduleEnableAnimation();
  }

  async function sync() {
    await nextTick();
    measure();
  }

  watch(
    () => unrefElement(containerRef),
    (el, prev) => {
      if (el !== prev) {
        ready.value = false;
        visible.value = false;
        animated.value = false;
        cancelAnimFrame();
      }
      sync();
    },
    { flush: 'post' },
  );

  if (watchSource) {
    watch(watchSource, () => { sync(); }, { flush: 'post' });
  }

  onMounted(() => {
    sync();
    if (typeof document !== 'undefined' && document.fonts?.ready) {
      document.fonts.ready.then(() => measure()).catch(() => {});
    }
  });

  onUnmounted(disconnectAll);

  const style = computed(() => ({
    transform: `translate3d(${rect.value.left}px, ${rect.value.top}px, 0)`,
    width: `${rect.value.width}px`,
    height: `${rect.value.height}px`,
    opacity: visible.value ? 1 : 0,
  }));

  return {
    ready,
    visible,
    animated,
    style,
    sync,
  };
}
