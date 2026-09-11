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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import type {
  MaskSaveResult,
  PreparedMaskTarget,
} from '../../lib/mask-preprocess'

vi.mock('@/features/keys/api', () => ({
  fetchTokenKey: vi.fn().mockResolvedValue({
    success: true,
    data: { key: 'selected-key' },
  }),
  getApiKeys: vi.fn().mockResolvedValue({
    success: true,
    data: {
      items: [
        {
          id: 1,
          name: 'Image key',
          key: 'sk-****demo',
          status: 1,
        },
      ],
      total: 1,
      page: 1,
      page_size: 100,
    },
  }),
}))

vi.mock('../../lib/storage', () => ({
  deleteTaskBundle: vi.fn().mockResolvedValue(undefined),
  getStoredImage: vi.fn(),
  listStoredTasks: vi.fn().mockResolvedValue([]),
  saveTaskBundle: vi.fn().mockResolvedValue(undefined),
}))

const preparedPngBase64 =
  'iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAHUlEQVR4nGP8////fwYKABMlmkcNGDVg1IDBZAAAa8oEHMu6Z0kAAAAASUVORK5CYII='
const preparedPngBytes = Uint8Array.from(atob(preparedPngBase64), (value) =>
  value.charCodeAt(0)
)

vi.mock('../mask-editor', () => ({
  MaskEditor: (props: {
    onSave: (target: PreparedMaskTarget, mask: MaskSaveResult) => void
    onClose: () => void
  }) => {
    const targetBlob = new Blob([preparedPngBytes, 'prepared-image'], {
      type: 'image/png',
    })
    const target: PreparedMaskTarget = {
      blob: targetBlob,
      dataUrl: `data:image/png;base64,${preparedPngBase64}`,
      width: 16,
      height: 16,
      scale: 1,
      wasResized: true,
      originalWidth: 20,
      originalHeight: 20,
      wasConvertedToPng: true,
    }
    const mask: MaskSaveResult = {
      maskBlob: new Blob([preparedPngBytes], { type: 'image/png' }),
      maskDataUrl: target.dataUrl,
      width: 16,
      height: 16,
    }
    return (
      <div role='dialog' aria-label='Mock mask editor'>
        <button type='button' onClick={() => props.onClose()}>
          Cancel mock mask
        </button>
        <button type='button' onClick={() => props.onSave(target, mask)}>
          Save mock mask
        </button>
      </div>
    )
  },
}))

const { ImagePlayground } = await import('../image-playground')

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ImagePlayground />
    </QueryClientProvider>
  )
}

describe('ImagePlayground', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  test('submits a generation with the selected API key and renders the result', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: [
            {
              b64_json:
                'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=',
            },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderPage()

    const prompt = await screen.findByRole('textbox', { name: 'Prompt' })
    await user.type(prompt, 'a red apple')
    await user.click(screen.getByRole('button', { name: 'Generate image' }))

    await waitFor(() => {
      expect(
        screen.getAllByRole('button', { name: 'a red apple' }).length
      ).toBeGreaterThan(0)
    })
    expect(fetchMock).toHaveBeenCalledWith(
      '/v1/images/generations',
      expect.objectContaining({
        method: 'POST',
        body: expect.stringContaining('a red apple'),
      })
    )
  })

  test('keeps one unified composer without a visible generation or edit mode', async () => {
    renderPage()

    expect(
      screen.queryByRole('tab', { name: 'Edit image' })
    ).not.toBeInTheDocument()
    expect(
      await screen.findByRole('button', { name: 'Upload reference image' })
    ).toBeEnabled()
    expect(screen.getByRole('textbox', { name: 'Prompt' })).toBeEnabled()
  })

  test('replaces the edited reference target before sending its mask', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: [{ b64_json: 'aGVsbG8=' }],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderPage()

    const file = new File(['original-image'], 'original.jpg', {
      type: 'image/jpeg',
    })
    const fileInput = document.querySelector('input[type="file"]')
    if (!(fileInput instanceof HTMLInputElement)) {
      throw new Error('Reference image file input is not rendered')
    }
    await user.upload(fileInput, file)
    await user.click(screen.getByRole('button', { name: 'Edit image' }))
    await user.click(screen.getByRole('button', { name: 'Save mock mask' }))

    const thumbnail = screen
      .getByRole('button', {
        name: 'Preview reference image',
      })
      .querySelector('img')
    expect(thumbnail).toHaveAttribute(
      'src',
      expect.stringContaining('data:image/png')
    )

    await user.type(
      screen.getByRole('textbox', { name: 'Prompt' }),
      'replace logo'
    )
    const generateButton = screen.getByRole('button', {
      name: 'Generate image',
    })
    await waitFor(() => expect(generateButton).toBeEnabled())
    await user.click(generateButton)

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/v1/images/edits',
        expect.objectContaining({ method: 'POST' })
      )
    )
    const formData = fetchMock.mock.calls[0]?.[1]?.body as FormData
    const sentImage = formData.get('image')
    expect(sentImage).toBeInstanceOf(Blob)
    await expect((sentImage as Blob).text()).resolves.toContain(
      'prepared-image'
    )
    expect(formData.get('mask')).toBeInstanceOf(Blob)
  })

  test('sends a later masked reference as the first edit image', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          data: [{ b64_json: 'aGVsbG8=' }],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      )
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderPage()

    const firstFile = new File(['first-image'], 'first.png', {
      type: 'image/png',
    })
    const secondFile = new File(['second-image'], 'second.png', {
      type: 'image/png',
    })
    const fileInput = document.querySelector('input[type="file"]')
    if (!(fileInput instanceof HTMLInputElement)) {
      throw new Error('Reference image file input is not rendered')
    }
    await user.upload(fileInput, [firstFile, secondFile])
    const editButtons = screen.getAllByRole('button', { name: 'Edit image' })
    await user.click(editButtons[1])
    await user.click(screen.getByRole('button', { name: 'Save mock mask' }))
    await user.type(
      screen.getByRole('textbox', { name: 'Prompt' }),
      'replace logo'
    )

    const generateButton = screen.getByRole('button', {
      name: 'Generate image',
    })
    await waitFor(() => expect(generateButton).toBeEnabled())
    await user.click(generateButton)

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/v1/images/edits',
        expect.objectContaining({ method: 'POST' })
      )
    )
    const formData = fetchMock.mock.calls[0]?.[1]?.body as FormData
    const sentImages = formData.getAll('image[]')
    expect(sentImages).toHaveLength(2)
    expect(sentImages[0]).toBeInstanceOf(Blob)
    await expect((sentImages[0] as Blob).text()).resolves.toContain(
      'prepared-image'
    )
  })
})
