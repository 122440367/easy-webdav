import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, type User } from './api'

/** Session plus the runtime settings every page needs. */
export const useSessionStore = defineStore('session', () => {
  const user = ref<User | null>(null)
  const insecure = ref(false)
  const siteName = ref('easy-webdav')
  const settings = ref<Record<string, string>>({})
  const setupRequired = ref(false)
  const loaded = ref(false)

  const isAdmin = computed(() => user.value?.role === 'admin')

  function setUser(value: User | null) {
    user.value = value
  }
  function clear() {
    user.value = null
  }
  function applySettings(values: Record<string, string>) {
    settings.value = values
    if (values.site_name) siteName.value = values.site_name
    document.title = siteName.value
  }

  async function loadSettings() {
    try {
      applySettings(await api.settings())
    } catch {
      /* settings are optional for the login screen */
    }
  }

  /** Resolves the current session; falls back to the setup status endpoint. */
  async function bootstrap() {
    try {
      const result = await api.me()
      user.value = result.user
      insecure.value = result.insecure
      setupRequired.value = false
      await loadSettings()
    } catch {
      user.value = null
      try {
        const status = await api.setupStatus()
        setupRequired.value = status.required
        insecure.value = status.insecure
        if (status.site_name) {
          siteName.value = status.site_name
          document.title = status.site_name
        }
      } catch {
        setupRequired.value = false
      }
    } finally {
      loaded.value = true
    }
  }

  async function signIn(username: string, password: string) {
    await api.login(username, password)
    await bootstrap()
  }

  async function signUp(username: string, password: string) {
    await api.setup(username, password)
    await bootstrap()
  }

  async function signOut() {
    try {
      await api.logout()
    } catch {
      /* the cookie is cleared locally either way */
    }
    clear()
    setupRequired.value = false
    await bootstrap()
  }

  return { user, insecure, siteName, settings, setupRequired, loaded, isAdmin, setUser, clear, applySettings, loadSettings, bootstrap, signIn, signUp, signOut }
})
