import {
  IMAGE_PLAYGROUND_DB_NAME,
  IMAGE_PLAYGROUND_DB_VERSION,
  IMAGE_PLAYGROUND_IMAGE_STORE,
  IMAGE_PLAYGROUND_TASK_STORE,
} from '../constants'
import type { StoredImage, StoredTask } from '../types'

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
  const tasks = await requestResult(
    transaction.objectStore(IMAGE_PLAYGROUND_TASK_STORE).getAll()
  )
  return tasks.sort((left, right) => right.createdAt - left.createdAt)
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
  ;[...task.inputImageIds, ...task.outputImageIds].forEach((id) =>
    imageStore.delete(id)
  )
  await transactionDone(transaction)
}
