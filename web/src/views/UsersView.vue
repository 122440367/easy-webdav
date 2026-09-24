<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NModal, NProgress, NSelect } from 'naive-ui'
import { ApiError, api, type UsageOverview, type User } from '../api'
import { errorKey } from '../error-codes'
import { QUOTA_UNITS, formatSize, quotaToBytes, splitQuota, usagePercent } from '../format'
import { useSessionStore } from '../store'

const session = useSessionStore()
const router = useRouter()
const { t } = useI18n()

const users = ref<User[]>([])
const usage = ref<UsageOverview | null>(null)
const loading = ref(false)
const error = ref('')
const notice = ref('')
const dialog = ref(false)
const editing = ref<User | null>(null)
const passwordDialog = ref(false)
const passwordTarget = ref<User | null>(null)
const passwordValue = ref('')
const form = ref({ username: '', password: '', rootDir: '', permission: 'readwrite', quotaAmount: 0, quotaUnit: QUOTA_UNITS[2].bytes, disabled: false })

const quotaUnits = QUOTA_UNITS.map((unit) => ({ label: unit.label, value: unit.bytes }))
const permissionOptions = computed(() => [
  { label: t('readOnly'), value: 'read' },
  { label: t('readWrite'), value: 'readwrite' }
])
const statusOptions = computed(() => [
  { label: t('enabled'), value: 'enabled' },
  { label: t('disabled'), value: 'disabled' }
])
const usedFor = (user: User) => usage.value?.users.find((item) => item.user.id === user.id)?.used || 0
const activeCount = computed(() => users.value.filter((user) => !user.disabled).length)
const activeAdmins = computed(() => users.value.filter((user) => user.role === 'admin' && !user.disabled).length)
function isLastAdmin(user: User) {
  return user.role === 'admin' && !user.disabled && activeAdmins.value <= 1
}

const columns = computed(() => [
  { title: t('username'), key: 'username' },
  { title: t('rootDir'), key: 'root_dir' },
  { title: t('permission'), key: 'permission', render: (user: User) => (user.permission === 'read' ? t('readOnly') : t('readWrite')) },
  { title: t('status'), key: 'disabled', render: (user: User) => (user.disabled ? t('disabled') : t('enabled')) },
  {
    title: t('usage'),
    key: 'usage',
    render: (user: User) =>
      h('div', { class: 'user-usage' }, [
        h('span', `${formatSize(usedFor(user))} / ${user.quota ? formatSize(user.quota) : t('unlimited')} · ${usagePercent(usedFor(user), user.quota)}%`),
        user.quota ? h(NProgress, { type: 'line', percentage: usagePercent(usedFor(user), user.quota), showIndicator: false }) : null
      ])
  },
  {
    title: t('actions'),
    key: 'actions',
    render: (user: User) =>
      h('div', { class: 'actions' }, [
        h(NButton, { size: 'small', onClick: () => router.push(`/files/u/${user.id}`) }, () => t('browseFiles')),
        h(NButton, { size: 'small', onClick: () => openDialog(user) }, () => t('edit')),
        h(NButton, { size: 'small', onClick: () => openPassword(user) }, () => t('resetPassword')),
        h(NButton, { size: 'small', disabled: isLastAdmin(user), onClick: () => toggleUser(user) }, () => (user.disabled ? t('enable') : t('disable'))),
        h(NButton, { size: 'small', type: 'error', disabled: isLastAdmin(user), onClick: () => removeUser(user) }, () => t('delete'))
      ])
  }
])

function fail(problem: unknown) {
  notice.value = ''
  error.value = problem instanceof ApiError ? t(errorKey(problem.code)) : String(problem)
}
const rowKey = (user: User) => user.id
function succeed(message: string) {
  error.value = ''
  notice.value = message
}
async function load() {
  loading.value = true
  try {
    const [list, stats] = await Promise.all([api.users(), api.usageOverview()])
    users.value = list
    usage.value = stats
    error.value = ''
  } catch (problem) {
    fail(problem)
  } finally {
    loading.value = false
  }
}
function openDialog(target?: User) {
  editing.value = target || null
  const quota = splitQuota(target?.quota ?? Number(session.settings.default_quota || 0))
  form.value = {
    username: target?.username || '',
    password: '',
    rootDir: target?.root_dir || '',
    permission: target?.permission || session.settings.default_permission || 'readwrite',
    quotaAmount: quota.amount,
    quotaUnit: quota.unit,
    disabled: target?.disabled || false
  }
  dialog.value = true
}
async function save() {
  const quota = quotaToBytes(form.value.quotaAmount, form.value.quotaUnit)
  if (!form.value.username.trim() || (!editing.value && form.value.password.length < 8) || quota < 0) {
    fail(new Error(t('invalidUserForm')))
    return
  }
  try {
    const body: Record<string, unknown> = {
      username: form.value.username.trim(),
      permission: form.value.permission,
      quota,
      disabled: form.value.disabled
    }
    if (form.value.rootDir.trim()) body.root_dir = form.value.rootDir.trim()
    if (!editing.value) body.password = form.value.password
    if (editing.value) await api.updateUser(editing.value.id, body)
    else await api.createUser(body)
    dialog.value = false
    await load()
    succeed(t('saved'))
  } catch (problem) {
    fail(problem)
  }
}
function openPassword(user: User) {
  passwordTarget.value = user
  passwordValue.value = ''
  passwordDialog.value = true
}
async function submitPassword() {
  if (passwordValue.value.length < 8) {
    fail(new Error(t('passwordTooShort')))
    return
  }
  try {
    await api.resetPassword(passwordTarget.value!.id, passwordValue.value)
    passwordDialog.value = false
    succeed(t('saved'))
  } catch (problem) {
    fail(problem)
  }
}
async function toggleUser(user: User) {
  try {
    await api.updateUser(user.id, {
      username: user.username,
      root_dir: user.root_dir,
      permission: user.permission,
      quota: user.quota,
      disabled: !user.disabled
    })
    await load()
  } catch (problem) {
    fail(problem)
  }
}
async function removeUser(user: User) {
  if (!window.confirm(t('confirmUserDelete', { name: user.username }))) return
  try {
    await api.deleteUser(user.id)
    await load()
  } catch (problem) {
    fail(problem)
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
        <span class="eyebrow">{{ t('users') }}</span>
        <h1>{{ t('users') }}</h1>
        <span class="hint">{{ t('activeUsers') }}: {{ activeCount }} / {{ users.length }}</span>
      </div>
      <n-button type="primary" @click="openDialog()">{{ t('addUser') }}</n-button>
    </header>
    <n-data-table :columns="columns" :data="users" :loading="loading" :row-key="rowKey" :bordered="false" />

    <n-modal v-model:show="dialog" preset="card" class="form-card" :title="editing ? t('editUser') : t('addUser')" :mask-closable="false">
      <n-form @submit.prevent="save">
        <n-form-item :label="t('username')">
          <n-input v-model:value="form.username" autocomplete="username" />
        </n-form-item>
        <n-form-item v-if="!editing" :label="t('password')">
          <n-input v-model:value="form.password" type="password" autocomplete="new-password" />
        </n-form-item>
        <n-form-item :label="t('rootDir')">
          <n-input v-model:value="form.rootDir" :placeholder="form.username" />
        </n-form-item>
        <n-form-item :label="t('permission')">
          <n-select v-model:value="form.permission" :options="permissionOptions" />
        </n-form-item>
        <n-form-item :label="t('quota')">
          <div class="actions" style="width: 100%">
            <n-input v-model:value="form.quotaAmount" type="number" min="0" style="max-width: 160px" />
            <n-select v-model:value="form.quotaUnit" :options="quotaUnits" style="max-width: 120px" />
            <span class="hint">{{ form.quotaAmount ? formatSize(quotaToBytes(form.quotaAmount, form.quotaUnit)) : t('unlimited') }}</span>
          </div>
        </n-form-item>
        <n-form-item v-if="editing" :label="t('status')">
          <n-select v-model:value="form.disabled" :options="statusOptions.map((option) => ({ label: option.label, value: option.value === 'disabled' }))" />
        </n-form-item>
        <div class="modal-actions">
          <n-button @click="dialog = false">{{ t('cancel') }}</n-button>
          <n-button type="primary" attr-type="submit">{{ t('save') }}</n-button>
        </div>
      </n-form>
    </n-modal>

    <n-modal v-model:show="passwordDialog" preset="card" class="form-card" :title="t('resetPassword')" :mask-closable="false">
      <p>{{ t('newPasswordFor', { name: passwordTarget?.username || '' }) }}</p>
      <n-input v-model:value="passwordValue" type="password" autocomplete="new-password" />
      <div class="modal-actions" style="margin-top: 16px">
        <n-button @click="passwordDialog = false">{{ t('cancel') }}</n-button>
        <n-button type="primary" @click="submitPassword">{{ t('save') }}</n-button>
      </div>
    </n-modal>
  </section>
</template>
