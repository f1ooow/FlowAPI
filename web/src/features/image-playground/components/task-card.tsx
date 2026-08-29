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
  Delete02Icon,
  Edit02Icon,
  InformationCircleIcon,
  Redo02Icon,
  Undo02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { PlaygroundImage, PlaygroundTask } from '../types'

interface TaskCardProps {
  task: PlaygroundTask
  onOpen: (task: PlaygroundTask) => void
  onReuse: (task: PlaygroundTask) => void
  onEdit: (task: PlaygroundTask) => void
  onRetry: (task: PlaygroundTask) => void
  onCancel: (task: PlaygroundTask) => void
  onDelete: (task: PlaygroundTask) => void
  onImageContextMenu: (
    event: React.MouseEvent,
    image: PlaygroundImage,
    task: PlaygroundTask
  ) => void
}

function formatDuration(milliseconds: number | null): string {
  if (milliseconds == null) return '--'
  return `${Math.max(1, Math.round(milliseconds / 1000))}s`
}

function statusPresentation(
  status: PlaygroundTask['status'],
  t: ReturnType<typeof useTranslation>['t']
): { label: string; className: string } {
  if (status === 'running') {
    return { label: t('Generating'), className: 'bg-blue-50 text-blue-700' }
  }
  if (status === 'error') {
    return { label: t('Failed'), className: 'bg-red-50 text-red-600' }
  }
  return { label: t('Default'), className: 'bg-slate-100 text-slate-600' }
}

function ImageSlot(props: {
  image?: PlaygroundImage
  task: PlaygroundTask
  onOpen: (task: PlaygroundTask) => void
  onContextMenu: TaskCardProps['onImageContextMenu']
}) {
  const { t } = useTranslation()
  if (!props.image) {
    if (props.task.status === 'error') {
      return (
        <div className='flex h-full min-h-44 items-center justify-center bg-red-50 text-red-500'>
          <div className='text-center'>
            <div className='mx-auto mb-2 flex size-9 items-center justify-center rounded-full border-2 border-red-300 text-lg'>
              !
            </div>
            <span className='text-xs'>{t('Failed')}</span>
          </div>
        </div>
      )
    }
    return (
      <div className='flex h-full min-h-44 items-center justify-center bg-slate-50 text-slate-400'>
        <div className='text-center'>
          <div className='mx-auto mb-2 size-8 animate-spin rounded-full border-2 border-slate-200 border-t-blue-500' />
          <span className='text-xs'>{t('Generating image...')}</span>
        </div>
      </div>
    )
  }
  const image = props.image
  return (
    <button
      type='button'
      className='group relative block h-full min-h-44 w-full overflow-hidden bg-slate-50'
      onClick={() => props.onOpen(props.task)}
      onContextMenu={(event) => props.onContextMenu(event, image, props.task)}
      aria-label={props.task.prompt}
    >
      <img
        src={image.src}
        alt=''
        className='h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.02]'
      />
      <span className='absolute top-3 left-3 rounded-md bg-slate-900/65 px-2 py-1 text-[11px] font-medium text-white'>
        {props.task.params.size}
      </span>
      {props.task.outputImages.length > 1 ? (
        <span className='absolute bottom-3 left-3 rounded-md bg-slate-900/65 px-2 py-1 text-[11px] font-medium text-white'>
          {props.task.outputImages.length}
        </span>
      ) : null}
    </button>
  )
}

export function TaskCard(props: TaskCardProps) {
  const { t } = useTranslation()
  const firstImage = props.task.outputImages[0]
  const isRunning = props.task.status === 'running'
  const isError = props.task.status === 'error'
  const status = statusPresentation(props.task.status, t)
  return (
    <article className='grid min-h-56 grid-cols-[minmax(140px,38%)_1fr] overflow-hidden rounded-xl border border-slate-200 bg-white shadow-[0_6px_20px_rgba(15,23,42,0.04)] transition-shadow hover:shadow-[0_10px_28px_rgba(15,23,42,0.08)]'>
      <ImageSlot
        image={firstImage}
        task={props.task}
        onOpen={props.onOpen}
        onContextMenu={props.onImageContextMenu}
      />
      <div className='flex min-w-0 flex-col p-4'>
        <button
          type='button'
          className='line-clamp-3 text-left text-sm leading-6 font-semibold text-slate-800 hover:text-blue-700'
          onClick={() => props.onOpen(props.task)}
        >
          {props.task.prompt}
        </button>
        <div className='mt-3 flex flex-wrap items-center gap-1.5'>
          <span
            className={`rounded-md px-2 py-1 text-[11px] font-medium ${status.className}`}
          >
            {status.label}
          </span>
          <span className='rounded-md bg-slate-100 px-2 py-1 text-[11px] text-slate-500'>
            {props.task.params.size}
          </span>
          <span className='rounded-md bg-slate-100 px-2 py-1 text-[11px] text-slate-500'>
            {props.task.params.quality}
          </span>
          {props.task.params.n > 1 ? (
            <span className='rounded-md bg-slate-100 px-2 py-1 text-[11px] text-slate-500'>
              x{props.task.params.n}
            </span>
          ) : null}
        </div>
        {isError ? (
          <p className='mt-3 line-clamp-2 text-xs leading-5 text-red-600'>
            {props.task.error}
          </p>
        ) : null}
        {isRunning ? (
          <p className='mt-3 text-xs text-slate-400'>
            {t('Your image will appear here when it is ready')}
          </p>
        ) : null}
        <div className='mt-auto flex items-center justify-between gap-2 pt-4'>
          <span className='text-[11px] text-slate-400 tabular-nums'>
            {formatDuration(props.task.elapsed)}
          </span>
          <div className='flex items-center gap-1'>
            {isRunning ? (
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onCancel(props.task)}
                aria-label={t('Stop generation')}
              >
                <HugeiconsIcon icon={Cancel01Icon} size={16} />
              </Button>
            ) : null}
            {isError ? (
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onRetry(props.task)}
                aria-label={t('Retry')}
              >
                <HugeiconsIcon icon={Redo02Icon} size={16} />
              </Button>
            ) : null}
            {!isRunning && !isError ? (
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onReuse(props.task)}
                aria-label={t('Reuse configuration')}
              >
                <HugeiconsIcon icon={Undo02Icon} size={16} />
              </Button>
            ) : null}
            {!isRunning && firstImage ? (
              <Button
                variant='ghost'
                size='icon-sm'
                onClick={() => props.onEdit(props.task)}
                aria-label={t('Edit output')}
              >
                <HugeiconsIcon icon={Edit02Icon} size={16} />
              </Button>
            ) : null}
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => props.onOpen(props.task)}
              aria-label={t('View details')}
            >
              <HugeiconsIcon icon={InformationCircleIcon} size={16} />
            </Button>
            <Button
              variant='ghost'
              size='icon-sm'
              className='text-red-500 hover:text-red-600'
              onClick={() => props.onDelete(props.task)}
              aria-label={t('Delete task')}
            >
              <HugeiconsIcon icon={Delete02Icon} size={16} />
            </Button>
          </div>
        </div>
      </div>
    </article>
  )
}
