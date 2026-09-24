<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NCard, NModal, NSpin } from 'naive-ui'
import { rawURL, type Entry } from '../api'
import { formatDate, formatSize } from '../format'
import { TEXT_PREVIEW_LIMIT, extensionOf, highlight, previewKind } from '../highlight'

const props = defineProps<{ entry: Entry; siblings: Entry[]; directory: string; userId?: string }>()
const emit = defineEmits<{ 'update:entry': [Entry]; close: [] }>()
const { t } = useI18n()

const text = ref('')
const textError = ref('')
const loadingText = ref(false)

const kind = computed(() => previewKind(props.entry.name))
const path = computed(() => [props.directory, props.entry.name].filter(Boolean).join('/'))
const url = computed(() => rawURL(path.value, props.userId))
const tooLarge = computed(() => kind.value === 'text' && props.entry.size > TEXT_PREVIEW_LIMIT)
const images = computed(() => props.siblings.filter((item) => previewKind(item.name) === 'image'))
const imageIndex = computed(() => images.value.findIndex((item) => item.name === props.entry.name))
const hasSiblings = computed(() => images.value.length > 1 && imageIndex.value >= 0)

function step(offset: number) {
  if (!hasSiblings.value) return
  const next = images.value[(imageIndex.value + offset + images.value.length) % images.value.length]
  if (next) emit('update:entry', next)
}

async function loadText() {
  text.value = ''
  textError.value = ''
  if (kind.value !== 'text' || tooLarge.value) return
  loadingText.value = true
  try {
    const response = await fetch(url.value, { credentials: 'include' })
    if (!response.ok) throw new Error(`${response.status}`)
    text.value = await response.text()
  } catch {
    textError.value = t('previewUnsupported')
  } finally {
    loadingText.value = false
  }
}

function onVisibility(value: unknown) {
  if (!value) emit('close')
}

watch(() => props.entry.name, loadText, { immediate: true })
</script>

<template>
  <n-modal :show="true" preset="card" class="preview" :title="entry.name" :mask-closable="true" @close="emit('close')" @update:show="onVisibility">
    <template #header-extra>
      <div class="actions">
        <n-button size="tiny" :disabled="!hasSiblings" :title="t('previewPrevious')" @click="step(-1)">◀</n-button>
        <n-button size="tiny" :disabled="!hasSiblings" :title="t('previewNext')" @click="step(1)">▶</n-button>
      </div>
    </template>

    <img v-if="kind === 'image'" :src="url" :alt="entry.name" />
    <iframe v-else-if="kind === 'pdf'" :src="url" :title="entry.name" />
    <video v-else-if="kind === 'video'" :src="url" controls />
    <audio v-else-if="kind === 'audio'" :src="url" controls />
    <n-spin v-else-if="kind === 'text' && !tooLarge && !textError" :show="loadingText">
      <pre v-if="text" v-html="highlight(text, extensionOf(entry.name))" />
      <div v-else style="min-height: 120px" />
    </n-spin>
    <n-card v-else size="small" class="info-card">
      <strong>{{ entry.name }}</strong>
      <span class="cell">{{ extensionOf(entry.name) || t('file') }} · {{ formatSize(entry.size) }} · {{ formatDate(entry.modified) }}</span>
      <span class="hint">{{ textError || (tooLarge ? t('previewTooLarge') : t('previewUnsupported')) }}</span>
      <n-button size="small" tag="a" :href="url" target="_blank" rel="noopener">{{ t('rawFile') }}</n-button>
    </n-card>
  </n-modal>
</template>
