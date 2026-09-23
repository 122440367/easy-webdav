import { createI18n } from 'vue-i18n'

const saved = localStorage.getItem('ew-locale')
const browser = navigator.languages?.find((item) => item.toLowerCase().startsWith('zh')) ? 'zh-CN' : 'en'
export const i18n = createI18n({ legacy: false, locale: saved || browser, fallbackLocale: 'en', messages: {
  en: { title: 'easy-webdav', login: 'Sign in', setup: 'Create your administrator', username: 'Username', password: 'Password', files: 'My files', empty: 'This folder is empty', root: 'Root', refresh: 'Refresh', signOut: 'Sign out', insecure: 'This connection is not encrypted.' },
  'zh-CN': { title: 'easy-webdav', login: '登录', setup: '创建管理员', username: '用户名', password: '密码', files: '我的文件', empty: '此文件夹为空', root: '根目录', refresh: '刷新', signOut: '退出登录', insecure: '当前连接未加密。' }
} })
