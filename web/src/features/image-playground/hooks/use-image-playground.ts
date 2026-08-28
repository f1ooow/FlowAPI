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
  PlaygroundMode,
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

function getPlaygroundErrorMessage(
  error: unknown,
  t: ReturnType<typeof useTranslation>['t']
): string {
  const message = error instanceof Error ? error.message : ''
  if (message === 'Enter a prompt before generating') {
    return t('Enter a prompt before generating')
  }
  if (message === 'Select an API key before generating') {
    return t('Select an API key before generating')
  }
  if (message === 'Add an image before editing') {
    return t('Add an image before editing')
  }
  if (message === 'This image is no longer available for editing') {
    return t('This image is no longer available for editing')
  }
  if (message === 'Unable to load the selected API key') {
    return t('Unable to load the selected API key')
  }
  if (message === 'The image service returned no images') {
    return t('The image service returned no images')
  }
  if (message.startsWith('Image request failed (')) {
    return t('Image request failed')
  }
  return message || t('Image request failed')
}

function toDisplayImage(
  stored: StoredImage,
  objectUrls: Set<string>
): PlaygroundImage {
  if (stored.blob) {
    const src = URL.createObjectURL(stored.blob)
    objectUrls.add(src)
    return { ...stored, src }
  }
  return {
    id: stored.id,
    src: stored.sourceUrl || '',
    sourceUrl: stored.sourceUrl,
    mimeType: stored.mimeType,
    role: stored.role,
  }
}

async function hydrateTask(
  storedTask: StoredTask,
  objectUrls: Set<string>
): Promise<PlaygroundTask> {
  const [inputImages, outputImages] = await Promise.all([
    Promise.all(storedTask.inputImageIds.map(getStoredImage)),
    Promise.all(storedTask.outputImageIds.map(getStoredImage)),
  ])

  return {
    ...storedTask,
    inputImages: inputImages
      .filter((image): image is StoredImage => Boolean(image))
      .map((image) => toDisplayImage(image, objectUrls)),
    outputImages: outputImages
      .filter((image): image is StoredImage => Boolean(image))
      .map((image) => toDisplayImage(image, objectUrls)),
  }
}

function toStoredImage(image: PlaygroundImage, createdAt: number): StoredImage {
  return {
    id: image.id,
    blob: image.blob,
    sourceUrl: image.sourceUrl,
    mimeType: image.mimeType,
    role: image.role,
    createdAt,
  }
}

function toDisplayOutput(image: NormalizedImage): PlaygroundImage {
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
  const [mode, setMode] = useState<PlaygroundMode>('generate')
  const [prompt, setPrompt] = useState('')
  const [params, setParams] = useState<ImageRequestParams>(DEFAULT_IMAGE_PARAMS)
  const [inputImages, setInputImages] = useState<PlaygroundImage[]>([])
  const [tasks, setTasks] = useState<PlaygroundTask[]>([])
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null)
  const [isLoadingHistory, setIsLoadingHistory] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [storageWarning, setStorageWarning] = useState<string | null>(null)
  const controllerRef = useRef<AbortController | null>(null)
  const objectUrlsRef = useRef<Set<string>>(new Set())

  useEffect(() => {
    let cancelled = false
    void listStoredTasks()
      .then(async (storedTasks) => {
        const hydrated = await Promise.all(
          storedTasks.map((task) => hydrateTask(task, objectUrlsRef.current))
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

  useEffect(() => {
    const objectUrls = objectUrlsRef.current
    return () => {
      controllerRef.current?.abort()
      objectUrls.forEach((url) => URL.revokeObjectURL(url))
      objectUrls.clear()
    }
  }, [])

  const revokeImageUrl = useCallback((image: PlaygroundImage) => {
    if (!image.src.startsWith('blob:')) return
    URL.revokeObjectURL(image.src)
    objectUrlsRef.current.delete(image.src)
  }, [])

  const addInputFiles = useCallback(
    (files: File[]) => {
      const nextFiles = files
        .filter((file) => file.type.startsWith('image/'))
        .slice(
          0,
          Math.max(0, IMAGE_PLAYGROUND_MAX_INPUT_IMAGES - inputImages.length)
        )
      if (!nextFiles.length) return

      const nextImages = nextFiles.map<PlaygroundImage>((file) => {
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
      setInputImages((current) => [...current, ...nextImages])
      setError(null)
    },
    [inputImages.length]
  )

  const removeInputImage = useCallback(
    (imageId: string) => {
      setInputImages((current) => {
        const removed = current.find((image) => image.id === imageId)
        if (removed) revokeImageUrl(removed)
        return current.filter((image) => image.id !== imageId)
      })
    },
    [revokeImageUrl]
  )

  const clearInputImages = useCallback(() => {
    inputImages.forEach(revokeImageUrl)
    setInputImages([])
  }, [inputImages, revokeImageUrl])

  const updateParam = useCallback(
    <K extends keyof ImageRequestParams>(
      key: K,
      value: ImageRequestParams[K]
    ) => {
      setParams((current) => ({ ...current, [key]: value }))
    },
    []
  )

  const submit = useCallback(
    async (tokenId: number | null, tokenName: string) => {
      const trimmedPrompt = prompt.trim()
      if (!trimmedPrompt) {
        setError(t('Enter a prompt before generating'))
        return null
      }
      if (tokenId == null) {
        setError(t('Select an API key before generating'))
        return null
      }
      if (mode === 'edit' && !inputImages.length) {
        setError(t('Add an image before editing'))
        return null
      }

      const controller = new AbortController()
      controllerRef.current = controller
      setIsSubmitting(true)
      setError(null)

      try {
        const apiKey = await resolvePlaygroundToken(tokenId)
        const result =
          mode === 'edit'
            ? await callImageEdit({
                apiKey,
                prompt: trimmedPrompt,
                params,
                images: inputImages.flatMap((image) =>
                  image.blob ? [image.blob] : []
                ),
                signal: controller.signal,
              })
            : await callImageGeneration({
                apiKey,
                prompt: trimmedPrompt,
                params,
                signal: controller.signal,
              })

        const createdAt = Date.now()
        const outputImages = result.images.map(toDisplayOutput)
        const task: PlaygroundTask = {
          id: createId(),
          mode,
          prompt: trimmedPrompt,
          model: 'gpt-image-2',
          params: { ...params },
          tokenId,
          tokenName,
          createdAt,
          completedAt: Date.now(),
          inputImages: inputImages.map((image) => ({ ...image })),
          outputImages,
        }
        const storedTask: StoredTask = {
          id: task.id,
          mode: task.mode,
          prompt: task.prompt,
          model: task.model,
          params: task.params,
          tokenId: task.tokenId,
          tokenName: task.tokenName,
          createdAt: task.createdAt,
          completedAt: task.completedAt,
          inputImageIds: task.inputImages.map((image) => image.id),
          outputImageIds: task.outputImages.map((image) => image.id),
        }
        const storedImages = [
          ...task.inputImages.map((image) => toStoredImage(image, createdAt)),
          ...task.outputImages.map((image) => toStoredImage(image, createdAt)),
        ]

        try {
          await saveTaskBundle(storedTask, storedImages)
          setStorageWarning(null)
        } catch {
          setStorageWarning(t('Could not save this result to browser history'))
        }
        setTasks((current) => [task, ...current])
        setSelectedTaskId(task.id)
        return task
      } catch (requestError) {
        if (!isAbortError(requestError)) {
          setError(getPlaygroundErrorMessage(requestError, t))
        }
        return null
      } finally {
        if (controllerRef.current === controller) controllerRef.current = null
        setIsSubmitting(false)
      }
    },
    [inputImages, mode, params, prompt, t]
  )

  const cancel = useCallback(() => controllerRef.current?.abort(), [])

  const deleteTask = useCallback(
    async (taskId: string) => {
      const task = tasks.find((item) => item.id === taskId)
      if (!task) return
      const storedTask: StoredTask = {
        id: task.id,
        mode: task.mode,
        prompt: task.prompt,
        model: task.model,
        params: task.params,
        tokenId: task.tokenId,
        tokenName: task.tokenName,
        createdAt: task.createdAt,
        completedAt: task.completedAt,
        inputImageIds: task.inputImages.map((image) => image.id),
        outputImageIds: task.outputImages.map((image) => image.id),
      }
      try {
        await deleteTaskBundle(storedTask)
      } catch {
        setStorageWarning(t('Could not delete the selected history item'))
      }
      ;[...task.inputImages, ...task.outputImages].forEach(revokeImageUrl)
      setTasks((current) => current.filter((item) => item.id !== taskId))
      setSelectedTaskId((current) => (current === taskId ? null : current))
    },
    [revokeImageUrl, t, tasks]
  )

  const editTask = useCallback(
    async (task: PlaygroundTask) => {
      const source = task.outputImages[0]
      if (!source) return
      let blob = source.blob
      if (!blob && source.sourceUrl) {
        try {
          const response = await fetch(source.sourceUrl)
          if (response.ok) blob = await response.blob()
        } catch {
          // Keep the original task intact when an expired URL cannot be reused.
        }
      }
      if (!blob) {
        setError(t('This image is no longer available for editing'))
        return
      }
      const src = URL.createObjectURL(blob)
      objectUrlsRef.current.add(src)
      setMode('edit')
      setPrompt(task.prompt)
      setParams({ ...task.params })
      inputImages.forEach(revokeImageUrl)
      setInputImages([
        {
          id: createId(),
          src,
          blob,
          mimeType: blob.type || 'image/png',
          role: 'input',
        },
      ])
      setError(null)
    },
    [inputImages, revokeImageUrl, t]
  )

  return {
    mode,
    setMode,
    prompt,
    setPrompt,
    params,
    updateParam,
    inputImages,
    addInputFiles,
    removeInputImage,
    clearInputImages,
    tasks,
    selectedTaskId,
    setSelectedTaskId,
    isLoadingHistory,
    isSubmitting,
    error,
    storageWarning,
    submit,
    cancel,
    deleteTask,
    editTask,
  }
}

export type ImagePlaygroundState = ReturnType<typeof useImagePlayground>
