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
import { describe, expect, test, vi } from 'vitest'

import { DEFAULT_IMAGE_PARAMS } from '../../constants'
import {
  callImageEdit,
  callImageGeneration,
  normalizeImageResponse,
} from '../api'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('image playground API', () => {
  test('sends generations through the gateway with the selected API key', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ data: [{ b64_json: 'aGVsbG8=' }] }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await callImageGeneration({
      apiKey: 'sk-selected-key',
      prompt: 'a red apple',
      params: { ...DEFAULT_IMAGE_PARAMS, n: 2 },
    })

    expect(result.images).toHaveLength(1)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v1/images/generations')
    expect((init.headers as Record<string, string>).Authorization).toBe(
      'Bearer sk-selected-key'
    )
    expect(JSON.parse(String(init.body))).toMatchObject({
      model: 'gpt-image-2',
      prompt: 'a red apple',
      n: 2,
      moderation: 'auto',
    })
    expect(
      fetchMock.mock.calls.some(([calledUrl]) => calledUrl === '/v1/responses')
    ).toBe(false)
  })

  test('sends edit images as multipart form data without overriding the boundary', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ data: [{ b64_json: 'aGVsbG8=' }] }))
    vi.stubGlobal('fetch', fetchMock)
    const reference = new Blob(['image'], { type: 'image/png' })

    await callImageEdit({
      apiKey: 'raw-key',
      prompt: 'make it blue',
      params: DEFAULT_IMAGE_PARAMS,
      images: [reference],
    })

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/v1/images/edits')
    expect((init.headers as Record<string, string>).Authorization).toBe(
      'Bearer sk-raw-key'
    )
    expect(
      (init.headers as Record<string, string>)['Content-Type']
    ).toBeUndefined()
    expect(init.body).toBeInstanceOf(FormData)
    const formData = init.body as FormData
    expect(formData.get('model')).toBe('gpt-image-2')
    expect(formData.get('prompt')).toBe('make it blue')
    expect(formData.get('image')).toBeInstanceOf(Blob)
  })

  test('includes a mask in the edit multipart request', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(jsonResponse({ data: [{ b64_json: 'aGVsbG8=' }] }))
    vi.stubGlobal('fetch', fetchMock)

    await callImageEdit({
      apiKey: 'sk-key',
      prompt: 'replace the background',
      params: DEFAULT_IMAGE_PARAMS,
      images: [new Blob(['source'], { type: 'image/png' })],
      mask: new Blob(['mask'], { type: 'image/png' }),
    })

    const formData = fetchMock.mock.calls[0]?.[1]?.body as FormData
    expect(formData.get('image')).toBeInstanceOf(Blob)
    expect(formData.get('mask')).toBeInstanceOf(Blob)
  })

  test('keeps URL results usable when URL materialization is blocked', async () => {
    const imageUrl = 'https://cdn.example.com/image.png'
    const fetchMock = vi.fn().mockRejectedValueOnce(new Error('CORS'))
    vi.stubGlobal('fetch', fetchMock)

    const result = await normalizeImageResponse(
      { data: [{ url: imageUrl }] },
      DEFAULT_IMAGE_PARAMS
    )

    expect(result.images[0]?.src).toBe(imageUrl)
    expect(result.images[0]?.sourceUrl).toBe(imageUrl)
    expect(result.images[0]?.blob).toBeUndefined()
  })

  test('surfaces gateway error messages', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        jsonResponse({ error: { message: 'insufficient quota' } }, 402)
      )
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      callImageGeneration({
        apiKey: 'sk-key',
        prompt: 'test',
        params: DEFAULT_IMAGE_PARAMS,
      })
    ).rejects.toThrow('insufficient quota')
  })
})
