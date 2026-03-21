import { ref } from 'vue'

export type ToastKind = 'success' | 'error' | 'info'

export interface ToastItem {
  id: number
  message: string
  kind: ToastKind
}

export const toastQueue = ref<ToastItem[]>([])

let toastSeq = 0

/** 非阻塞提示；支持 message 内换行（white-space: pre-line） */
export function showToast(message: string, kind: ToastKind = 'info', durationMs = 4800): void {
  const id = ++toastSeq
  toastQueue.value = [...toastQueue.value, { id, message, kind }]
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
