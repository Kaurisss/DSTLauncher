// 界面外壳状态：当前标签页与导航分组。
//
// 侧边栏按作用域分为「当前房间」与「全局管理」两组：
// - room 作用域页面随顶栏房间切换器切换上下文；
// - global 作用域页面管理全站资源，顶栏不展示房间切换器以避免作用域误导。

import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';

/** activeTab 的本地记忆 key，与房间选择的记忆方式保持一致。 */
const TAB_KEY = 'dst-tab';

function readStoredTab(validKeys) {
  try {
    const v = localStorage.getItem(TAB_KEY);
    return validKeys.includes(v) ? v : '';
  } catch (_) {
    return '';
  }
}

function storeTab(key) {
  try {
    if (key) localStorage.setItem(TAB_KEY, key);
    else localStorage.removeItem(TAB_KEY);
  } catch (_) {
    // 写不了只是刷新后记不住选择，不影响功能
  }
}

export const useUiStore = defineStore('ui', () => {
  /** 导航分组：组序与项序即侧边栏展示顺序，key 决定视图映射（见 App.vue）。 */
  const navGroups = [
    {
      key: 'room',
      label: '当前房间',
      items: [
        { key: 'overview', label: '概览', icon: 'dashboard', scope: 'room' },
        { key: 'console', label: '控制台', icon: 'terminal', scope: 'room' },
        { key: 'world', label: '世界配置', icon: 'globe', scope: 'room' },
        { key: 'mods', label: '模组', icon: 'package', scope: 'room' },
        { key: 'logs', label: '玩家管理', icon: 'users', scope: 'room' },
        { key: 'backups', label: '备份与回档', icon: 'archive', scope: 'room' },
      ],
    },
    {
      key: 'global',
      label: '全局管理',
      items: [
        { key: 'rooms', label: '房间管理', icon: 'server', scope: 'global' },
        { key: 'saves', label: '磁盘存档', icon: 'folder', scope: 'global' },
        { key: 'settings', label: '系统设置', icon: 'settings', scope: 'global' },
      ],
    },
  ];

  /** 扁平化的全部导航项，供标题解析、路由校验与测试遍历。 */
  const tabs = navGroups.flatMap(group => group.items);

  // 初值取上次的选择；存了不认识的旧 key（如已下沉的 config）时回落到概览
  const activeTab = ref(readStoredTab(tabs.map(t => t.key)) || 'overview');
  const activeTabMeta = computed(() => tabs.find(t => t.key === activeTab.value) || tabs[0]);
  const isGlobalTab = computed(() => activeTabMeta.value?.scope === 'global');
  const settingsSection = ref('maintenance');
  const pendingEditRoom = ref(null);

  // 与 rooms.js 的房间记忆同理：切换就落盘，刷新后停在原页面
  watch(activeTab, storeTab);

  function select(key) {
    if (tabs.some(t => t.key === key)) activeTab.value = key;
  }

  function openSettings(section = 'maintenance') {
    settingsSection.value = section;
    select('settings');
  }

  function openRoomEditor(roomId = null, section = 'general') {
    pendingEditRoom.value = { roomId: roomId ? Number(roomId) : 0, section };
    select('rooms');
  }

  function consumePendingEditRoom() {
    const req = pendingEditRoom.value;
    pendingEditRoom.value = null;
    return req;
  }

  return {
    navGroups, tabs, activeTab, activeTabMeta, isGlobalTab,
    settingsSection, select, openSettings, openRoomEditor, consumePendingEditRoom,
  };
});
