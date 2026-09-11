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
  Cancel01Icon,
  Copy01Icon,
  Download01Icon,
  Edit02Icon,
  Image01Icon,
  InformationCircleIcon,
  Upload04Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
  type MouseEvent,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import {
  IMAGE_FORMAT_OPTIONS,
  IMAGE_QUALITY_OPTIONS,
  IMAGE_SIZE_OPTIONS,
} from '../constants'
import { useImagePlayground } from '../hooks/use-image-playground'
import { copyImageToClipboard, downloadImage } from '../lib/image-utils'
import {
  createMaskPreviewDataUrl,
  type MaskSaveResult,
  type PreparedMaskTarget,
} from '../lib/mask-preprocess'
import { getPlaygroundTokens } from '../lib/tokens'
import type {
  PlaygroundImage,
  PlaygroundMask,
  PlaygroundTask,
  PlaygroundToken,
} from '../types'
import { MaskEditor } from './mask-editor'
import { TaskCard } from './task-card'
import { TaskDetail } from './task-detail'

interface ContextMenuState {
  image: PlaygroundImage
  task?: PlaygroundTask
  x: number
  y: number
}

function formatTaskTime(createdAt: number): string {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(createdAt)
}

function ImageContextMenu(props: {
  state: ContextMenuState
  onClose: () => void
  onCopy: () => void
  onDownload: () => void
  onEdit: () => void
}) {
  const { t } = useTranslation()
  useEffect(() => {
    const close = () => props.onClose()
    const keydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') props.onClose()
    }
    window.addEventListener('click', close)
    window.addEventListener('scroll', close, true)
    window.addEventListener('keydown', keydown)
    return () => {
      window.removeEventListener('click', close)
      window.removeEventListener('scroll', close, true)
      window.removeEventListener('keydown', keydown)
    }
  }, [props])
  return (
    <div
      className='fixed z-[90] w-44 rounded-xl border border-slate-200 bg-white p-1.5 shadow-xl'
      style={{
        left: Math.min(props.state.x, window.innerWidth - 190),
        top: Math.min(props.state.y, window.innerHeight - 170),
      }}
      onClick={(event) => event.stopPropagation()}
      role='menu'
    >
      <button
        type='button'
        onClick={props.onCopy}
        className='flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm text-slate-700 hover:bg-slate-50'
        role='menuitem'
      >
        <HugeiconsIcon icon={Copy01Icon} size={17} />
        {t('Copy')}
      </button>
      <button
        type='button'
        onClick={props.onDownload}
        className='flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm text-slate-700 hover:bg-slate-50'
        role='menuitem'
      >
        <HugeiconsIcon icon={Download01Icon} size={17} />
        {t('Download')}
      </button>
      {props.state.task ? (
        <button
          type='button'
          onClick={props.onEdit}
          className='flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm text-slate-700 hover:bg-slate-50'
          role='menuitem'
        >
          <HugeiconsIcon icon={Edit02Icon} size={17} />
          {t('Edit output')}
        </button>
      ) : null}
    </div>
  )
}

function ReferenceThumb(props: {
  image: PlaygroundImage
  mask?: PlaygroundMask
  onRemove: () => void
  onPreview: () => void
  onEdit: () => void
  onContextMenu: (event: MouseEvent) => void
}) {
  const { t } = useTranslation()
  const isMaskTarget = props.mask?.targetImageId === props.image.id
  const [maskPreviewSrc, setMaskPreviewSrc] = useState<string | null>(null)

  useEffect(() => {
    if (!isMaskTarget || !props.mask) {
      setMaskPreviewSrc(null)
      return
    }

    let cancelled = false
    setMaskPreviewSrc(null)
    void createMaskPreviewDataUrl(props.image.src, props.mask.src)
      .then((previewSrc) => {
        if (!cancelled) setMaskPreviewSrc(previewSrc)
      })
      .catch(() => {
        if (!cancelled) setMaskPreviewSrc(null)
      })

    return () => {
      cancelled = true
    }
  }, [isMaskTarget, props.image.src, props.mask])

  const displaySrc = maskPreviewSrc || props.image.src
  return (
    <div
      className={`group relative size-16 shrink-0 overflow-hidden rounded-lg border bg-slate-50 ${isMaskTarget ? 'border-blue-400 ring-2 ring-blue-100' : 'border-slate-200'}`}
    >
      <button
        type='button'
        onClick={props.onPreview}
        onContextMenu={props.onContextMenu}
        className='block h-full w-full'
        aria-label={t('Preview reference image')}
      >
        <img src={displaySrc} alt='' className='h-full w-full object-cover' />
        {isMaskTarget ? (
          <span
            className='absolute top-1 right-1 size-2 rounded-full bg-blue-500 ring-2 ring-white'
            aria-label={t('Mask ready')}
          />
        ) : null}
      </button>
      <div className='pointer-events-none absolute inset-x-0 bottom-0 flex justify-center gap-1 bg-slate-950/60 p-1 opacity-0 transition-opacity group-hover:pointer-events-auto group-hover:opacity-100'>
        <button
          type='button'
          onClick={props.onEdit}
          className='inline-flex size-5 items-center justify-center rounded text-white hover:bg-white/20'
          aria-label={t('Edit image')}
        >
          <HugeiconsIcon icon={Edit02Icon} size={12} />
        </button>
        <button
          type='button'
          onClick={props.onRemove}
          className='inline-flex size-5 items-center justify-center rounded text-white hover:bg-white/20'
          aria-label={t('Remove reference image')}
        >
          <HugeiconsIcon icon={Cancel01Icon} size={12} />
        </button>
      </div>
    </div>
  )
}

function ParameterSelect(props: {
  label: string
  value: string
  options: Array<{ value: string; labelKey: string }>
  onChange: (value: string) => void
}) {
  const { t } = useTranslation()
  return (
    <label className='min-w-0 flex-1'>
      <span className='mb-1.5 block text-[11px] font-medium text-slate-400'>
        {props.label}
      </span>
      <select
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
        className='h-9 w-full rounded-lg border border-slate-200 bg-white px-2.5 text-xs text-slate-700 transition-colors outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-100'
      >
        {props.options.map((option) => (
          <option key={option.value} value={option.value}>
            {t(option.labelKey)}
          </option>
        ))}
      </select>
    </label>
  )
}

export function ImagePlayground() {
  const { t } = useTranslation()
  const state = useImagePlayground()
  const [tokens, setTokens] = useState<PlaygroundToken[]>([])
  const [tokenId, setTokenId] = useState<number | null>(null)
  const [tokenError, setTokenError] = useState<string | null>(null)
  const [dragActive, setDragActive] = useState(false)
  const [previewImage, setPreviewImage] = useState<PlaygroundImage | null>(null)
  const [maskImage, setMaskImage] = useState<PlaygroundImage | null>(null)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const selectedTask = state.selectedTaskId
    ? state.tasks.find((task) => task.id === state.selectedTaskId)
    : undefined
  const selectedToken = tokens.find((token) => token.id === tokenId)
  let galleryContent
  if (state.isLoadingHistory) {
    galleryContent = (
      <div className='grid gap-4 xl:grid-cols-2'>
        <div className='h-56 animate-pulse rounded-xl bg-slate-200' />
        <div className='h-56 animate-pulse rounded-xl bg-slate-200' />
      </div>
    )
  } else if (state.tasks.length) {
    galleryContent = (
      <div className='grid gap-4 xl:grid-cols-2'>
        {state.tasks.map((task) => (
          <div key={task.id} className='relative'>
            <TaskCard
              task={task}
              onOpen={(item) => state.setSelectedTaskId(item.id)}
              onReuse={state.reuseTask}
              onEdit={state.editTask}
              onRetry={state.retryTask}
              onCancel={(item) => state.cancelTask(item.id)}
              onDelete={(item) => void state.deleteTask(item.id)}
              onImageContextMenu={(event, image, item) =>
                openContextMenu(event, image, item)
              }
            />
            <span className='pointer-events-none absolute top-3 right-4 text-[10px] text-slate-300'>
              {formatTaskTime(task.createdAt)}
            </span>
          </div>
        ))}
      </div>
    )
  } else {
    galleryContent = (
      <div className='flex min-h-[420px] flex-col items-center justify-center rounded-xl border border-dashed border-slate-200 bg-white text-center'>
        <div className='mb-3 flex size-12 items-center justify-center rounded-xl bg-slate-50 text-slate-400'>
          <HugeiconsIcon icon={Image01Icon} size={24} />
        </div>
        <h3 className='text-sm font-medium text-slate-700'>
          {t('Your gallery is empty')}
        </h3>
        <p className='mt-1 max-w-xs text-xs leading-5 text-slate-400'>
          {t('Generated images will stay here for reuse and editing')}
        </p>
      </div>
    )
  }

  useEffect(() => {
    let cancelled = false
    void getPlaygroundTokens()
      .then((items) => {
        if (cancelled) return
        setTokens(items)
        setTokenId((current) => current ?? items[0]?.id ?? null)
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setTokenError(
            error instanceof Error
              ? error.message
              : t('Unable to load API keys')
          )
        }
      })
    return () => {
      cancelled = true
    }
  }, [t])

  useEffect(() => {
    if (!contextMenu) return
    const close = () => setContextMenu(null)
    window.addEventListener('resize', close)
    return () => window.removeEventListener('resize', close)
  }, [contextMenu])

  const addFiles = (files: File[]) => state.addInputFiles(files)
  const onFileChange = (event: ChangeEvent<HTMLInputElement>) => {
    if (event.target.files) addFiles([...event.target.files])
    event.target.value = ''
  }
  const onDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    setDragActive(false)
    addFiles([...event.dataTransfer.files])
  }
  const onPaste = (event: React.ClipboardEvent) => {
    const files = [...event.clipboardData.items]
      .map((item) => item.getAsFile())
      .filter((file): file is File =>
        Boolean(file && file.type.startsWith('image/'))
      )
    if (files.length) {
      event.preventDefault()
      addFiles(files)
    }
  }
  const openContextMenu = (
    event: MouseEvent,
    image: PlaygroundImage,
    task?: PlaygroundTask
  ) => {
    event.preventDefault()
    setContextMenu({ image, task, x: event.clientX, y: event.clientY })
  }
  const copyImage = async (image: PlaygroundImage) => {
    setContextMenu(null)
    try {
      await copyImageToClipboard(image)
    } catch {
      /* Clipboard support is browser-dependent. */
    }
  }
  const beginMaskEdit = async (image: PlaygroundImage) => {
    if (!image.blob && image.sourceUrl) {
      try {
        const response = await fetch(image.sourceUrl)
        if (response.ok) image = { ...image, blob: await response.blob() }
      } catch {
        /* The editor will show a preparation error. */
      }
    }
    setMaskImage(image)
  }
  const saveMask = (target: PreparedMaskTarget, result: MaskSaveResult) => {
    if (!maskImage) return
    const currentImage = state.inputImages.find(
      (image) => image.id === maskImage.id
    )
    if (!currentImage) {
      setMaskImage(null)
      return
    }
    state.replaceInputImage(maskImage.id, {
      src: target.dataUrl,
      blob: target.blob,
      mimeType: 'image/png',
      sourceUrl: undefined,
      width: target.width,
      height: target.height,
    })
    state.setMask({
      targetImageId: maskImage.id,
      blob: result.maskBlob,
      src: result.maskDataUrl,
      width: result.width,
      height: result.height,
      originalWidth: target.originalWidth,
      originalHeight: target.originalHeight,
      wasResized: target.wasResized,
    })
    setMaskImage(null)
  }
  const downloadOutput = (
    image: PlaygroundImage,
    task: PlaygroundTask,
    index: number
  ) => {
    downloadImage(
      image,
      `flow-api-${task.id}-${index + 1}.${task.params.output_format === 'jpeg' ? 'jpg' : task.params.output_format}`
    )
    setContextMenu(null)
  }
  const submit = () => {
    void state.submit(tokenId, selectedToken?.name || '')
  }

  return (
    <div className='flex h-full min-h-0 flex-col bg-[#f8fafc] text-slate-900'>
      <header className='flex h-14 shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4 sm:px-6'>
        <div className='flex items-center gap-3'>
          <div className='flex size-8 items-center justify-center rounded-lg bg-blue-50 text-blue-600'>
            <HugeiconsIcon icon={Image01Icon} size={18} />
          </div>
          <div>
            <h1 className='text-sm font-semibold text-slate-900'>
              {t('生图工作台')}
            </h1>
            <p className='hidden text-[11px] text-slate-400 sm:block'>
              {t('Create and refine images with GPT Image')}
            </p>
          </div>
        </div>
        <div className='flex items-center gap-2 text-xs text-slate-400'>
          <HugeiconsIcon icon={InformationCircleIcon} size={16} />
          {t('Only image generation and editing')}
        </div>
      </header>
      <div className='grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[360px_minmax(0,1fr)]'>
        <aside className='min-h-0 overflow-y-auto border-b border-slate-200 bg-white p-4 sm:p-5 lg:border-r lg:border-b-0'>
          <div className='mb-5 flex items-center justify-between'>
            <div>
              <h2 className='text-sm font-semibold text-slate-900'>
                {t('Create image')}
              </h2>
              <p className='mt-1 text-xs text-slate-400'>
                {t('Describe the image you want')}
              </p>
            </div>
            <span className='rounded-md bg-blue-50 px-2 py-1 text-[11px] font-medium text-blue-700'>
              GPT Image
            </span>
          </div>
          <div
            className={`rounded-xl border ${dragActive ? 'border-blue-400 bg-blue-50/40' : 'border-slate-200 bg-white'} transition-colors`}
            onDragOver={(event) => {
              event.preventDefault()
              setDragActive(true)
            }}
            onDragLeave={() => setDragActive(false)}
            onDrop={onDrop}
            onPaste={onPaste}
          >
            <label htmlFor='image-playground-prompt' className='sr-only'>
              {t('Prompt')}
            </label>
            <textarea
              id='image-playground-prompt'
              value={state.prompt}
              onChange={(event) => state.setPrompt(event.target.value)}
              placeholder={t('Describe your image...')}
              className='min-h-36 w-full resize-none rounded-xl border-0 bg-transparent p-4 text-sm leading-6 text-slate-800 outline-none placeholder:text-slate-400'
            />
            {state.inputImages.length ? (
              <div className='flex gap-2 overflow-x-auto border-t border-slate-100 px-3 py-3'>
                {state.inputImages.map((image) => (
                  <ReferenceThumb
                    key={image.id}
                    image={image}
                    mask={state.mask}
                    onRemove={() => state.removeInputImage(image.id)}
                    onPreview={() => setPreviewImage(image)}
                    onEdit={() => void beginMaskEdit(image)}
                    onContextMenu={(event) => openContextMenu(event, image)}
                  />
                ))}
              </div>
            ) : null}
            <div className='flex items-center justify-between border-t border-slate-100 px-3 py-2.5'>
              <div className='flex items-center gap-1'>
                <button
                  type='button'
                  onClick={() => fileInputRef.current?.click()}
                  className='inline-flex size-8 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100'
                  aria-label={t('Upload reference image')}
                >
                  <HugeiconsIcon icon={Upload04Icon} size={18} />
                </button>
                {state.inputImages.length ? (
                  <span className='text-[11px] text-slate-400'>
                    {state.inputImages.length} {t('reference images')}
                  </span>
                ) : (
                  <span className='text-[11px] text-slate-400'>
                    {t('Paste or drop images here')}
                  </span>
                )}
              </div>
              <button
                type='button'
                onClick={submit}
                disabled={
                  state.isSubmitting || !state.prompt.trim() || tokenId == null
                }
                className='inline-flex h-9 items-center gap-2 rounded-lg bg-blue-600 px-4 text-sm font-medium text-white shadow-sm hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50'
              >
                {state.isSubmitting ? t('Generating') : t('Generate image')}
                <span aria-hidden='true'>→</span>
              </button>
            </div>
          </div>
          <input
            ref={fileInputRef}
            type='file'
            accept='image/*'
            multiple
            className='hidden'
            onChange={onFileChange}
          />
          <div className='mt-5 space-y-4'>
            <div>
              <label
                htmlFor='image-playground-token'
                className='mb-1.5 block text-[11px] font-medium text-slate-400'
              >
                {t('API key')}
              </label>
              <select
                id='image-playground-token'
                value={tokenId ?? ''}
                onChange={(event) =>
                  setTokenId(
                    event.target.value ? Number(event.target.value) : null
                  )
                }
                className='h-10 w-full rounded-lg border border-slate-200 bg-white px-3 text-sm text-slate-700 outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-100'
              >
                <option value=''>
                  {tokens.length
                    ? t('Select an API key')
                    : t('Loading API keys...')}
                </option>
                {tokens.map((token) => (
                  <option key={token.id} value={token.id}>
                    {token.name} · {token.maskedKey}
                  </option>
                ))}
              </select>
              {tokenError ? (
                <p className='mt-1 text-xs text-red-600'>{tokenError}</p>
              ) : null}
            </div>
            <div className='flex gap-2'>
              <ParameterSelect
                label={t('Size')}
                value={state.params.size}
                options={IMAGE_SIZE_OPTIONS}
                onChange={(value) =>
                  state.updateParam('size', value as typeof state.params.size)
                }
              />
              <ParameterSelect
                label={t('Quality')}
                value={state.params.quality}
                options={IMAGE_QUALITY_OPTIONS}
                onChange={(value) =>
                  state.updateParam(
                    'quality',
                    value as typeof state.params.quality
                  )
                }
              />
            </div>
            <div className='flex gap-2'>
              <ParameterSelect
                label={t('Format')}
                value={state.params.output_format}
                options={IMAGE_FORMAT_OPTIONS}
                onChange={(value) =>
                  state.updateParam(
                    'output_format',
                    value as typeof state.params.output_format
                  )
                }
              />
              <label className='min-w-0 flex-1'>
                <span className='mb-1.5 block text-[11px] font-medium text-slate-400'>
                  {t('Quantity')}
                </span>
                <input
                  type='number'
                  min={1}
                  max={4}
                  value={state.params.n}
                  onChange={(event) =>
                    state.updateParam(
                      'n',
                      Math.min(4, Math.max(1, Number(event.target.value) || 1))
                    )
                  }
                  className='h-9 w-full rounded-lg border border-slate-200 bg-white px-2.5 text-xs text-slate-700 outline-none focus:border-blue-400 focus:ring-2 focus:ring-blue-100'
                />
              </label>
            </div>
          </div>
          {state.mask ? (
            <div className='mt-5 rounded-lg border border-blue-100 bg-blue-50/50 p-3 text-xs text-blue-700'>
              <div className='flex items-center justify-between'>
                <span>{t('Mask ready')}</span>
                <button
                  type='button'
                  onClick={() => state.setMask(undefined)}
                  className='text-blue-500 hover:text-blue-800'
                >
                  {t('Remove')}
                </button>
              </div>
              <p className='mt-1 text-blue-600/80'>
                {state.mask.width} x {state.mask.height} PNG
              </p>
            </div>
          ) : null}
          {state.error ? (
            <div className='mt-5 rounded-lg border border-red-100 bg-red-50 p-3 text-xs leading-5 text-red-700'>
              {state.error}
            </div>
          ) : null}
          {state.storageWarning ? (
            <div className='mt-5 rounded-lg border border-amber-100 bg-amber-50 p-3 text-xs leading-5 text-amber-700'>
              {state.storageWarning}
            </div>
          ) : null}
        </aside>
        <main className='min-h-0 overflow-y-auto p-4 sm:p-6'>
          <div className='mx-auto max-w-6xl'>
            <div className='mb-4 flex items-center justify-between gap-3'>
              <div>
                <h2 className='text-base font-semibold text-slate-900'>
                  {t('Gallery')}
                </h2>
                <p className='mt-1 text-xs text-slate-400'>
                  {t('Click an image to view details')}
                </p>
              </div>
              <span className='text-xs text-slate-400 tabular-nums'>
                {state.tasks.length} {t('tasks')}
              </span>
            </div>
            {galleryContent}
          </div>
        </main>
      </div>
      {selectedTask ? (
        <TaskDetail
          task={selectedTask}
          onClose={() => state.setSelectedTaskId(null)}
          onReuse={(task) => {
            state.reuseTask(task)
            state.setSelectedTaskId(null)
          }}
          onEdit={(task) => {
            state.editTask(task)
            state.setSelectedTaskId(null)
          }}
          onDelete={(task) => {
            void state.deleteTask(task.id)
          }}
          onDownload={downloadOutput}
          onCopy={(image) => void copyImage(image)}
        />
      ) : null}
      {previewImage ? (
        <div
          className='fixed inset-0 z-40 flex items-center justify-center bg-slate-950/25 p-4 backdrop-blur-sm'
          role='dialog'
          aria-modal='true'
          aria-label={t('Reference image preview')}
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setPreviewImage(null)
          }}
        >
          <div className='relative max-h-[90vh] max-w-3xl rounded-2xl bg-white p-3 shadow-2xl'>
            <img
              src={previewImage.src}
              alt=''
              className='max-h-[78vh] max-w-full rounded-xl object-contain'
            />
            <div className='flex justify-end gap-2 p-2'>
              <Button
                variant='outline'
                onClick={() => {
                  setPreviewImage(null)
                  void beginMaskEdit(previewImage)
                }}
              >
                <HugeiconsIcon icon={Edit02Icon} size={16} />
                {t('Edit image')}
              </Button>
              <Button
                variant='ghost'
                size='icon'
                onClick={() => setPreviewImage(null)}
                aria-label={t('Close')}
              >
                <HugeiconsIcon icon={Cancel01Icon} size={18} />
              </Button>
            </div>
          </div>
        </div>
      ) : null}
      {maskImage ? (
        <MaskEditor
          image={maskImage}
          onClose={() => setMaskImage(null)}
          onSave={saveMask}
        />
      ) : null}
      {contextMenu ? (
        <ImageContextMenu
          state={contextMenu}
          onClose={() => setContextMenu(null)}
          onCopy={() => void copyImage(contextMenu.image)}
          onDownload={() => {
            if (contextMenu.task) {
              downloadOutput(
                contextMenu.image,
                contextMenu.task,
                contextMenu.task.outputImages.findIndex(
                  (image) => image.id === contextMenu.image.id
                )
              )
            } else downloadImage(contextMenu.image, 'flow-api-image.png')
            setContextMenu(null)
          }}
          onEdit={() => {
            if (contextMenu.task) {
              state.editTask(contextMenu.task)
              setContextMenu(null)
            }
          }}
        />
      ) : null}
    </div>
  )
}
