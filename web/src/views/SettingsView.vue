<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput, NSelect } from 'naive-ui'
import { ApiError, api } from '../api'
import { errorKey } from '../error-codes'
import { QUOTA_UNITS, formatSize, quotaToBytes, splitQuota } from '../format'
import { useSessionStore } from '../store'

const session = useSessionStore()
const { t } = useI18n()
const siteName = ref('')
const quotaAmount = ref(0)
const quotaUnit = ref(QUOTA_UNITS[2].bytes)
const permission = ref('readwrite')
const error = ref('')
const notice = ref('')
const busy = ref(false)

const quotaUnits = QUOTA_UNITS.map((unit) => ({ label: unit.label, value: unit.bytes }))
const permissionOptions = computed(() => [
  { label: t('readOnly'), value: 'read' },
  { label: t('readWrite'), value: 'readwrite' }
])

function fail(problem: unknown) {
  notice.value = ''
  error.value = problem instanceof ApiError ? t(errorKey(problem.code)) : String(problem)
}
async function load() {
  try {
    const values = await api.settings()
    siteName.value = values.site_name || session.siteName
    const quota = splitQuota(Number(values.default_quota || 0))
    quotaAmount.value = quota.amount
    quotaUnit.value = quota.unit
    permission.value = values.default_permission || 'readwrite'
  } catch (problem) {
    fail(problem)
  }
}
async function save() {
  busy.value = true
  try {
    const values = {
      site_name: siteName.value,
      default_quota: String(quotaToBytes(quotaAmount.value, quotaUnit.value)),
      default_permission: permission.value
    }
    await api.saveSettings(values)
    session.applySettings(values)
    error.value = ''
    notice.value = t('saved')
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
        <span class="eyebrow">{{ t('settings') }}</span>
        <h1>{{ t('settings') }}</h1>
      </div>
    </header>
    <n-card class="form-card" size="small">
      <n-form @submit.prevent="save">
        <n-form-item :label="t('siteName')">
          <n-input v-model:value="siteName" />
        </n-form-item>
        <n-form-item :label="t('defaultQuota')">
          <div class="actions" style="width: 100%">
            <n-input v-model:value="quotaAmount" type="number" min="0" style="max-width: 160px" />
            <n-select v-model:value="quotaUnit" :options="quotaUnits" style="max-width: 120px" />
            <span class="hint">{{ quotaAmount ? formatSize(quotaToBytes(quotaAmount, quotaUnit)) : t('unlimited') }}</span>
          </div>
        </n-form-item>
        <n-form-item :label="t('defaultPermission')">
          <n-select v-model:value="permission" :options="permissionOptions" style="max-width: 220px" />
        </n-form-item>
        <n-button type="primary" attr-type="submit" :loading="busy">{{ t('saveSettings') }}</n-button>
      </n-form>
    </n-card>
  </section>
</template>
