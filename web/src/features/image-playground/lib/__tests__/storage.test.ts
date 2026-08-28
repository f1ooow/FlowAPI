import { describe, expect, test } from 'vitest'

import { listStoredTasks } from '../storage'

describe('image playground browser history', () => {
  test('reports unavailable storage instead of blocking the feature', async () => {
    const originalIndexedDb = globalThis.indexedDB
    Object.defineProperty(globalThis, 'indexedDB', {
      configurable: true,
      value: undefined,
    })
    await expect(listStoredTasks()).rejects.toThrow(
      'Browser storage is unavailable'
    )
    Object.defineProperty(globalThis, 'indexedDB', {
      configurable: true,
      value: originalIndexedDb,
    })
  })
})
