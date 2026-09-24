<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput } from 'naive-ui'
import { ApiError, api } from '../api'
import { errorKey } from '../error-codes'
import { useSessionStore } from '../store'

const session = useSessionStore()
const { t } = useI18n()
const currentPassword = ref('')
const newPassword = ref('')
const error = ref('')
const notice = ref('')
const busy = ref(false)

async function submit() {
  error.value = ''
  notice.value = ''
  if (newPassword.value.length < 8) {
    error.value = t('passwordTooShort')
    return
  }
  busy.value = true
  try {
    await api.changePassword(currentPassword.value, newPassword.value)
    currentPassword.value = ''
    newPassword.value = ''
    notice.value = t('saved')
  } catch (problem) {
    error.value = problem instanceof ApiError ? t(errorKey(problem.code)) : String(problem)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section>
    <n-alert v-if="error" type="error" closable class="alert" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="notice" type="success" closable class="alert" @close="notice = ''">{{ notice }}</n-alert>
    <header class="heading">
      <div>
        <span class="eyebrow">{{ t('account') }}</span>
        <h1>{{ session.user?.username }}</h1>
        <span class="hint">{{ session.user?.root_dir }}</span>
      </div>
    </header>
    <n-card class="form-card" size="small">
      <n-form @submit.prevent="submit">
        <n-form-item :label="t('currentPassword')">
          <n-input v-model:value="currentPassword" type="password" autocomplete="current-password" />
        </n-form-item>
        <n-form-item :label="t('newPassword')">
          <n-input v-model:value="newPassword" type="password" autocomplete="new-password" />
        </n-form-item>
        <n-button type="primary" attr-type="submit" :loading="busy">{{ t('changePassword') }}</n-button>
      </n-form>
    </n-card>
  </section>
</template>
