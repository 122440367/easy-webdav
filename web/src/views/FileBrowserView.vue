<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NCheckbox, NDropdown, NEmpty, NInput, NModal, NProgress, NSelect, NVirtualList } from 'naive-ui'
import { ApiError, api, downloadURL, type Entry, type User } from '../api'
import { errorKey } from '../error-codes'
import { formatDate, formatSize, usagePercent } from '../format'
import { useSessionStore } from '../store'
import { createTask, runUpload, type ConflictChoice, type UploadTask } from '../uploader'
import PreviewOverlay from '../components/PreviewOverlay.vue'

type Picked = { file: File; relativePath: string }

const session = useSessionStore()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const entries = ref<Entry[]>([])
const loading = ref(false)
const error = ref('')
const notice = ref('')
const query = ref('')
const hidden = ref(localStorage.getItem('ew-hidden') === 'true')
const sortKey = ref<'name' | 'size' | 'modified'>('name')
const sortDirection = ref(1)
const selected = ref<string[]>([])
const previewEntry = ref<Entry | null>(null)
const usage = ref<{ used: number; quota: number }>({ used: 0, quota: 0 })
const targetUser = ref<User | null>(null)
const uploadTasks = ref<UploadTask[]>([])
const dropActive = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const fileInputFolder = ref<HTMLInputElement | null>(null)

const nameModal = ref(false)
const nameAction = ref<'mkdir' | 'rename' | 'copy'>('mkdir')
const nameValue = ref('')
const nameTarget = ref<Entry | null>(null)
const moveModal = ref(false)
const moveValue = ref('')
const moveTarget = ref<Entry | null>(null)
const deleteModal = ref(false)
const deleteTarget = ref<Entry | null>(null)
const conflictModal = ref(false)
const conflictName = ref('')
const conflictApplyAll = ref(false)
const conflictChoice = ref<ConflictChoice>('rename')
let conflictResolver: ((value: ConflictChoice | null) => void) | null = null

const userId = computed(() => (route.name === 'user-files' ? String(route.params.userId) : ''))
const segments = computed(() => {
  const value = route.params.pathMatch
  const list = Array.isArray(value) ? value : value ? [value] : []
  return list.filter((part) => part !== '' && part !== undefined) as string[]
})
const currentPath = computed(() => segments.value.join('/'))
const canWrite = computed(() => Boolean(session.user && (userId.value || session.user.permission !== 'read')))
const rows = computed(() =>
  entries.value
    .filter((entry) => hidden.value || !entry.name.startsWith('.'))
    .filter((entry) => entry.name.toLowerCase().includes(query.value.toLowerCase()))
    .sort((left, right) => {
      if (left.directory !== right.directory) return left.directory ? -1 : 1
      if (sortKey.value === 'size') return (left.size - right.size) * sortDirection.value
      if (sortKey.value === 'modified') return (new Date(left.modified).getTime() - new Date(right.modified).getTime()) * sortDirection.value
      return left.name.localeCompare(right.name) * sortDirection.value
    })
)
const conflictOptions = computed(() => [
  { label: t('conflictRename'), value: 'rename' },
  { label: t('conflictOverwrite'), value: 'overwrite' },
  { label: t('conflictSkip'), value: 'skip' }
])

function join(name: string) {
  return [currentPath.value, name].filter(Boolean).join('/')
}
function toggleSelect(name: string) {
  selected.value = selected.value.includes(name) ? selected.value.filter((item) => item !== name) : [...selected.value, name]
}
function setPreview(entry: Entry) {
  previewEntry.value = entry
}
function fail(problem: unknown) {
  notice.value = ''
  error.value = problem instanceof ApiError ? t(errorKey(problem.code)) : String(problem)
  if (problem instanceof ApiError && problem.code === 'QUOTA_EXCEEDED') {
    const remaining = Number(problem.details?.remaining ?? 0)
    error.value = remaining > 0 ? `${t('errors.quotaExceeded')} · ${t('quotaRemaining', { size: formatSize(remaining) })}` : t('errors.quotaExceeded')
  }
}
function succeed(message: string) {
  error.value = ''
  notice.value = message
}
function go(path = currentPath.value) {
  const tail = path.split('/').filter(Boolean).map((part) => encodeURIComponent(part)).join('/')
  const prefix = userId.value ? `/files/u/${userId.value}` : '/files'
  router.push(tail ? `${prefix}/${tail}` : prefix)
}
function setSort(key: 'name' | 'size' | 'modified') {
  if (sortKey.value === key) sortDirection.value = -sortDirection.value
  else {
    sortKey.value = key
    sortDirection.value = 1
  }
}
function sortMark(key: 'name' | 'size' | 'modified') {
  return sortKey.value === key ? (sortDirection.value === 1 ? ' ▲' : ' ▼') : ''
}

async function browse() {
  loading.value = true
  try {
    const result = await api.list(currentPath.value, userId.value)
    entries.value = result.entries || []
    selected.value = []
    error.value = ''
  } catch (problem) {
    fail(problem)
  } finally {
    loading.value = false
  }
}

async function loadUsage() {
  try {
    if (userId.value && session.isAdmin) {
      const overview = await api.usageOverview()
      const match = overview.users.find((item) => String(item.user.id) === userId.value)
      targetUser.value = match?.user || null
      usage.value = { used: match?.used || 0, quota: match?.user.quota || 0 }
    } else {
      const mine = await api.usageMe()
      usage.value = { used: mine.used, quota: mine.quota }
    }
  } catch {
    usage.value = { used: 0, quota: 0 }
  }
}

async function reload() {
  await Promise.all([browse(), loadUsage()])
}

async function action(method: string, body: unknown) {
  await api.fileAction(method, body, userId.value)
  await reload()
}

function promptName(kind: 'mkdir' | 'rename' | 'copy', entry?: Entry) {
  nameAction.value = kind
  nameTarget.value = entry || null
  nameValue.value = kind === 'rename' ? entry?.name || '' : kind === 'copy' ? `${entry?.name || ''} copy` : ''
  nameModal.value = true
}
async function submitName() {
  const value = nameValue.value.trim()
  if (!value) return
  try {
    if (nameAction.value === 'mkdir') {
      await action('POST', { path: join(value) })
    } else if (nameAction.value === 'rename') {
      await action('PUT', { action: 'move', path: join(nameTarget.value?.name || ''), destination: join(value) })
    } else {
      await action('PUT', { action: 'copy', path: join(nameTarget.value?.name || ''), destination: join(value) })
    }
    nameModal.value = false
  } catch (problem) {
    fail(problem)
  }
}
function promptMove(entry: Entry) {
  moveTarget.value = entry
  moveValue.value = join(entry.name)
  moveModal.value = true
}
async function submitMove() {
  const value = moveValue.value.trim()
  if (!value) return
  try {
    await action('PUT', { action: 'move', path: join(moveTarget.value?.name || ''), destination: value })
    moveModal.value = false
  } catch (problem) {
    fail(problem)
  }
}
function promptDelete(entry: Entry) {
  deleteTarget.value = entry
  deleteModal.value = true
}
async function confirmDelete() {
  try {
    await action('DELETE', { path: join(deleteTarget.value?.name || '') })
    deleteModal.value = false
  } catch (problem) {
    fail(problem)
  }
}
function download(names?: string[]) {
  const paths = names?.length ? names.map(join) : selected.value.map(join)
  if (!paths.length) return
  location.assign(downloadURL(paths, userId.value))
}

function rowOptions(entry: Entry) {
  const options = [{ label: t('download'), key: 'download' }]
  if (canWrite.value) {
    options.push(
      { label: t('rename'), key: 'rename' },
      { label: t('move'), key: 'move' },
      { label: t('copy'), key: 'copy' },
      { label: t('delete'), key: 'delete' }
    )
  }
  return options
}
function rowAction(entry: Entry, key: string) {
  if (key === 'download') download([entry.name])
  else if (key === 'rename') promptName('rename', entry)
  else if (key === 'move') promptMove(entry)
  else if (key === 'copy') promptName('copy', entry)
  else if (key === 'delete') promptDelete(entry)
}
function rowSelect(entry: Entry, key: unknown) {
  rowAction(entry, String(key))
}

function hasConflict(relativePath: string) {
  const top = relativePath.split('/')[0]
  return entries.value.some((entry) => entry.name === top)
}
function askConflict(name: string) {
  conflictName.value = name
  conflictChoice.value = 'rename'
  conflictApplyAll.value = false
  conflictModal.value = true
  return new Promise<ConflictChoice | null>((resolve) => {
    conflictResolver = resolve
  })
}
function resolveConflict(value: ConflictChoice | null) {
  conflictModal.value = false
  const resolver = conflictResolver
  conflictResolver = null
  resolver?.(value)
}

async function uploadPicked(picked: Picked[]) {
  if (!picked.length) return
  const tasks: UploadTask[] = []
  let applyAll: ConflictChoice | null = null
  for (const item of picked) {
    const task = createTask(item.file, item.relativePath, 'rename')
    if (hasConflict(item.relativePath)) {
      if (!applyAll) {
        const choice = await askConflict(item.relativePath)
        if (conflictApplyAll.value && choice) applyAll = choice
        if (!choice || choice === 'skip') continue
        task.conflict = choice
      } else {
        if (applyAll === 'skip') continue
        task.conflict = applyAll
      }
    }
    tasks.push(task)
  }
  if (!tasks.length) return
  uploadTasks.value = tasks
  let failed = 0
  for (const task of tasks) {
    try {
      await runUpload(task, {
        userId: userId.value,
        directory: currentPath.value,
        onProgress: () => {
          uploadTasks.value = [...uploadTasks.value]
        }
      })
    } catch (problem) {
      failed += 1
      fail(problem)
    }
  }
  await reload()
  if (!failed) succeed(t('uploadComplete'))
  window.setTimeout(() => {
    uploadTasks.value = []
  }, 4000)
}

async function readEntries(reader: any): Promise<any[]> {
  const result: any[] = []
  for (;;) {
    const batch: any[] = await new Promise((resolve, reject) => reader.readEntries(resolve, reject))
    if (!batch.length) break
    result.push(...batch)
  }
  return result
}
async function collectEntry(entry: any, prefix: string, out: Picked[]) {
  if (!entry) return
  if (entry.isFile) {
    const file: File = await new Promise((resolve, reject) => entry.file(resolve, reject))
    out.push({ file, relativePath: `${prefix}${file.name}` })
    return
  }
  if (entry.isDirectory) {
    const children = await readEntries(entry.createReader())
    for (const child of children) await collectEntry(child, `${prefix}${entry.name}/`, out)
  }
}
async function onDrop(event: DragEvent) {
  dropActive.value = false
  const items = event.dataTransfer?.items
  const picked: Picked[] = []
  if (items?.length && typeof (items[0] as any).webkitGetAsEntry === 'function') {
    for (const item of Array.from(items)) {
      const entry = (item as any).webkitGetAsEntry?.()
      if (entry) await collectEntry(entry, '', picked)
    }
  } else {
    for (const file of Array.from(event.dataTransfer?.files || [])) {
      const relative = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name
      picked.push({ file, relativePath: relative })
    }
  }
  if (!picked.length && event.dataTransfer?.files?.length) {
    for (const file of Array.from(event.dataTransfer.files)) picked.push({ file, relativePath: file.name })
  }
  await uploadPicked(picked)
}
async function onPick(input: HTMLInputElement | null) {
  const files = input?.files
  if (!files?.length) return
  const picked = Array.from(files).map((file) => ({
    file,
    relativePath: (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name
  }))
  if (input) input.value = ''
  await uploadPicked(picked)
}

watch(() => route.fullPath, () => {
  if (String(route.name).startsWith('files') || route.name === 'user-files') reload()
})
watch(hidden, (value) => localStorage.setItem('ew-hidden', String(value)))
onMounted(reload)
</script>

<template>
  <section
    :class="{ 'drop-active': dropActive }"
    @dragover.prevent="canWrite && (dropActive = true)"
    @dragleave="dropActive = false"
    @drop.prevent="onDrop($event)"
  >
    <n-alert v-if="error" type="error" closable class="alert" @close="error = ''">{{ error }}</n-alert>
    <n-alert v-if="notice" type="success" closable class="alert" @close="notice = ''">{{ notice }}</n-alert>
    <div v-if="userId" class="notice-bar">
      <span>{{ t('viewingUser', { name: targetUser?.username || `#${userId}` }) }}</span>
      <n-button size="small" @click="$router.push('/users')">{{ t('leaveUser') }}</n-button>
    </div>
    <header class="heading">
      <div>
        <span class="eyebrow">{{ t('fileBrowser') }}</span>
        <h1>{{ currentPath || (userId ? t('filesOf', { name: targetUser?.username || `#${userId}` }) : t('files')) }}</h1>
        <div class="crumb">
          <button @click="go('')">{{ t('root') }}</button>
          <template v-for="(part, index) in segments" :key="index">
            <span>/</span>
            <button @click="go(segments.slice(0, index + 1).join('/'))">{{ part }}</button>
          </template>
        </div>
      </div>
      <n-card v-if="!userId" class="usage-card" size="small">
        <span class="cell">{{ t('storageUsed') }}</span>
        <strong>{{ formatSize(usage.used) }} <small v-if="usage.quota">/ {{ formatSize(usage.quota) }}</small></strong>
        <n-progress v-if="usage.quota" type="line" :percentage="usagePercent(usage.used, usage.quota)" :show-indicator="false" />
      </n-card>
    </header>

    <div class="toolbar">
      <n-button size="small" @click="reload">{{ t('refresh') }}</n-button>
      <n-button v-if="canWrite" size="small" type="primary" @click="fileInput?.click()">{{ t('upload') }}</n-button>
      <n-button v-if="canWrite" size="small" @click="fileInputFolder?.click()">{{ t('uploadFolder') }}</n-button>
      <n-button v-if="canWrite" size="small" @click="promptName('mkdir')">{{ t('newFolder') }}</n-button>
      <n-button v-if="selected.length" size="small" @click="download()">{{ t('downloadSelected') }} ({{ selected.length }})</n-button>
      <n-input v-model:value="query" class="search" size="small" clearable :placeholder="t('search')" />
      <n-checkbox v-model:checked="hidden">{{ t('showHidden') }}</n-checkbox>
      <input ref="fileInput" class="hidden-input" type="file" multiple @change="onPick(fileInput)" />
      <input ref="fileInputFolder" class="hidden-input" type="file" webkitdirectory multiple aria-hidden="true" @change="onPick(fileInputFolder)" />
    </div>
    <p v-if="canWrite" class="hint">{{ t('dropFiles') }}</p>

    <div v-if="uploadTasks.length" class="upload-panel">
      <div v-for="task in uploadTasks" :key="task.relativePath + task.file.size" class="upload-item">
        <span class="upload-name">{{ task.relativePath }}</span>
        <n-progress
          type="line"
          :status="task.state.status === 'failed' ? 'error' : task.state.status === 'complete' ? 'success' : 'default'"
          :percentage="Math.round((task.state.completed / Math.max(1, task.state.total)) * 100)"
          :show-indicator="false"
        />
      </div>
    </div>

    <n-empty v-if="!rows.length && !loading" :description="query ? t('noResults') : t('empty')">
      <template #extra>
        <div class="actions" style="justify-content: center">
          <n-button v-if="canWrite" size="small" type="primary" @click="fileInput?.click()">{{ t('upload') }}</n-button>
          <n-button v-if="canWrite" size="small" @click="promptName('mkdir')">{{ t('newFolder') }}</n-button>
        </div>
      </template>
    </n-empty>

    <template v-else>
      <div class="list-head">
        <span />
        <button class="link-button" @click="setSort('name')">{{ t('name') }}{{ sortMark('name') }}</button>
        <button class="link-button" @click="setSort('size')">{{ t('size') }}{{ sortMark('size') }}</button>
        <button class="link-button" @click="setSort('modified')">{{ t('modified') }}{{ sortMark('modified') }}</button>
        <span class="head-actions">{{ t('actions') }}</span>
      </div>
      <n-virtual-list class="list-body" :items="rows" :item-size="57">
        <template #default="{ item }">
          <div class="row">
            <n-checkbox :checked="selected.includes(item.name)" @update:checked="toggleSelect(item.name)" />
            <button class="filename" @click="item.directory ? go(join(item.name)) : (previewEntry = item)">
              <span class="badge">{{ item.directory ? t('folder') : t('file') }}</span>
              <span>{{ item.name }}</span>
            </button>
            <span class="cell">{{ item.directory ? '—' : formatSize(item.size) }}</span>
            <span class="cell">{{ formatDate(item.modified) }}</span>
            <div class="actions">
              <n-button size="tiny" @click="download([item.name])">{{ t('download') }}</n-button>
              <template v-if="canWrite">
                <n-button size="tiny" @click="promptName('rename', item)">{{ t('rename') }}</n-button>
                <n-button size="tiny" @click="promptMove(item)">{{ t('move') }}</n-button>
                <n-button size="tiny" @click="promptName('copy', item)">{{ t('copy') }}</n-button>
                <n-button size="tiny" type="error" @click="promptDelete(item)">{{ t('delete') }}</n-button>
              </template>
            </div>
            <div class="row-menu">
              <n-dropdown trigger="click" :options="rowOptions(item)" @select="rowSelect(item, $event)">
                <n-button size="tiny" quaternary>⋯</n-button>
              </n-dropdown>
            </div>
          </div>
        </template>
      </n-virtual-list>
    </template>

    <n-modal v-model:show="nameModal" preset="card" class="form-card" :title="nameAction === 'mkdir' ? t('newFolder') : nameAction === 'rename' ? t('rename') : t('copyName')" :mask-closable="false">
      <n-input v-model:value="nameValue" :placeholder="nameAction === 'mkdir' ? t('folderName') : t('newName')" @keyup.enter="submitName" />
      <div class="modal-actions" style="margin-top: 16px">
        <n-button @click="nameModal = false">{{ t('cancel') }}</n-button>
        <n-button type="primary" @click="submitName">{{ t('save') }}</n-button>
      </div>
    </n-modal>

    <n-modal v-model:show="moveModal" preset="card" class="form-card" :title="t('move')" :mask-closable="false">
      <n-input v-model:value="moveValue" :placeholder="t('moveTo', { name: moveTarget?.name || '' })" @keyup.enter="submitMove" />
      <div class="modal-actions" style="margin-top: 16px">
        <n-button @click="moveModal = false">{{ t('cancel') }}</n-button>
        <n-button type="primary" @click="submitMove">{{ t('save') }}</n-button>
      </div>
    </n-modal>

    <n-modal v-model:show="deleteModal" preset="card" class="form-card" :title="t('delete')" :mask-closable="false">
      <p>{{ t('confirmDelete', { name: deleteTarget?.name || '' }) }}</p>
      <div class="modal-actions">
        <n-button @click="deleteModal = false">{{ t('cancel') }}</n-button>
        <n-button type="error" @click="confirmDelete">{{ t('delete') }}</n-button>
      </div>
    </n-modal>

    <n-modal v-model:show="conflictModal" preset="card" class="form-card" :title="t('conflict')" :mask-closable="false">
      <p>{{ t('conflictQuestion', { name: conflictName }) }}</p>
      <n-select v-model:value="conflictChoice" :options="conflictOptions" />
      <n-checkbox v-model:checked="conflictApplyAll" style="margin-top: 12px">{{ t('conflictApplyAll') }}</n-checkbox>
      <div class="modal-actions" style="margin-top: 16px">
        <n-button @click="resolveConflict(null)">{{ t('cancel') }}</n-button>
        <n-button type="primary" @click="resolveConflict(conflictChoice)">{{ t('save') }}</n-button>
      </div>
    </n-modal>

    <preview-overlay
      v-if="previewEntry"
      :entry="previewEntry"
      :siblings="rows"
      :user-id="userId"
      @update:entry="setPreview"
      @close="previewEntry = null"
    />
  </section>
</template>
