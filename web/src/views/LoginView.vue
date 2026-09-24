<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput } from 'naive-ui'
import { ApiError } from '../api'
import { errorKey } from '../error-codes'
import { useSessionStore } from '../store'

const session = useSessionStore()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const error = ref('')
const busy = ref(false)
const setup = computed(() => session.setupRequired)

async function submit() {
  error.value = ''
  busy.value = true
  try {
    if (setup.value) {
      await session.signUp(username.value, password.value)
    } else {
      await session.signIn(username.value, password.value)
    }
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/files'
    await router.push(redirect)
  } catch (problem) {
    error.value = problem instanceof ApiError ? t(errorKey(problem.code)) : String(problem)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="auth">
    <n-card class="auth-card">
      <span class="eyebrow">{{ session.siteName }}</span>
      <h1>{{ setup ? t('setup') : t('login') }}</h1>
      <p class="hint">{{ t('authDescription') }}</p>
      <n-alert v-if="error" type="error" class="alert">{{ error }}</n-alert>
      <n-form @submit.prevent="submit">
        <n-form-item :label="t('username')">
          <n-input v-model:value="username" autocomplete="username" />
        </n-form-item>
        <n-form-item :label="t('password')">
          <n-input v-model:value="password" type="password" show-password-on="click" autocomplete="current-password" />
        </n-form-item>
        <n-button type="primary" attr-type="submit" block :loading="busy">{{ setup ? t('createAdmin') : t('login') }}</n-button>
      </n-form>
    </n-card>
  </section>
</template>
