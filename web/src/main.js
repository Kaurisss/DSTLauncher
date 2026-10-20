import { createApp } from 'vue';
import { createPinia } from 'pinia';

import App from '@/App.vue';
import { initTheme } from '@/composables/useTheme.js';

import 'vue-sonner/style.css';
import '@/styles/tokens.css';

// 主题要在挂载前落到 <html> 上，与 index.html 里的防闪屏脚本配合
initTheme();

createApp(App).use(createPinia()).mount('#app');
