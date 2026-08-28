import { fetchTokenKey } from '@/features/keys/api'

import {
  IMAGE_PLAYGROUND_MAX_OUTPUTS,
  IMAGE_PLAYGROUND_MODEL,
} from '../constants'
import type {
  ImageApiResponse,
  ImageApiResult,
  ImageRequestParams,
  ImageResponseItem,
  NormalizedImage,
} from '../types'

function createImageId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function outputMimeType(format: ImageRequestParams['output_format']): string {
  return format === 'jpeg' ? 'image/jpeg' : `image/${format}`
}

function dataUrlFromBase64(value: string, mimeType: string): string {
  return `data:${mimeType};base64,${value}`
}

function base64ToBlob(value: string, mimeType: string): Blob {
  const binary = atob(value)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }
  return new Blob([bytes], { type: mimeType })
}

function getErrorMessage(payload: ImageApiResponse, status: number): string {
  return (
    payload.error?.message ||
    payload.message ||
    `Image request failed (${status})`
  )
}

async function parseResponse(response: Response): Promise<ImageApiResponse> {
  let payload: ImageApiResponse
  try {
    payload = (await response.json()) as ImageApiResponse
  } catch {
    throw new Error(`Image request failed (${response.status})`)
  }

  if (!response.ok) throw new Error(getErrorMessage(payload, response.status))
  return payload
}

async function normalizeImage(
  item: ImageResponseItem,
  mimeType: string
): Promise<NormalizedImage | null> {
  if (item.b64_json) {
    const blob = base64ToBlob(item.b64_json, mimeType)
    return {
      id: createImageId(),
      src: dataUrlFromBase64(item.b64_json, mimeType),
      blob,
      mimeType,
      revisedPrompt: item.revised_prompt,
    }
  }

  if (!item.url) return null

  try {
    const imageResponse = await fetch(item.url, { cache: 'no-store' })
    if (imageResponse.ok) {
      const blob = await imageResponse.blob()
      return {
        id: createImageId(),
        src: URL.createObjectURL(blob),
        blob,
        sourceUrl: item.url,
        mimeType: blob.type || mimeType,
        revisedPrompt: item.revised_prompt,
      }
    }
  } catch {
    // The URL remains usable for an immediate preview even when CORS blocks materialization.
  }

  return {
    id: createImageId(),
    src: item.url,
    sourceUrl: item.url,
    mimeType,
    revisedPrompt: item.revised_prompt,
  }
}

export async function normalizeImageResponse(
  payload: ImageApiResponse,
  params: ImageRequestParams
): Promise<ImageApiResult> {
  const items = Array.isArray(payload.data) ? payload.data : []
  const images = (
    await Promise.all(
      items
        .slice(0, IMAGE_PLAYGROUND_MAX_OUTPUTS)
        .map((item) =>
          normalizeImage(item, outputMimeType(params.output_format))
        )
    )
  ).filter((image): image is NormalizedImage => image !== null)

  if (!images.length) throw new Error('The image service returned no images')
  return { images }
}

function normalizeApiKey(apiKey: string): string {
  return apiKey.startsWith('sk-') ? apiKey : `sk-${apiKey}`
}

function requestHeaders(apiKey: string): HeadersInit {
  return {
    Authorization: `Bearer ${normalizeApiKey(apiKey)}`,
  }
}

function boundedOutputCount(value: number): number {
  if (!Number.isFinite(value)) return 1
  return Math.min(IMAGE_PLAYGROUND_MAX_OUTPUTS, Math.max(1, Math.trunc(value)))
}

export async function resolvePlaygroundToken(tokenId: number): Promise<string> {
  const result = await fetchTokenKey(tokenId)
  if (!result.success || !result.data?.key) {
    throw new Error(result.message || 'Unable to load the selected API key')
  }
  return normalizeApiKey(result.data.key)
}

export async function callImageGeneration(options: {
  apiKey: string
  prompt: string
  params: ImageRequestParams
  signal?: AbortSignal
}): Promise<ImageApiResult> {
  const body = {
    model: IMAGE_PLAYGROUND_MODEL,
    prompt: options.prompt.trim(),
    size: options.params.size,
    quality: options.params.quality,
    output_format: options.params.output_format,
    n: boundedOutputCount(options.params.n),
    moderation: 'auto',
  }
  const response = await fetch('/v1/images/generations', {
    method: 'POST',
    headers: {
      ...requestHeaders(options.apiKey),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
    cache: 'no-store',
    signal: options.signal,
  })
  return normalizeImageResponse(await parseResponse(response), options.params)
}

export async function callImageEdit(options: {
  apiKey: string
  prompt: string
  params: ImageRequestParams
  images: Blob[]
  signal?: AbortSignal
}): Promise<ImageApiResult> {
  if (!options.images.length) throw new Error('Add an image before editing')

  const body = new FormData()
  body.append('model', IMAGE_PLAYGROUND_MODEL)
  body.append('prompt', options.prompt.trim())
  body.append('size', options.params.size)
  body.append('quality', options.params.quality)
  body.append('output_format', options.params.output_format)
  body.append('n', String(boundedOutputCount(options.params.n)))
  body.append('moderation', 'auto')
  options.images.forEach((image, index) => {
    body.append('image', image, `reference-${index + 1}.png`)
  })

  const response = await fetch('/v1/images/edits', {
    method: 'POST',
    headers: requestHeaders(options.apiKey),
    body,
    cache: 'no-store',
    signal: options.signal,
  })
  return normalizeImageResponse(await parseResponse(response), options.params)
}
