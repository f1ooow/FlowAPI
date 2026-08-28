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

  test('switches to edit mode and requires a reference image before submit', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(await screen.findByRole('tab', { name: 'Edit image' }))
    expect(screen.getByRole('button', { name: 'Edit image' })).toBeDisabled()
    expect(
      screen.getByRole('button', { name: 'Upload reference image' })
    ).toBeEnabled()
  })
})
