<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NProgress } from 'naive-ui'
import { ApiError, api, type UsageOverview } from '../api'
import { errorKey } from '../error-codes'
import { formatSize, usagePercent } from '../format'

const { t } = useI18n()
const overview = ref<UsageOverview | null>(null)
const error = ref('')
const notice = ref('')
const busy = ref(false)
const loading = ref(false)

const totalUsed = computed(() => (overview.value?.users || []).reduce((sum, item) => sum + item.used, 0))
const activeUsers = computed(() => (overview.value?.users || []).filter((item) => !item.user.disabled).length)
const largest = computed(() => [...(overview.value?.users || [])].sort((left, right) => right.used - left.used).slice(0, 10))

function fail(problem: unknown) {
  notice.value = ''
  error.value = problem instanceof ApiError ? t(errorKey(problem.code)) : String(problem)
}
async function load() {
  loading.value = true
  try {
    overview.value = await api.usageOverview()
    error.value = ''
  } catch (problem) {
    fail(problem)
  } finally {
    loading.value = false
  }
}
async function recalculate() {
  busy.value = true
  try {
    await api.recalculate()
    await load()
    error.value = ''
    notice.value = t('recalculated')
  } catch (problem) {
    fail(problem)
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section>
    <n-alert v-if="error" type="error" closable class="alert" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="notice" type="success" closable class="alert" @close="notice = ''">{{ notice }}</n-alert>
    <header class="heading">
      <div>
        <span class="eyebrow">{{ t('overview') }}</span>
        <h1>{{ t('overview') }}</h1>
      </div>
      <n-button :loading="busy" @click="recalculate">{{ t('recalculate') }}</n-button>
    </header>
    <div class="metrics">
      <n-card size="small"><span class="cell">{{ t('totalUsers') }}</span><strong>{{ overview?.users.length || 0 }}</strong></n-card>
      <n-card size="small"><span class="cell">{{ t('activeUsers') }}</span><strong>{{ activeUsers }}</strong></n-card>
      <n-card size="small"><span class="cell">{{ t('totalUsed') }}</span><strong>{{ formatSize(totalUsed) }}</strong></n-card>
      <n-card size="small">
        <span class="cell">{{ t('diskFree') }}</span>
        <strong>{{ overview?.disk.free ? formatSize(overview.disk.free) : t('unavailable') }}</strong>
      </n-card>
    </div>
    <n-card size="small">
      <h2>{{ t('largestUsers') }}</h2>
      <div v-for="item in largest" :key="item.user.id" class="usage-line">
        <span>{{ item.user.username }}</span>
        <span>{{ formatSize(item.used) }}<template v-if="item.user.quota"> / {{ formatSize(item.user.quota) }}</template></span>
        <n-progress type="line" :percentage="usagePercent(item.used, item.user.quota)" :show-indicator="false" />
      </div>
      <p v-if="!largest.length && !loading" class="hint">{{ t('empty') }}</p>
    </n-card>
  </section>
</template>
