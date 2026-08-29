/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  IMAGE_PLAYGROUND_DB_NAME,
  IMAGE_PLAYGROUND_DB_VERSION,
  IMAGE_PLAYGROUND_IMAGE_STORE,
  IMAGE_PLAYGROUND_TASK_STORE,
} from '../constants'
import type {
  ImageRequestParams,
  PlaygroundOperation,
  PlaygroundTaskStatus,
  StoredImage,
  StoredTask,
} from '../types'

function getIndexedDb(): IDBFactory | null {
  return typeof indexedDB === 'undefined' ? null : indexedDB
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.addEventListener('success', () => resolve(request.result), {
      once: true,
    })
    request.addEventListener(
      'error',
      () => reject(request.error || new Error('IndexedDB request failed')),
      { once: true }
    )
  })
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.addEventListener('complete', () => resolve(), { once: true })
    transaction.addEventListener(
      'error',
      () =>
        reject(transaction.error || new Error('IndexedDB transaction failed')),
      { once: true }
    )
    transaction.addEventListener(
      'abort',
      () =>
        reject(transaction.error || new Error('IndexedDB transaction aborted')),
      { once: true }
    )
  })
}

let databasePromise: Promise<IDBDatabase> | null = null

function openDatabase(): Promise<IDBDatabase> {
  const factory = getIndexedDb()
  if (!factory) {
    return Promise.reject(new Error('Browser storage is unavailable'))
  }
  if (databasePromise) return databasePromise

  const nextDatabasePromise: Promise<IDBDatabase> = new Promise<IDBDatabase>(
    (resolve, reject) => {
      const request = factory.open(
        IMAGE_PLAYGROUND_DB_NAME,
        IMAGE_PLAYGROUND_DB_VERSION
      )
      request.addEventListener('upgradeneeded', () => {
        const database = request.result
        if (!database.objectStoreNames.contains(IMAGE_PLAYGROUND_TASK_STORE)) {
          database.createObjectStore(IMAGE_PLAYGROUND_TASK_STORE, {
            keyPath: 'id',
          })
        }
        if (!database.objectStoreNames.contains(IMAGE_PLAYGROUND_IMAGE_STORE)) {
          database.createObjectStore(IMAGE_PLAYGROUND_IMAGE_STORE, {
            keyPath: 'id',
          })
        }
      })
      request.addEventListener('success', () => resolve(request.result), {
        once: true,
      })
      request.addEventListener(
        'error',
        () =>
          reject(request.error || new Error('Unable to open browser storage')),
        { once: true }
      )
    }
  )
  databasePromise = nextDatabasePromise
  void nextDatabasePromise.catch(() => {
    if (databasePromise === nextDatabasePromise) databasePromise = null
  })

  return nextDatabasePromise
}

export async function saveTaskBundle(
  task: StoredTask,
  images: StoredImage[]
): Promise<void> {
  const database = await openDatabase()
  const transaction = database.transaction(
    [IMAGE_PLAYGROUND_TASK_STORE, IMAGE_PLAYGROUND_IMAGE_STORE],
    'readwrite'
  )
  transaction.objectStore(IMAGE_PLAYGROUND_TASK_STORE).put(task)
  const imageStore = transaction.objectStore(IMAGE_PLAYGROUND_IMAGE_STORE)
  images.forEach((image) => imageStore.put(image))
  await transactionDone(transaction)
}

export async function listStoredTasks(): Promise<StoredTask[]> {
  const database = await openDatabase()
  const transaction = database.transaction(
    IMAGE_PLAYGROUND_TASK_STORE,
    'readonly'
  )
  const tasks = await requestResult<unknown[]>(
    transaction.objectStore(IMAGE_PLAYGROUND_TASK_STORE).getAll()
  )
  return tasks
    .map(normalizeStoredTask)
    .filter((task): task is StoredTask => task !== null)
    .sort((left, right) => right.createdAt - left.createdAt)
}

function normalizeStoredTask(value: unknown): StoredTask | null {
  if (!value || typeof value !== 'object') return null
  const raw = value as Partial<StoredTask> & { mode?: 'generate' | 'edit' }
  if (typeof raw.id !== 'string' || typeof raw.prompt !== 'string') return null
  const operation: PlaygroundOperation =
    raw.operation ||
    (raw.mode === 'edit' || raw.inputImageIds?.length ? 'edit' : 'generation')
  const status: PlaygroundTaskStatus = raw.status || 'done'
  const params: ImageRequestParams = raw.params || {
    size: 'auto',
    quality: 'auto',
    output_format: 'png',
    n: 1,
  }
  const createdAt = Number(raw.createdAt) || Date.now()
  const startedAt = Number(raw.startedAt) || createdAt
  let completedAt: number | null
  if (raw.completedAt == null) {
    completedAt = status === 'done' ? createdAt : null
  } else completedAt = Number(raw.completedAt)
  let elapsed: number | null
  if (raw.elapsed == null) {
    elapsed = completedAt == null ? null : Math.max(0, completedAt - startedAt)
  } else elapsed = Number(raw.elapsed)
  return {
    id: raw.id,
    operation,
    status,
    prompt: raw.prompt,
    model: raw.model || 'gpt-image-2',
    params,
    tokenId: Number(raw.tokenId) || 0,
    tokenName: raw.tokenName || '',
    createdAt,
    startedAt,
    completedAt,
    elapsed,
    error: raw.error || null,
    inputImageIds: Array.isArray(raw.inputImageIds)
      ? raw.inputImageIds.filter((id): id is string => typeof id === 'string')
      : [],
    outputImageIds: Array.isArray(raw.outputImageIds)
      ? raw.outputImageIds.filter((id): id is string => typeof id === 'string')
      : [],
    maskImageId: raw.maskImageId,
    maskTargetImageId: raw.maskTargetImageId,
    maskWidth: raw.maskWidth,
    maskHeight: raw.maskHeight,
    maskOriginalWidth: raw.maskOriginalWidth,
    maskOriginalHeight: raw.maskOriginalHeight,
    maskWasResized: raw.maskWasResized,
    actualParams: raw.actualParams,
    revisedPrompts: raw.revisedPrompts,
  }
}

export async function getStoredImage(
  id: string
): Promise<StoredImage | undefined> {
  const database = await openDatabase()
  const transaction = database.transaction(
    IMAGE_PLAYGROUND_IMAGE_STORE,
    'readonly'
  )
  return requestResult(
    transaction.objectStore(IMAGE_PLAYGROUND_IMAGE_STORE).get(id)
  )
}

export async function deleteTaskBundle(task: StoredTask): Promise<void> {
  const database = await openDatabase()
  const transaction = database.transaction(
    [IMAGE_PLAYGROUND_TASK_STORE, IMAGE_PLAYGROUND_IMAGE_STORE],
    'readwrite'
  )
  transaction.objectStore(IMAGE_PLAYGROUND_TASK_STORE).delete(task.id)
  const imageStore = transaction.objectStore(IMAGE_PLAYGROUND_IMAGE_STORE)
  ;[
    ...task.inputImageIds,
    ...task.outputImageIds,
    ...(task.maskImageId ? [task.maskImageId] : []),
  ].forEach((id) => imageStore.delete(id))
  await transactionDone(transaction)
}
