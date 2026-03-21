/** 单批条数：控制单次 Wails 调用时长，块之间让出渲染 */
export const IMPORT_CHUNK_SIZE = 8

export function yieldToUI(): Promise<void> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => {
      setTimeout(resolve, 0)
    })
  })
}

export async function importBatched<T, R>(
  items: T[],
  importChunk: (slice: T[]) => Promise<R[]>,
  onProgress?: (accumulated: R[]) => void,
  chunkSize: number = IMPORT_CHUNK_SIZE,
): Promise<R[]> {
  if (items.length <= chunkSize) {
    const r = (await importChunk(items)) || []
    onProgress?.(r)
    return r
  }
  const acc: R[] = []
  for (let i = 0; i < items.length; i += chunkSize) {
    const slice = items.slice(i, i + chunkSize)
    const part = (await importChunk(slice)) || []
    acc.push(...part)
    onProgress?.([...acc])
    await yieldToUI()
  }
  return acc
}
