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
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DEFAULT_IMAGE_PARAMS,
  IMAGE_PLAYGROUND_MAX_INPUT_IMAGES,
} from '../constants'
import {
  callImageEdit,
  callImageGeneration,
  resolvePlaygroundToken,
} from '../lib/api'
import {
  deleteTaskBundle,
  getStoredImage,
  listStoredTasks,
  saveTaskBundle,
} from '../lib/storage'
import type {
  ImageRequestParams,
  NormalizedImage,
  PlaygroundImage,
  PlaygroundMask,
  PlaygroundOperation,
  PlaygroundTask,
  StoredImage,
  StoredTask,
} from '../types'

function createId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === 'AbortError'
}

function localizedError(
  error: unknown,
  t: ReturnType<typeof useTranslation>['t']
): string {
  const message = error instanceof Error ? error.message : ''
  const known: Record<string, string> = {
    'Enter a prompt before generating': 'Enter a prompt before generating',
    'Select an API key before generating':
      'Select an API key before generating',
    'Add an image before editing': 'Add an image before editing',
    'Unable to load the selected API key':
      'Unable to load the selected API key',
    'The image service returned no images':
      'The image service returned no images',
    'This image is no longer available for editing':
      'This image is no longer available for editing',
    'Could not save this result to browser history':
      'Could not save this result to browser history',
    'Could not delete the selected history item':
      'Could not delete the selected history item',
    'Generation stopped': 'Generation stopped',
  }
  if (known[message]) return t(known[message])
  if (message.startsWith('Image request failed (')) {
    return t('Image request failed')
  }
  return message || t('Image request failed')
}

function releaseImage(image: PlaygroundImage, urls: Set<string>): void {
  if (!image.src.startsWith('blob:')) return
  URL.revokeObjectURL(image.src)
  urls.delete(image.src)
}

function cloneImage(
  image: PlaygroundImage,
  urls: Set<string>
): PlaygroundImage {
  if (!image.blob) return { ...image }
  const src = URL.createObjectURL(image.blob)
  urls.add(src)
  return { ...image, id: createId(), src }
}

function cloneMask(mask: PlaygroundMask, urls: Set<string>): PlaygroundMask {
  const src = URL.createObjectURL(mask.blob)
  urls.add(src)
  return { ...mask, src }
}

function storedImageToDisplay(
  image: StoredImage,
  urls: Set<string>
): PlaygroundImage {
  if (image.blob) {
    const src = URL.createObjectURL(image.blob)
    urls.add(src)
    return { ...image, src, role: image.role === 'mask' ? 'input' : image.role }
  }
  return {
    id: image.id,
    src: image.sourceUrl || '',
    sourceUrl: image.sourceUrl,
    mimeType: image.mimeType,
    width: image.width,
    height: image.height,
    role: image.role === 'mask' ? 'input' : image.role,
  }
}

async function hydrateTask(
  stored: StoredTask,
  urls: Set<string>
): Promise<PlaygroundTask> {
  const [inputs, outputs, storedMask] = await Promise.all([
    Promise.all(stored.inputImageIds.map(getStoredImage)),
    Promise.all(stored.outputImageIds.map(getStoredImage)),
    stored.maskImageId
      ? getStoredImage(stored.maskImageId)
      : Promise.resolve(undefined),
  ])
  let mask: PlaygroundMask | undefined
  if (
    storedMask?.blob &&
    stored.maskTargetImageId &&
    stored.maskWidth &&
    stored.maskHeight
  ) {
    const src = URL.createObjectURL(storedMask.blob)
    urls.add(src)
    mask = {
      targetImageId: stored.maskTargetImageId,
      blob: storedMask.blob,
      src,
      width: stored.maskWidth,
      height: stored.maskHeight,
      originalWidth: stored.maskOriginalWidth,
      originalHeight: stored.maskOriginalHeight,
      wasResized: stored.maskWasResized,
    }
  }
  return {
    id: stored.id,
    operation: stored.operation,
    status: stored.status,
    prompt: stored.prompt,
    model: stored.model,
    params: stored.params,
    tokenId: stored.tokenId,
    tokenName: stored.tokenName,
    createdAt: stored.createdAt,
    startedAt: stored.startedAt,
    completedAt: stored.completedAt,
    elapsed: stored.elapsed,
    error: stored.error,
    actualParams: stored.actualParams,
    revisedPrompts: stored.revisedPrompts,
    mask,
    inputImages: inputs
      .filter((image): image is StoredImage => Boolean(image))
      .map((image) => storedImageToDisplay(image, urls)),
    outputImages: outputs
      .filter((image): image is StoredImage => Boolean(image))
      .map((image) => storedImageToDisplay(image, urls)),
  }
}

function taskToStored(task: PlaygroundTask): StoredTask {
  return {
    id: task.id,
    operation: task.operation,
    status: task.status,
    prompt: task.prompt,
    model: task.model,
    params: task.params,
    tokenId: task.tokenId,
    tokenName: task.tokenName,
    createdAt: task.createdAt,
    startedAt: task.startedAt,
    completedAt: task.completedAt,
    elapsed: task.elapsed,
    error: task.error,
    actualParams: task.actualParams,
    revisedPrompts: task.revisedPrompts,
    inputImageIds: task.inputImages.map((image) => image.id),
    outputImageIds: task.outputImages.map((image) => image.id),
    maskImageId: task.mask ? `${task.id}:mask` : undefined,
    maskTargetImageId: task.mask?.targetImageId,
    maskWidth: task.mask?.width,
    maskHeight: task.mask?.height,
    maskOriginalWidth: task.mask?.originalWidth,
    maskOriginalHeight: task.mask?.originalHeight,
    maskWasResized: task.mask?.wasResized,
  }
}

function imageToStored(
  image: PlaygroundImage,
  createdAt: number,
  role: StoredImage['role'] = image.role
): StoredImage {
  return {
    id: image.id,
    blob: image.blob,
    sourceUrl: image.sourceUrl,
    mimeType: image.mimeType,
    role,
    createdAt,
    width: image.width,
    height: image.height,
  }
}

function outputToDisplay(image: NormalizedImage): PlaygroundImage {
  return {
    id: image.id,
    src: image.src,
    blob: image.blob,
    sourceUrl: image.sourceUrl,
    mimeType: image.mimeType,
    role: 'output',
  }
}

export function useImagePlayground() {
  const { t } = useTranslation()
  const [prompt, setPrompt] = useState('')
  const [params, setParams] = useState<ImageRequestParams>(DEFAULT_IMAGE_PARAMS)
  const [inputImages, setInputImages] = useState<PlaygroundImage[]>([])
  const [mask, setMask] = useState<PlaygroundMask | undefined>()
  const [tasks, setTasks] = useState<PlaygroundTask[]>([])
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [runningTaskIds, setRunningTaskIds] = useState<Set<string>>(new Set())
  const [isLoadingHistory, setIsLoadingHistory] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [storageWarning, setStorageWarning] = useState<string | null>(null)
  const controllersRef = useRef(new Map<string, AbortController>())
  const objectUrlsRef = useRef(new Set<string>())

  useEffect(() => {
    let cancelled = false
    void listStoredTasks()
      .then(async (stored) => {
        const hydrated = await Promise.all(
          stored.map((task) => hydrateTask(task, objectUrlsRef.current))
        )
        if (!cancelled) setTasks(hydrated)
      })
      .catch(() => {
        if (!cancelled) {
          setStorageWarning(
            t(
              'Browser history is unavailable; results will stay in this session'
            )
          )
        }
      })
      .finally(() => {
        if (!cancelled) setIsLoadingHistory(false)
      })
    return () => {
      cancelled = true
    }
  }, [t])

  useEffect(
    () => () => {
      controllersRef.current.forEach((controller) => controller.abort())
      objectUrlsRef.current.forEach((url) => URL.revokeObjectURL(url))
      objectUrlsRef.current.clear()
    },
    []
  )

  const updateTask = useCallback(
    (id: string, updater: (task: PlaygroundTask) => PlaygroundTask) => {
      setTasks((current) =>
        current.map((task) => (task.id === id ? updater(task) : task))
      )
    },
    []
  )

  const addInputFiles = useCallback((files: File[]) => {
    setInputImages((current) => {
      const accepted = files
        .filter((file) => file.type.startsWith('image/'))
        .slice(
          0,
          Math.max(0, IMAGE_PLAYGROUND_MAX_INPUT_IMAGES - current.length)
        )
      const next = accepted.map<PlaygroundImage>((file) => {
        const src = URL.createObjectURL(file)
        objectUrlsRef.current.add(src)
        return {
          id: createId(),
          src,
          blob: file,
          mimeType: file.type || 'image/png',
          role: 'input',
        }
      })
      if (next.length) setError(null)
      return [...current, ...next]
    })
  }, [])

  const removeInputImage = useCallback((id: string) => {
    setInputImages((current) => {
      const image = current.find((item) => item.id === id)
      if (image) releaseImage(image, objectUrlsRef.current)
      return current.filter((item) => item.id !== id)
    })
    setMask((current) => (current?.targetImageId === id ? undefined : current))
  }, [])

  const clearInputImages = useCallback(() => {
    setInputImages((current) => {
      current.forEach((image) => releaseImage(image, objectUrlsRef.current))
      return []
    })
    setMask((current) => {
      if (current) {
        releaseImage(
          {
            id: `${current.targetImageId}:mask`,
            src: current.src,
            blob: current.blob,
            mimeType: 'image/png',
            role: 'input',
          },
          objectUrlsRef.current
        )
      }
      return undefined
    })
  }, [])

  const updateParam = useCallback(
    <K extends keyof ImageRequestParams>(
      key: K,
      value: ImageRequestParams[K]
    ) => {
      setParams((current) => ({ ...current, [key]: value }))
    },
    []
  )

  const persistTask = useCallback(
    async (task: PlaygroundTask, createdAt: number) => {
      const images: StoredImage[] = [
        ...task.inputImages.map((image) => imageToStored(image, createdAt)),
        ...task.outputImages.map((image) => imageToStored(image, createdAt)),
      ]
      if (task.mask) {
        images.push(
          imageToStored(
            {
              id: `${task.id}:mask`,
              src: task.mask.src,
              blob: task.mask.blob,
              mimeType: 'image/png',
              role: 'input',
              width: task.mask.width,
              height: task.mask.height,
            },
            createdAt,
            'mask'
          )
        )
      }
      await saveTaskBundle(taskToStored(task), images)
    },
    []
  )

  const submit = useCallback(
    async (tokenId: number | null, tokenName: string) => {
      const text = prompt.trim()
      if (!text) {
        setError(t('Enter a prompt before generating'))
        return null
      }
      if (tokenId == null) {
        setError(t('Select an API key before generating'))
        return null
      }

      const startedAt = Date.now()
      const operation: PlaygroundOperation = inputImages.length
        ? 'edit'
        : 'generation'
      const task: PlaygroundTask = {
        id: createId(),
        operation,
        status: 'running',
        prompt: text,
        model: 'gpt-image-2',
        params: { ...params },
        tokenId,
        tokenName,
        createdAt: startedAt,
        startedAt,
        completedAt: null,
        elapsed: null,
        error: null,
        inputImages: inputImages.map((image) =>
          cloneImage(image, objectUrlsRef.current)
        ),
        mask: mask ? cloneMask(mask, objectUrlsRef.current) : undefined,
        outputImages: [],
      }
      setTasks((current) => [task, ...current])
      setRunningTaskIds((current) => new Set(current).add(task.id))
      setPrompt('')
      setInputImages((current) => {
        current.forEach((image) => releaseImage(image, objectUrlsRef.current))
        return []
      })
      setMask((current) => {
        if (current) {
          releaseImage(
            {
              id: `${current.targetImageId}:mask`,
              src: current.src,
              blob: current.blob,
              mimeType: 'image/png',
              role: 'input',
            },
            objectUrlsRef.current
          )
        }
        return undefined
      })
      setError(null)

      const controller = new AbortController()
      controllersRef.current.set(task.id, controller)
      try {
        const apiKey = await resolvePlaygroundToken(tokenId)
        const result =
          operation === 'edit'
            ? await callImageEdit({
                apiKey,
                prompt: text,
                params: task.params,
                images: task.inputImages.flatMap((image) =>
                  image.blob ? [image.blob] : []
                ),
                mask: task.mask?.blob,
                signal: controller.signal,
              })
            : await callImageGeneration({
                apiKey,
                prompt: text,
                params: task.params,
                signal: controller.signal,
              })
        const completedAt = Date.now()
        const done: PlaygroundTask = {
          ...task,
          status: 'done',
          completedAt,
          elapsed: completedAt - startedAt,
          outputImages: result.images.map(outputToDisplay),
        }
        try {
          await persistTask(done, startedAt)
        } catch {
          setStorageWarning(t('Could not save this result to browser history'))
        }
        updateTask(task.id, () => done)
        return done
      } catch (requestError) {
        const completedAt = Date.now()
        const message = isAbortError(requestError)
          ? t('Generation stopped')
          : localizedError(requestError, t)
        const failed: PlaygroundTask = {
          ...task,
          status: 'error',
          completedAt,
          elapsed: completedAt - startedAt,
          error: message,
        }
        try {
          await persistTask(failed, startedAt)
        } catch {
          setStorageWarning(t('Could not save this result to browser history'))
        }
        updateTask(task.id, () => failed)
        if (!isAbortError(requestError)) setError(message)
        return null
      } finally {
        controllersRef.current.delete(task.id)
        setRunningTaskIds((current) => {
          const next = new Set(current)
          next.delete(task.id)
          return next
        })
      }
    },
    [inputImages, mask, params, persistTask, prompt, t, updateTask]
  )

  const cancelTask = useCallback(
    (id: string) => controllersRef.current.get(id)?.abort(),
    []
  )

  const reuseTask = useCallback((task: PlaygroundTask) => {
    setPrompt(task.prompt)
    setParams({ ...task.params })
    setInputImages(
      task.inputImages.map((image) => cloneImage(image, objectUrlsRef.current))
    )
    setMask(task.mask ? cloneMask(task.mask, objectUrlsRef.current) : undefined)
    setError(null)
  }, [])

  const editTask = useCallback(
    (task: PlaygroundTask) => {
      const image = task.outputImages[0]
      if (!image?.blob) {
        setError(t('This image is no longer available for editing'))
        return
      }
      const src = URL.createObjectURL(image.blob)
      objectUrlsRef.current.add(src)
      setPrompt(task.prompt)
      setParams({ ...task.params })
      setInputImages([
        {
          id: createId(),
          src,
          blob: image.blob,
          mimeType: image.mimeType,
          role: 'input',
        },
      ])
      setMask(undefined)
      setError(null)
    },
    [t]
  )

  const retryTask = useCallback(
    (task: PlaygroundTask) => reuseTask(task),
    [reuseTask]
  )

  const deleteTask = useCallback(
    async (id: string) => {
      const task = tasks.find((item) => item.id === id)
      if (!task) return
      try {
        await deleteTaskBundle(taskToStored(task))
      } catch {
        setStorageWarning(t('Could not delete the selected history item'))
      }
      task.inputImages.forEach((image) =>
        releaseImage(image, objectUrlsRef.current)
      )
      task.outputImages.forEach((image) =>
        releaseImage(image, objectUrlsRef.current)
      )
      if (task.mask) {
        releaseImage(
          {
            id: `${task.id}:mask`,
            src: task.mask.src,
            blob: task.mask.blob,
            mimeType: 'image/png',
            role: 'input',
          },
          objectUrlsRef.current
        )
      }
      setTasks((current) => current.filter((item) => item.id !== id))
      setSelectedTaskId((current) => (current === id ? null : current))
    },
    [t, tasks]
  )

  return {
    prompt,
    setPrompt,
    params,
    updateParam,
    inputImages,
    mask,
    setMask,
    addInputFiles,
    removeInputImage,
    clearInputImages,
    tasks,
    selectedTaskId,
    setSelectedTaskId,
    isLoadingHistory,
    isSubmitting: runningTaskIds.size > 0,
    error,
    storageWarning,
    submit,
    cancelTask,
    reuseTask,
    editTask,
    retryTask,
    deleteTask,
  }
}

export type ImagePlaygroundState = ReturnType<typeof useImagePlayground>
