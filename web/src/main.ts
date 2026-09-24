import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { i18n } from './i18n'
import { router } from './router'
import './styles.css'

const app = createApp(App)
app.use(createPinia()).use(router).use(i18n)
router.isReady().then(() => app.mount('#app'))
