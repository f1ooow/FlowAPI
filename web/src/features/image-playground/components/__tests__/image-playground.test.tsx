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
})
