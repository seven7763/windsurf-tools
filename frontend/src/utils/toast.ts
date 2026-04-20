import { ref } from 'vue'

export type ToastKind = 'success' | 'error' | 'info' | 'warning'

export interface ToastItem {
  id: number
  message: string
  kind: ToastKind
}

export const toastQueue = ref<ToastItem[]>([])

let toastSeq = 0

const MAX_TOAST_QUEUE = 6

/** 非阻塞提示；支持 message 内换行（white-space: pre-line） */
export function showToast(message: string, kind: ToastKind = 'info', durationMs = 4800): void {
  const id = ++toastSeq
  const next = [...toastQueue.value, { id, message, kind }]
  toastQueue.value = next.length > MAX_TOAST_QUEUE ? next.slice(-MAX_TOAST_QUEUE) : next
  window.setTimeout(() => {
    toastQueue.value = toastQueue.value.filter((t) => t.id !== id)
  }, durationMs)
}

export interface ConfirmState {
  visible: boolean
  message: string
  confirmText: string
  cancelText: string
  destructive: boolean
  resolve: ((value: boolean) => void) | null
}

export const confirmState = ref<ConfirmState>({
  visible: false,
  message: '',
  confirmText: '确定',
  cancelText: '取消',
  destructive: false,
  resolve: null,
})

export function resolveConfirm(value: boolean): void {
  const r = confirmState.value.resolve
  confirmState.value.visible = false
  confirmState.value.resolve = null
  r?.(value)
}

export function confirmDialog(
  message: string,
  options?: { confirmText?: string; cancelText?: string; destructive?: boolean },
): Promise<boolean> {
  const prev = confirmState.value.resolve
  if (prev) {
    prev(false)
  }
  return new Promise((resolve) => {
    confirmState.value = {
      visible: true,
      message,
      confirmText: options?.confirmText ?? '确定',
      cancelText: options?.cancelText ?? '取消',
      destructive: options?.destructive ?? false,
      resolve,
    }
  })
}
