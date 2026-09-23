import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import { useSessionStore } from './store'
describe('session store',()=>{it('sets and clears user',()=>{setActivePinia(createPinia());const store=useSessionStore();store.setUser({id:1});expect(store.user.id).toBe(1);store.clear();expect(store.user).toBeNull()})})
