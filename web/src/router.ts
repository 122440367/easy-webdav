import { createRouter, createWebHistory } from 'vue-router'
import { useSessionStore } from './store'
import AccountView from './views/AccountView.vue'
import FileBrowserView from './views/FileBrowserView.vue'
import LoginView from './views/LoginView.vue'
import OverviewView from './views/OverviewView.vue'
import SettingsView from './views/SettingsView.vue'
import UsersView from './views/UsersView.vue'

const base = document.querySelector('base')?.getAttribute('href') || '/'

const adminOnly = new Set(['users', 'overview', 'settings'])

export const router = createRouter({
  history: createWebHistory(base),
  routes: [
    { path: '/', redirect: '/files' },
    { path: '/login', name: 'login', component: LoginView, meta: { public: true } },
    { path: '/files/u/:userId/:pathMatch(.*)*', name: 'user-files', component: FileBrowserView },
    { path: '/files/:pathMatch(.*)*', name: 'files', component: FileBrowserView },
    { path: '/users', name: 'users', component: UsersView },
    { path: '/overview', name: 'overview', component: OverviewView },
    { path: '/settings', name: 'settings', component: SettingsView },
    { path: '/account', name: 'account', component: AccountView },
    { path: '/:pathMatch(.*)*', redirect: '/files' }
  ]
})

router.beforeEach(async (to) => {
  const session = useSessionStore()
  if (!session.loaded) await session.bootstrap()
  const isPublic = to.meta.public === true
  if (!session.user && !isPublic) {
    return { name: 'login', query: to.fullPath === '/files' ? {} : { redirect: to.fullPath } }
  }
  if (session.user && isPublic) {
    return { name: 'files' }
  }
  const section = String(to.path).split('/')[1]
  if (session.user && adminOnly.has(section) && !session.isAdmin) {
    return { name: 'files' }
  }
  return true
})
