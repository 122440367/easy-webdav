<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NInput, NModal, NTabPane, NTabs } from 'naive-ui'
import { buildConnectInfo, currentBaseURL } from '../connect'

const props = defineProps<{ show: boolean; username: string; insecure: boolean }>()
const emit = defineEmits<{ 'update:show': [boolean] }>()
const { t } = useI18n()

const copied = ref('')
const info = computed(() =>
  buildConnectInfo({
    origin: typeof window === 'undefined' ? 'http://localhost' : window.location.origin,
    baseURL: currentBaseURL(),
    username: props.username,
    insecure: props.insecure
  })
)

function steps(key: string) {
  return t(key) as unknown as string[]
}
function onVisibility(value: unknown) {
  emit('update:show', Boolean(value))
}
async function copy(label: string, value: string) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value)
    } else {
      const area = document.createElement('textarea')
      area.value = value
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.appendChild(area)
      area.select()
      const ok = document.execCommand('copy')
      document.body.removeChild(area)
      if (!ok) throw new Error('copy rejected')
    }
    copied.value = label
  } catch {
    copied.value = 'failed'
  }
  window.setTimeout(() => {
    copied.value = ''
  }, 2500)
}
function copyLabel(label: string) {
  if (copied.value === 'failed') return t('connectCopyFailed')
  return copied.value === label ? t('connectCopied') : t('connectCopy')
}
</script>

<template>
  <n-modal :show="show" preset="card" class="connect-card" :title="t('connectTitle')" @update:show="onVisibility">
    <p class="hint">{{ t('connectIntro') }}</p>
    <div class="snippet-row">
      <n-input :value="info.davURL" readonly />
      <n-button size="small" @click="copy('url', info.davURL)">{{ copyLabel('url') }}</n-button>
    </div>
    <n-alert v-if="info.insecure" type="warning" class="alert">{{ t('connectInsecureWarning') }}</n-alert>

    <n-tabs type="line" animated>
      <n-tab-pane name="windows" :tab="t('connectWindows')">
        <ol class="steps">
          <li v-for="(step, index) in steps('connectWindowsSteps')" :key="index">{{ step }}</li>
        </ol>
        <div class="snippet-row">
          <pre class="snippet">{{ info.windowsPath }}</pre>
          <n-button size="tiny" @click="copy('unc', info.windowsPath)">{{ copyLabel('unc') }}</n-button>
        </div>
        <div class="snippet-row">
          <pre class="snippet">{{ info.netUseCommand }}</pre>
          <n-button size="tiny" @click="copy('netuse', info.netUseCommand)">{{ copyLabel('netuse') }}</n-button>
        </div>
      </n-tab-pane>

      <n-tab-pane name="macos" :tab="t('connectMac')">
        <ol class="steps">
          <li v-for="(step, index) in steps('connectMacSteps')" :key="index">{{ step }}</li>
        </ol>
        <div class="snippet-row">
          <pre class="snippet">{{ info.davURL }}</pre>
          <n-button size="tiny" @click="copy('mac', info.davURL)">{{ copyLabel('mac') }}</n-button>
        </div>
      </n-tab-pane>

      <n-tab-pane name="linux" :tab="t('connectLinux')">
        <ol class="steps">
          <li v-for="(step, index) in steps('connectLinuxSteps')" :key="index">{{ step }}</li>
        </ol>
        <div class="snippet-row">
          <pre class="snippet">{{ info.davfsCommand }}</pre>
          <n-button size="tiny" @click="copy('davfs', info.davfsCommand)">{{ copyLabel('davfs') }}</n-button>
        </div>
      </n-tab-pane>

      <n-tab-pane name="rclone" tab="rclone">
        <ol class="steps">
          <li v-for="(step, index) in steps('connectRcloneSteps')" :key="index">{{ step }}</li>
        </ol>
        <div class="snippet-row">
          <pre class="snippet">{{ info.rcloneCreateCommand }}</pre>
          <n-button size="tiny" @click="copy('rclone', info.rcloneCreateCommand)">{{ copyLabel('rclone') }}</n-button>
        </div>
        <div class="snippet-row">
          <pre class="snippet">{{ info.rcloneVerifyCommand }}</pre>
          <n-button size="tiny" @click="copy('verify', info.rcloneVerifyCommand)">{{ copyLabel('verify') }}</n-button>
        </div>
      </n-tab-pane>

      <n-tab-pane name="mobile" :tab="t('connectMobile')">
        <ol class="steps">
          <li v-for="(step, index) in steps('connectMobileSteps')" :key="index">{{ step }}</li>
        </ol>
        <div class="snippet-row">
          <pre class="snippet">{{ info.davURL }}</pre>
          <n-button size="tiny" @click="copy('mobile', info.davURL)">{{ copyLabel('mobile') }}</n-button>
        </div>
      </n-tab-pane>
    </n-tabs>
    <p class="hint">{{ t('connectCredentials') }}</p>
  </n-modal>
</template>
