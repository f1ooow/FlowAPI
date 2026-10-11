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
import type { CellContext } from '@tanstack/react-table'
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { TaskLog } from '../../types'
import { useTaskLogsColumns } from '../columns/task-logs-columns'
import { VideoPreviewCell } from '../video-preview'

const task: TaskLog = {
  id: 1,
  user_id: 7,
  platform: 'openai',
  task_id: 'video/id',
  action: 'generate',
  channel_id: 1,
  submit_time: 1,
  status: 'SUCCESS',
}

beforeEach(() => {
  vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:video')
  vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined)
})
afterEach(() => vi.restoreAllMocks())

function renderDetails(log: TaskLog) {
  const { result } = renderHook(() => useTaskLogsColumns(false))
  const column = result.current.find(
    (entry) => 'accessorKey' in entry && entry.accessorKey === 'fail_reason'
  )
  if (!column) throw new Error('Task details column missing')
  const cell = column.cell as (
    props: CellContext<TaskLog, unknown>
  ) => React.ReactNode
  return render(
    createElement(cell, {
      row: { original: log, getValue: () => log.fail_reason },
    } as unknown as CellContext<TaskLog, unknown>)
  )
}

describe('task log video preview', () => {
  test('successful relative result_url offers the authenticated preview', () => {
    renderDetails({ ...task, result_url: '/v1/videos/video-id/content' })
    expect(
      screen.getByRole('button', { name: 'Click to preview video' })
    ).toBeVisible()
  })

  test('successful plain legacy fail_reason remains error details', () => {
    renderDetails({
      ...task,
      result_url: 'Result unavailable',
      fail_reason: 'Result unavailable',
    })
    expect(screen.getByText('Result unavailable')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Click to preview video' })
    ).not.toBeInTheDocument()
  })

  test('successful result_url offers a preview without fetching until clicked', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockResolvedValue({ data: new Blob(['video'], { type: 'video/mp4' }) })
    renderDetails({ ...task, result_url: 'https://provider.example/video.mp4' })
    expect(get).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Click to preview video' })
    )
    expect(
      await screen.findByLabelText('Click to preview video', {
        selector: 'video',
      })
    ).toHaveAttribute('src', 'blob:video')
    expect(get).toHaveBeenCalledWith(
      '/api/task/video%2Fid/content',
      expect.objectContaining({
        responseType: 'blob',
        signal: expect.any(AbortSignal),
        disableDuplicate: true,
      })
    )
  })

  test('legacy successful fail_reason URL still offers a preview', () => {
    renderDetails({
      ...task,
      fail_reason: 'https://provider.example/legacy.mp4',
    })
    expect(
      screen.getByRole('button', { name: 'Click to preview video' })
    ).toBeVisible()
  })

  test('failed tasks retain the error details instead of a video button', () => {
    renderDetails({
      ...task,
      status: 'FAILURE',
      result_url: 'https://provider.example/video.mp4',
      fail_reason: 'Generation failed',
    })
    expect(screen.getByText('Generation failed')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Click to preview video' })
    ).not.toBeInTheDocument()
  })

  test('successful Suno music keeps the audio preview', () => {
    renderDetails({
      ...task,
      platform: 'suno',
      action: 'MUSIC',
      data: JSON.stringify([
        { audio_url: 'https://provider.example/audio.mp3' },
      ]),
    })
    expect(
      screen.getByRole('button', { name: 'Click to preview audio' })
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Click to preview video' })
    ).not.toBeInTheDocument()
  })

  test('pending requests show loading, and failures show an inline error', async () => {
    let reject!: (error: Error) => void
    vi.spyOn(api, 'get').mockImplementation(
      () =>
        new Promise((_resolve, rejectPromise) => {
          reject = rejectPromise
        }) as ReturnType<typeof api.get>
    )
    render(<VideoPreviewCell taskId={task.task_id} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Click to preview video' })
    )
    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
    await act(async () => reject(new Error('unavailable')))
    expect(screen.getByRole('alert')).toHaveTextContent('Failed to load')
  })

  test('closing a loaded preview revokes its blob URL', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: new Blob(['video'], { type: 'video/mp4' }),
    })
    render(<VideoPreviewCell taskId={task.task_id} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Click to preview video' })
    )
    await screen.findByLabelText('Click to preview video', {
      selector: 'video',
    })
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() =>
      expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:video')
    )
  })

  test('unmount aborts a pending request and ignores its stale result', async () => {
    let resolve!: (response: { data: Blob }) => void
    const get = vi.spyOn(api, 'get').mockImplementation(
      () =>
        new Promise((resolvePromise) => {
          resolve = resolvePromise
        }) as ReturnType<typeof api.get>
    )
    const view = render(<VideoPreviewCell taskId={task.task_id} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Click to preview video' })
    )
    const signal = get.mock.calls[0][1]?.signal
    view.unmount()
    expect(signal?.aborted).toBe(true)
    await act(async () => resolve({ data: new Blob(['video']) }))
    expect(URL.createObjectURL).not.toHaveBeenCalled()
  })

  test('closing a pending preview aborts it and reopening starts a new request', async () => {
    let resolve!: (response: { data: Blob }) => void
    const get = vi.spyOn(api, 'get').mockImplementation(
      () =>
        new Promise((resolvePromise) => {
          resolve = resolvePromise
        }) as ReturnType<typeof api.get>
    )
    render(<VideoPreviewCell taskId={task.task_id} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Click to preview video' })
    )
    const signal = get.mock.calls[0][1]?.signal
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    await waitFor(() => expect(signal?.aborted).toBe(true))
    await act(async () => resolve({ data: new Blob(['stale']) }))
    expect(URL.createObjectURL).not.toHaveBeenCalled()
    fireEvent.click(
      screen.getByRole('button', { name: 'Click to preview video' })
    )
    await waitFor(() => expect(get).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('status')).toHaveTextContent('Loading...')
  })
})
