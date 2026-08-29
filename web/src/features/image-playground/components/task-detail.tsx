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
  Delete02Icon,
  Download01Icon,
  Edit02Icon,
  Undo02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import type { PlaygroundImage, PlaygroundTask } from '../types'

function statusPresentation(
  status: PlaygroundTask['status'],
  t: ReturnType<typeof useTranslation>['t']
): { label: string; className: string } {
  if (status === 'error') {
    return { label: t('Failed'), className: 'bg-red-50 text-red-600' }
  }
  if (status === 'running') {
    return { label: t('Generating'), className: 'bg-blue-50 text-blue-700' }
  }
  return { label: t('Completed'), className: 'bg-slate-100 text-slate-600' }
}

interface TaskDetailProps {
  task: PlaygroundTask
  onClose: () => void
  onReuse: (task: PlaygroundTask) => void
  onEdit: (task: PlaygroundTask) => void
  onDelete: (task: PlaygroundTask) => void
  onDownload: (
    image: PlaygroundImage,
    task: PlaygroundTask,
    index: number
  ) => void
  onCopy: (image: PlaygroundImage) => void
}

export function TaskDetail(props: TaskDetailProps) {
  const { t } = useTranslation()
  const image = props.task.outputImages[0]
  const status = statusPresentation(props.task.status, t)
  return (
    <div
      className='fixed inset-0 z-50 flex items-center justify-center bg-slate-950/25 p-4 backdrop-blur-sm'
      role='dialog'
      aria-modal='true'
      aria-label={t('Image details')}
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) props.onClose()
      }}
    >
      <div className='grid max-h-[min(760px,calc(100vh-32px))] w-full max-w-5xl overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl lg:grid-cols-[minmax(0,1.08fr)_minmax(300px,0.92fr)]'>
        <div className='relative flex min-h-[300px] items-center justify-center bg-slate-50 p-5'>
          {image ? (
            <img
              src={image.src}
              alt={props.task.prompt}
              className='max-h-[68vh] max-w-full rounded-lg object-contain'
            />
          ) : (
            <div className='text-sm text-slate-400'>
              {props.task.status === 'running'
                ? t('Generating image...')
                : t('No image available')}
            </div>
          )}
          <button
            type='button'
            onClick={props.onClose}
            className='absolute top-4 right-4 inline-flex size-9 items-center justify-center rounded-full bg-white/90 text-slate-500 shadow-sm hover:bg-white'
            aria-label={t('Close')}
          >
            <HugeiconsIcon icon={Cancel01Icon} size={18} />
          </button>
          {props.task.outputImages.length > 1 ? (
            <div className='absolute bottom-4 left-4 flex gap-2'>
              {props.task.outputImages.map((output, index) => (
                <button
                  type='button'
                  key={output.id}
                  onClick={() => props.onDownload(output, props.task, index)}
                  className='size-12 overflow-hidden rounded-md border-2 border-white bg-white shadow-sm'
                  aria-label={`${t('Download image')} ${index + 1}`}
                >
                  <img
                    src={output.src}
                    alt=''
                    className='h-full w-full object-cover'
                  />
                </button>
              ))}
            </div>
          ) : null}
        </div>
        <div className='flex min-h-0 flex-col overflow-y-auto p-6'>
          <div className='flex items-start justify-between gap-4'>
            <div>
              <p className='text-xs font-medium tracking-[0.14em] text-slate-400 uppercase'>
                {t('Input prompt')}
              </p>
              <h2 className='mt-2 text-lg leading-7 font-semibold text-slate-900'>
                {props.task.prompt}
              </h2>
            </div>
            <span
              className={`rounded-md px-2 py-1 text-xs font-medium ${status.className}`}
            >
              {status.label}
            </span>
          </div>
          <div className='mt-7'>
            <p className='text-xs font-medium tracking-[0.14em] text-slate-400 uppercase'>
              {t('Parameter configuration')}
            </p>
            <dl className='mt-3 grid grid-cols-2 gap-3'>
              <div className='rounded-lg bg-slate-50 p-3'>
                <dt className='text-xs text-slate-400'>{t('Source')}</dt>
                <dd className='mt-1 text-sm font-medium text-slate-700'>
                  {props.task.tokenName || t('Selected API key')}
                </dd>
              </div>
              <div className='rounded-lg bg-slate-50 p-3'>
                <dt className='text-xs text-slate-400'>{t('Model')}</dt>
                <dd className='mt-1 text-sm font-medium text-slate-700'>
                  {props.task.model}
                </dd>
              </div>
              <div className='rounded-lg bg-slate-50 p-3'>
                <dt className='text-xs text-slate-400'>{t('Size')}</dt>
                <dd className='mt-1 text-sm font-medium text-slate-700'>
                  {props.task.params.size}
                </dd>
              </div>
              <div className='rounded-lg bg-slate-50 p-3'>
                <dt className='text-xs text-slate-400'>{t('Quality')}</dt>
                <dd className='mt-1 text-sm font-medium text-slate-700'>
                  {props.task.params.quality}
                </dd>
              </div>
              <div className='rounded-lg bg-slate-50 p-3'>
                <dt className='text-xs text-slate-400'>{t('Format')}</dt>
                <dd className='mt-1 text-sm font-medium text-slate-700 uppercase'>
                  {props.task.params.output_format}
                </dd>
              </div>
              <div className='rounded-lg bg-slate-50 p-3'>
                <dt className='text-xs text-slate-400'>{t('Quantity')}</dt>
                <dd className='mt-1 text-sm font-medium text-slate-700'>
                  {props.task.params.n}
                </dd>
              </div>
            </dl>
          </div>
          {props.task.error ? (
            <p className='mt-5 rounded-lg bg-red-50 p-3 text-sm leading-6 text-red-700'>
              {props.task.error}
            </p>
          ) : null}
          <div className='mt-auto flex flex-wrap items-center gap-2 border-t border-slate-100 pt-5'>
            {image ? (
              <Button variant='outline' onClick={() => props.onCopy(image)}>
                <HugeiconsIcon icon={Copy01Icon} size={16} />
                {t('Copy')}
              </Button>
            ) : null}
            <Button variant='outline' onClick={() => props.onReuse(props.task)}>
              <HugeiconsIcon icon={Undo02Icon} size={16} />
              {t('Reuse configuration')}
            </Button>
            {image ? (
              <Button
                variant='outline'
                onClick={() => props.onEdit(props.task)}
              >
                <HugeiconsIcon icon={Edit02Icon} size={16} />
                {t('Edit output')}
              </Button>
            ) : null}
            {props.task.outputImages.map((output, index) => (
              <Button
                key={output.id}
                variant='outline'
                onClick={() => props.onDownload(output, props.task, index)}
              >
                <HugeiconsIcon icon={Download01Icon} size={16} />
                {t('Download')}{' '}
                {props.task.outputImages.length > 1 ? index + 1 : ''}
              </Button>
            ))}
            <Button
              variant='destructive'
              onClick={() => props.onDelete(props.task)}
            >
              <HugeiconsIcon icon={Delete02Icon} size={16} />
              {t('Delete task')}
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
