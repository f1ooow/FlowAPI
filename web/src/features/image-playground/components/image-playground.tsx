import {
  Download01Icon,
  Edit02Icon,
  Image01Icon,
  InformationCircleIcon,
  Delete02Icon,
  Cancel01Icon,
  Upload04Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { API_KEY_STATUSES } from '@/features/keys/constants'
import { useIsMobile } from '@/hooks/use-mobile'
import { cn } from '@/lib/utils'

import {
  IMAGE_FORMAT_OPTIONS,
  IMAGE_PLAYGROUND_MAX_INPUT_IMAGES,
  IMAGE_PLAYGROUND_MAX_OUTPUTS,
  IMAGE_QUALITY_OPTIONS,
  IMAGE_SIZE_OPTIONS,
} from '../constants'
import { useImagePlayground } from '../hooks/use-image-playground'
import { getPlaygroundTokens } from '../lib/tokens'
import type {
  ImageRequestParams,
  PlaygroundImage,
  PlaygroundMode,
  PlaygroundTask,
} from '../types'

function formatTaskTime(timestamp: number): string {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(timestamp)
}

function outputExtension(
  format: PlaygroundTask['params']['output_format']
): string {
  return format === 'jpeg' ? 'jpg' : format
}

function downloadImage(image: PlaygroundImage, filename: string): void {
  const anchor = document.createElement('a')
  anchor.href = image.src
  anchor.download = filename
  anchor.target = '_blank'
  anchor.rel = 'noreferrer'
  anchor.click()
}

function downloadTaskImages(task: PlaygroundTask): void {
  const extension = outputExtension(task.params.output_format)
  task.outputImages.forEach((image, index) => {
    downloadImage(image, `flow-api-${task.id}-${index + 1}.${extension}`)
  })
}

function tokenStatusLabel(status: number, t: TFunction): string {
  const config = API_KEY_STATUSES[status]
  return config ? t(config.label) : t('Unknown')
}

function TokenPicker(props: {
  tokens: ReturnType<typeof getPlaygroundTokens> extends Promise<infer T>
    ? T
    : never
  value: number | null
  onChange: (value: number | null) => void
  isLoading: boolean
  isError: boolean
}) {
  const { t } = useTranslation()
  const selected = props.tokens.find((token) => token.id === props.value)
  return (
    <div className='grid gap-1.5'>
      <Label htmlFor='image-playground-token'>{t('API key')}</Label>
      <Select
        value={props.value == null ? '' : String(props.value)}
        onValueChange={(value) => props.onChange(value ? Number(value) : null)}
        disabled={props.isLoading || props.isError || props.tokens.length === 0}
      >
        <SelectTrigger id='image-playground-token' className='w-full'>
          <SelectValue
            placeholder={
              props.isLoading ? t('Loading...') : t('Select an API key')
            }
          >
            {selected ? (
              <span className='flex min-w-0 items-center gap-2'>
                <span className='truncate'>
                  {selected.name || t('Unnamed API key')}
                </span>
                <span className='text-muted-foreground truncate text-xs'>
                  {selected.maskedKey} · {tokenStatusLabel(selected.status, t)}
                </span>
              </span>
            ) : null}
          </SelectValue>
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {props.tokens.map((token) => (
              <SelectItem key={token.id} value={String(token.id)}>
                <span className='flex min-w-0 items-center gap-2'>
                  <span className='truncate'>
                    {token.name || t('Unnamed API key')}
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {token.maskedKey}
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {tokenStatusLabel(token.status, t)}
                  </span>
                </span>
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {props.isError && (
        <p className='text-destructive text-xs'>
          {t('Failed to load API keys')}
        </p>
      )}
      {!props.isLoading && !props.isError && props.tokens.length === 0 && (
        <p className='text-muted-foreground text-xs'>
          {t('Create an API key before using the playground')}
        </p>
      )}
    </div>
  )
}

function ParameterSelect(props: {
  id: string
  label: string
  value: string
  options: Array<{ value: string; labelKey: string }>
  t: TFunction
  onChange: (value: string) => void
}) {
  return (
    <div className='grid min-w-0 gap-1.5'>
      <Label htmlFor={props.id}>{props.label}</Label>
      <Select
        value={props.value}
        onValueChange={(value) => props.onChange(value || props.value)}
      >
        <SelectTrigger id={props.id} className='w-full'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {props.options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {props.t(option.labelKey)}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  )
}

function InputImageList(props: {
  images: PlaygroundImage[]
  onRemove: (id: string) => void
}) {
  const { t } = useTranslation()
  if (!props.images.length) return null
  return (
    <div className='grid grid-cols-4 gap-2'>
      {props.images.map((image, index) => (
        <div
          key={image.id}
          className='group bg-muted relative aspect-square overflow-hidden rounded-md border'
        >
          <img
            src={image.src}
            alt={t('Reference image {{number}}', { number: index + 1 })}
            className='size-full object-cover'
          />
          <Button
            aria-label={t('Remove reference image {{number}}', {
              number: index + 1,
            })}
            className='absolute top-1 right-1 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100'
            onClick={() => props.onRemove(image.id)}
            size='icon-xs'
            variant='secondary'
          >
            <HugeiconsIcon icon={Delete02Icon} aria-hidden='true' />
          </Button>
        </div>
      ))}
    </div>
  )
}

function RequestComposer(props: {
  state: ReturnType<typeof useImagePlayground>
  tokens: Parameters<typeof TokenPicker>[0]['tokens']
  selectedTokenId: number | null
  onTokenChange: (value: number | null) => void
  tokensLoading: boolean
  tokensError: boolean
}) {
  const { t } = useTranslation()
  const inputRef = useRef<HTMLInputElement>(null)
  const token = props.tokens.find((item) => item.id === props.selectedTokenId)
  const canSubmit =
    Boolean(token) &&
    Boolean(props.state.prompt.trim()) &&
    (props.state.mode === 'generate' || props.state.inputImages.length > 0)
  let submitLabel = t('Generate image')
  if (props.state.isSubmitting) submitLabel = t('Generating...')
  else if (props.state.mode === 'edit') submitLabel = t('Edit image')

  const update = <K extends keyof ImageRequestParams>(
    key: K,
    value: ImageRequestParams[K]
  ) => {
    props.state.updateParam(key, value)
  }

  return (
    <div className='bg-card grid content-start gap-4 rounded-lg border p-4 shadow-xs'>
      <div className='flex items-center gap-2'>
        <HugeiconsIcon
          icon={Image01Icon}
          className='text-primary size-5'
          aria-hidden='true'
        />
        <div>
          <h3 className='text-sm font-semibold'>{t('Create an image')}</h3>
          <p className='text-muted-foreground text-xs'>
            {t('Use your API key with GPT Image')}
          </p>
        </div>
      </div>

      <div
        className='bg-muted/30 flex rounded-md border p-1'
        role='tablist'
        aria-label={t('Image mode')}
      >
        {(['generate', 'edit'] as PlaygroundMode[]).map((value) => (
          <button
            key={value}
            type='button'
            role='tab'
            aria-selected={props.state.mode === value}
            className={cn(
              'min-h-8 flex-1 rounded px-3 text-sm transition-colors',
              props.state.mode === value
                ? 'bg-background font-medium shadow-xs'
                : 'text-muted-foreground hover:text-foreground'
            )}
            onClick={() => props.state.setMode(value)}
          >
            {value === 'generate' ? t('Generate') : t('Edit image')}
          </button>
        ))}
      </div>

      <TokenPicker
        tokens={props.tokens}
        value={props.selectedTokenId}
        onChange={props.onTokenChange}
        isLoading={props.tokensLoading}
        isError={props.tokensError}
      />

      <div className='grid gap-1.5'>
        <Label htmlFor='image-playground-prompt'>{t('Prompt')}</Label>
        <Textarea
          id='image-playground-prompt'
          value={props.state.prompt}
          onChange={(event) => props.state.setPrompt(event.target.value)}
          placeholder={
            props.state.mode === 'edit'
              ? t('Describe how to change the image')
              : t('Describe the image you want to create')
          }
          className='min-h-28 resize-y'
          disabled={props.state.isSubmitting}
        />
      </div>

      {props.state.mode === 'edit' && (
        <div className='grid gap-2'>
          <div className='flex items-center justify-between gap-2'>
            <Label>{t('Reference images')}</Label>
            <span className='text-muted-foreground text-xs'>
              {props.state.inputImages.length}/
              {IMAGE_PLAYGROUND_MAX_INPUT_IMAGES}
            </span>
          </div>
          <input
            ref={inputRef}
            type='file'
            accept='image/*'
            multiple
            className='sr-only'
            onChange={(event) => {
              props.state.addInputFiles([...(event.target.files || [])])
              event.target.value = ''
            }}
          />
          <button
            type='button'
            className='text-muted-foreground hover:border-primary hover:text-foreground focus-visible:border-ring focus-visible:ring-ring/50 flex min-h-20 items-center justify-center gap-2 rounded-md border border-dashed px-3 text-sm transition-colors focus-visible:ring-3'
            onClick={() => inputRef.current?.click()}
            disabled={
              props.state.isSubmitting ||
              props.state.inputImages.length >=
                IMAGE_PLAYGROUND_MAX_INPUT_IMAGES
            }
          >
            <HugeiconsIcon icon={Upload04Icon} aria-hidden='true' />
            {t('Upload reference image')}
          </button>
          <InputImageList
            images={props.state.inputImages}
            onRemove={props.state.removeInputImage}
          />
        </div>
      )}

      <div className='grid grid-cols-2 gap-3'>
        <ParameterSelect
          id='image-playground-size'
          label={t('Size')}
          value={props.state.params.size}
          options={IMAGE_SIZE_OPTIONS}
          t={t}
          onChange={(value) =>
            update('size', value as ImageRequestParams['size'])
          }
        />
        <ParameterSelect
          id='image-playground-quality'
          label={t('Quality')}
          value={props.state.params.quality}
          options={IMAGE_QUALITY_OPTIONS}
          t={t}
          onChange={(value) =>
            update('quality', value as ImageRequestParams['quality'])
          }
        />
        <ParameterSelect
          id='image-playground-format'
          label={t('Format')}
          value={props.state.params.output_format}
          options={IMAGE_FORMAT_OPTIONS}
          t={t}
          onChange={(value) =>
            update(
              'output_format',
              value as ImageRequestParams['output_format']
            )
          }
        />
        <div className='grid gap-1.5'>
          <Label htmlFor='image-playground-count'>{t('Images')}</Label>
          <Input
            id='image-playground-count'
            type='number'
            min={1}
            max={IMAGE_PLAYGROUND_MAX_OUTPUTS}
            step={1}
            value={props.state.params.n}
            onChange={(event) =>
              update(
                'n',
                Math.min(
                  IMAGE_PLAYGROUND_MAX_OUTPUTS,
                  Math.max(1, Number(event.target.value) || 1)
                )
              )
            }
            disabled={props.state.isSubmitting}
          />
        </div>
      </div>

      {props.state.error && (
        <p className='text-destructive text-sm' role='alert'>
          {props.state.error}
        </p>
      )}
      {props.state.storageWarning && (
        <p className='text-muted-foreground text-xs' role='status'>
          {props.state.storageWarning}
        </p>
      )}

      <div className='flex items-center justify-end gap-2'>
        {props.state.isSubmitting && (
          <Button type='button' variant='outline' onClick={props.state.cancel}>
            {t('Stop')}
          </Button>
        )}
        <Button
          type='button'
          disabled={!canSubmit || props.state.isSubmitting}
          onClick={() =>
            void props.state.submit(
              props.selectedTokenId,
              token?.name || t('Unnamed API key')
            )
          }
        >
          <HugeiconsIcon icon={Image01Icon} aria-hidden='true' />
          {submitLabel}
        </Button>
      </div>
    </div>
  )
}

function TaskCard(props: {
  task: PlaygroundTask
  onOpen: (task: PlaygroundTask) => void
  onEdit: (task: PlaygroundTask) => void
  onDelete: (task: PlaygroundTask) => void
}) {
  const { t } = useTranslation()
  const image = props.task.outputImages[0]
  return (
    <article className='group bg-card overflow-hidden rounded-lg border shadow-xs'>
      <button
        type='button'
        className='focus-visible:ring-ring/50 block w-full text-left focus-visible:ring-3'
        onClick={() => props.onOpen(props.task)}
      >
        <div className='bg-muted relative aspect-square'>
          {image ? (
            <img
              src={image.src}
              alt={props.task.prompt}
              className='size-full object-cover'
              loading='lazy'
            />
          ) : (
            <div className='flex size-full items-center justify-center'>
              <HugeiconsIcon
                icon={Image01Icon}
                className='text-muted-foreground size-8'
              />
            </div>
          )}
          {props.task.outputImages.length > 1 && (
            <span className='bg-background/90 absolute right-2 bottom-2 rounded px-1.5 py-0.5 text-xs font-medium'>
              {t('{{count}} images', { count: props.task.outputImages.length })}
            </span>
          )}
        </div>
      </button>
      <div className='grid gap-2 p-3'>
        <p className='line-clamp-2 min-h-10 text-sm'>{props.task.prompt}</p>
        <div className='text-muted-foreground flex items-center justify-between gap-2 text-xs'>
          <span>
            {props.task.mode === 'edit' ? t('Edit') : t('Generate')} ·{' '}
            {formatTaskTime(props.task.createdAt)}
          </span>
          <span className='truncate'>{props.task.tokenName}</span>
        </div>
        <div className='flex items-center justify-end gap-1'>
          {image && (
            <Button
              aria-label={t('Download images')}
              size='icon-xs'
              variant='ghost'
              onClick={() => downloadTaskImages(props.task)}
            >
              <HugeiconsIcon icon={Download01Icon} aria-hidden='true' />
            </Button>
          )}
          <Button
            aria-label={t('Edit again')}
            size='icon-xs'
            variant='ghost'
            onClick={() => props.onEdit(props.task)}
          >
            <HugeiconsIcon icon={Edit02Icon} aria-hidden='true' />
          </Button>
          <Button
            aria-label={t('Delete history item')}
            size='icon-xs'
            variant='ghost'
            onClick={() => props.onDelete(props.task)}
          >
            <HugeiconsIcon icon={Delete02Icon} aria-hidden='true' />
          </Button>
        </div>
      </div>
    </article>
  )
}

function TaskDetail(props: {
  task: PlaygroundTask | null
  onClose: () => void
  onEdit: (task: PlaygroundTask) => void
  onDelete: (task: PlaygroundTask) => void
}) {
  const { t } = useTranslation()
  const task = props.task
  if (!task) return null
  return (
    <div
      className='fixed inset-0 z-50 flex items-center justify-center bg-black/35 p-4'
      role='dialog'
      aria-modal='true'
      aria-label={t('Image details')}
    >
      <div className='bg-background grid max-h-[min(90vh,760px)] w-full max-w-5xl overflow-auto rounded-lg border shadow-xl md:grid-cols-[minmax(0,1.1fr)_minmax(280px,0.9fr)]'>
        <div className='bg-muted grid min-h-64 grid-cols-2 content-start gap-2 p-4'>
          {task.outputImages.map((image, index) => (
            <div
              key={image.id}
              className='bg-background relative overflow-hidden rounded-md'
            >
              <img
                src={image.src}
                alt={`${task.prompt} ${index + 1}`}
                className='aspect-square size-full object-contain'
              />
            </div>
          ))}
        </div>
        <div className='grid content-start gap-4 p-5'>
          <div className='flex items-start justify-between gap-4'>
            <div>
              <h2 className='text-base font-semibold'>{t('Image details')}</h2>
              <p className='text-muted-foreground text-xs'>
                {formatTaskTime(task.createdAt)}
              </p>
            </div>
            <Button
              aria-label={t('Close')}
              size='icon-sm'
              variant='ghost'
              onClick={props.onClose}
            >
              <HugeiconsIcon icon={Cancel01Icon} aria-hidden='true' />
            </Button>
          </div>
          <div className='grid gap-1.5'>
            <span className='text-muted-foreground text-xs'>{t('Prompt')}</span>
            <p className='text-sm leading-6'>{task.prompt}</p>
          </div>
          <dl className='grid grid-cols-2 gap-2 text-xs'>
            <div>
              <dt className='text-muted-foreground'>{t('Model')}</dt>
              <dd className='font-medium'>{task.model}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('API key')}</dt>
              <dd className='truncate font-medium'>{task.tokenName}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Size')}</dt>
              <dd className='font-medium'>{task.params.size}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Quality')}</dt>
              <dd className='font-medium'>{task.params.quality}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Format')}</dt>
              <dd className='font-medium'>{task.params.output_format}</dd>
            </div>
            <div>
              <dt className='text-muted-foreground'>{t('Images')}</dt>
              <dd className='font-medium'>{task.outputImages.length}</dd>
            </div>
          </dl>
          <div className='flex flex-wrap gap-2 border-t pt-4'>
            <Button variant='secondary' onClick={() => props.onEdit(task)}>
              <HugeiconsIcon icon={Edit02Icon} aria-hidden='true' />
              {t('Edit again')}
            </Button>
            {task.outputImages.length > 0 && (
              <Button
                variant='outline'
                onClick={() => downloadTaskImages(task)}
              >
                <HugeiconsIcon icon={Download01Icon} aria-hidden='true' />
                {t('Download all')}
              </Button>
            )}
            <Button variant='destructive' onClick={() => props.onDelete(task)}>
              <HugeiconsIcon icon={Delete02Icon} aria-hidden='true' />
              {t('Delete')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

export function ImagePlayground() {
  const { t } = useTranslation()
  const isMobile = useIsMobile()
  const state = useImagePlayground()
  const [selectedTokenId, setSelectedTokenId] = useState<number | null>(null)
  const tokensQuery = useQuery({
    queryKey: ['image-playground', 'tokens'],
    queryFn: getPlaygroundTokens,
    staleTime: 30_000,
  })
  const tokens = useMemo(() => tokensQuery.data ?? [], [tokensQuery.data])
  const selectedTask =
    state.tasks.find((task) => task.id === state.selectedTaskId) || null

  useEffect(() => {
    if (selectedTokenId == null && tokens[0]) setSelectedTokenId(tokens[0].id)
  }, [selectedTokenId, tokens])

  const historyContent = useMemo(() => {
    if (state.isLoadingHistory) {
      return (
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
          <Skeleton className='aspect-square' />
          <Skeleton className='aspect-square' />
          <Skeleton className='aspect-square' />
        </div>
      )
    }
    if (!state.tasks.length) {
      return (
        <Empty className='min-h-64 border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Image01Icon} />
            </EmptyMedia>
            <EmptyTitle>{t('No images yet')}</EmptyTitle>
            <EmptyDescription>
              {t('Your generated images will appear here')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )
    }
    return (
      <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-4'>
        {state.tasks.map((task) => (
          <TaskCard
            key={task.id}
            task={task}
            onOpen={(item) => state.setSelectedTaskId(item.id)}
            onEdit={(item) => void state.editTask(item)}
            onDelete={(item) => void state.deleteTask(item.id)}
          />
        ))}
      </div>
    )
  }, [state, t])

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('GPT Image Playground')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div
            className={cn(
              'mx-auto grid h-full min-h-0 w-full max-w-7xl gap-4',
              isMobile
                ? 'overflow-auto'
                : 'grid-cols-[minmax(280px,360px)_minmax(0,1fr)]'
            )}
          >
            <RequestComposer
              state={state}
              tokens={tokens}
              selectedTokenId={selectedTokenId}
              onTokenChange={setSelectedTokenId}
              tokensLoading={tokensQuery.isLoading}
              tokensError={tokensQuery.isError}
            />
            <section
              className='min-h-0 overflow-auto'
              aria-label={t('Image history')}
            >
              {state.storageWarning && (
                <Alert className='mb-3'>
                  <HugeiconsIcon
                    icon={InformationCircleIcon}
                    aria-hidden='true'
                  />
                  <AlertDescription>{state.storageWarning}</AlertDescription>
                </Alert>
              )}
              {historyContent}
            </section>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <TaskDetail
        task={selectedTask}
        onClose={() => state.setSelectedTaskId(null)}
        onEdit={(task) => {
          state.setSelectedTaskId(null)
          void state.editTask(task)
        }}
        onDelete={(task) => {
          state.setSelectedTaskId(null)
          void state.deleteTask(task.id)
        }}
      />
    </>
  )
}
