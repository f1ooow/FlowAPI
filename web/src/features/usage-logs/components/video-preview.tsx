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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { api } from '@/lib/api'

function VideoPreviewPlayer(props: { taskId: string }) {
  const { t } = useTranslation()
  const [videoUrl, setVideoUrl] = useState('')
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    let objectUrl = ''

    // Keep credentials on the dashboard route, never on the provider result URL.
    api
      .get<Blob>(`/api/task/${encodeURIComponent(props.taskId)}/content`, {
        responseType: 'blob',
        signal: controller.signal,
        disableDuplicate: true,
        skipErrorHandler: true,
      })
      .then((response) => {
        if (controller.signal.aborted) return
        if (!response.data.size || response.data.type.includes('json')) {
          setFailed(true)
          return
        }
        objectUrl = URL.createObjectURL(response.data)
        setVideoUrl(objectUrl)
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true)
      })

    return () => {
      controller.abort()
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [props.taskId])

  if (failed) return <p role='alert'>{t('Failed to load')}</p>
  if (!videoUrl) return <p role='status'>{t('Loading...')}</p>

  return (
    <video
      aria-label={t('Click to preview video')}
      src={videoUrl}
      controls
      playsInline
      className='max-h-[65vh] w-full rounded-md'
      onError={() => setFailed(true)}
    />
  )
}

export function VideoPreviewCell(props: { taskId: string }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)

  return (
    <>
      <button
        type='button'
        className='text-foreground text-xs hover:underline'
        onClick={() => setOpen(true)}
      >
        {t('Click to preview video')}
      </button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        title={t('Click to preview video')}
        contentHeight='auto'
      >
        {open && (
          <VideoPreviewPlayer key={props.taskId} taskId={props.taskId} />
        )}
      </Dialog>
    </>
  )
}
