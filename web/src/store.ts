import { defineStore } from 'pinia'
import { ref } from 'vue'
export const useSessionStore = defineStore('session', () => { const user=ref<any>(null); function setUser(value:any){user.value=value}; function clear(){user.value=null}; return { user, setUser, clear } })
