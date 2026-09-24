<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NConfigProvider, NSpin, darkTheme, dateEnUS, dateZhCN, enUS, lightTheme, zhCN } from 'naive-ui'
import { useSessionStore } from './store'

const session = useSessionStore()
const { locale, t } = useI18n()
const route = useRoute()
const router = useRouter()
const stored = localStorage.getItem('ew-theme')
const systemDark = typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches
const dark = ref(stored ? stored === 'dark' : systemDark)

watch(dark, (value) => {
  localStorage.setItem('ew-theme', value ? 'dark' : 'light')
  document.documentElement.dataset.theme = value ? 'dark' : 'light'
}, { immediate: true })

const theme = computed(() => (dark.value ? darkTheme : lightTheme))
const naiveLocale = computed(() => (locale.value === 'zh-CN' ? zhCN : enUS))
const naiveDateLocale = computed(() => (locale.value === 'zh-CN' ? dateZhCN : dateEnUS))
const active = computed(() => String(route.path).split('/')[1] || 'files')
const waiting = computed(() => !session.loaded)

function go(path: string) {
  router.push(path)
}
function toggleLocale() {
  locale.value = locale.value === 'en' ? 'zh-CN' : 'en'
  localStorage.setItem('ew-locale', locale.value)
}
async function signOut() {
  await session.signOut()
  router.push({ name: 'login' })
}

onMounted(() => {
  if (!session.loaded) session.bootstrap()
})
</script>

<template>
  <n-config-provider :theme="theme" :locale="naiveLocale" :date-locale="naiveDateLocale">
    <div class="shell">
      <header class="topbar">
        <button class="brand" @click="go('/files')"><b>EW</b> {{ session.siteName }}</button>
        <nav v-if="session.user">
          <button :class="{ active: active === 'files' }" @click="go('/files')">{{ t('files') }}</button>
          <template v-if="session.isAdmin">
            <button :class="{ active: active === 'overview' }" @click="go('/overview')">{{ t('overview') }}</button>
            <button :class="{ active: active === 'users' }" @click="go('/users')">{{ t('users') }}</button>
            <button :class="{ active: active === 'settings' }" @click="go('/settings')">{{ t('settings') }}</button>
          </template>
          <button :class="{ active: active === 'account' }" @click="go('/account')">{{ t('account') }}</button>
        </nav>
        <div class="top-actions">
          <n-button quaternary size="small" :title="locale === 'en' ? '中文' : 'English'" @click="toggleLocale">{{ locale === 'en' ? '中文' : 'EN' }}</n-button>
          <n-button
            quaternary
            size="small"
            :title="dark ? t('light') : t('dark')"
            @click="dark = !dark"
          >
            {{ dark ? '☀' : '☾' }}
          </n-button>
          <n-button v-if="session.user" quaternary size="small" @click="signOut">{{ t('signOut') }}</n-button>
        </div>
      </header>
      <main class="content">
        <n-alert v-if="session.insecure" type="warning" class="alert">{{ t('insecure') }}</n-alert>
        <n-spin :show="waiting">
          <router-view v-if="!waiting" />
          <div v-else style="min-height: 50vh" />
        </n-spin>
      </main>
    </div>
  </n-config-provider>
</template>

<style>
.alert {
  margin-bottom: 16px;
}
</style>
