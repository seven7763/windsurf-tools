import { ref, watch } from 'vue'

const STORAGE_KEY = 'windsurf-tools-theme'

export type ThemeMode = 'system' | 'light' | 'dark'

export const themeMode = ref<ThemeMode>('system')

function readStored(): ThemeMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY) as ThemeMode | null
    if (v === 'light' || v === 'dark' || v === 'system') {
      return v
    }
  } catch {
    /* ignore */
  }
  return 'system'
}

themeMode.value = readStored()

export function applyTheme(): void {
  const root = document.documentElement
  let dark = false
  if (themeMode.value === 'dark') {
    dark = true
  } else if (themeMode.value === 'light') {
    dark = false
  } else {
    dark = window.matchMedia('(prefers-color-scheme: dark)').matches
  }
  if (dark) {
    root.classList.add('dark')
  } else {
    root.classList.remove('dark')
  }
}

applyTheme()

watch(themeMode, () => {
  applyTheme()
  try {
    localStorage.setItem(STORAGE_KEY, themeMode.value)
  } catch {
    /* ignore */
  }
})

if (typeof window !== 'undefined') {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (themeMode.value === 'system') {
      applyTheme()
    }
  })
}

export function cycleTheme(): void {
  const order: ThemeMode[] = ['system', 'light', 'dark']
  const i = order.indexOf(themeMode.value)
  themeMode.value = order[(i + 1) % order.length]!
}

export function themeLabel(mode: ThemeMode): string {
  switch (mode) {
    case 'system':
      return '主题：跟随系统'
    case 'light':
      return '主题：浅色'
    case 'dark':
      return '主题：深色'
    default:
      return '主题'
  }
}
