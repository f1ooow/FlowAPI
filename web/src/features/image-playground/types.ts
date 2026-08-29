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
export type PlaygroundOperation = 'generation' | 'edit'
export type PlaygroundTaskStatus = 'running' | 'done' | 'error'

export type ImageSize = 'auto' | '1024x1024' | '1024x1536' | '1536x1024'

export type ImageQuality = 'auto' | 'low' | 'medium' | 'high'
export type ImageOutputFormat = 'png' | 'jpeg' | 'webp'

export interface ImageRequestParams {
  size: ImageSize
  quality: ImageQuality
  output_format: ImageOutputFormat
  n: number
}

export interface PlaygroundToken {
  id: number
  name: string
  maskedKey: string
  status: number
}

export interface PlaygroundImage {
  id: string
  src: string
  blob?: Blob
  sourceUrl?: string
  mimeType: string
  width?: number
  height?: number
  role: 'input' | 'output'
}

export interface PlaygroundMask {
  targetImageId: string
  blob: Blob
  src: string
  width: number
  height: number
  originalWidth?: number
  originalHeight?: number
  wasResized?: boolean
}

export interface PlaygroundTask {
  id: string
  operation: PlaygroundOperation
  status: PlaygroundTaskStatus
  prompt: string
  model: string
  params: ImageRequestParams
  tokenId: number
  tokenName: string
  createdAt: number
  startedAt: number
  completedAt: number | null
  elapsed: number | null
  error: string | null
  inputImages: PlaygroundImage[]
  mask?: PlaygroundMask
  outputImages: PlaygroundImage[]
  actualParams?: Partial<ImageRequestParams>
  revisedPrompts?: string[]
}

export interface ImageResponseItem {
  b64_json?: string
  url?: string
  revised_prompt?: string
}

export interface ImageApiResponse {
  data?: ImageResponseItem[]
  error?: {
    message?: string
    type?: string
    code?: string
  }
  message?: string
}

export interface NormalizedImage {
  id: string
  src: string
  blob?: Blob
  sourceUrl?: string
  mimeType: string
  revisedPrompt?: string
}

export interface ImageApiResult {
  images: NormalizedImage[]
}

export interface StoredImage {
  id: string
  blob?: Blob
  sourceUrl?: string
  mimeType: string
  role: 'input' | 'output' | 'mask'
  createdAt: number
  width?: number
  height?: number
}

export interface StoredTask {
  id: string
  operation: PlaygroundOperation
  status: PlaygroundTaskStatus
  prompt: string
  model: string
  params: ImageRequestParams
  tokenId: number
  tokenName: string
  createdAt: number
  startedAt: number
  completedAt: number | null
  elapsed: number | null
  error: string | null
  inputImageIds: string[]
  outputImageIds: string[]
  maskImageId?: string
  maskTargetImageId?: string
  maskWidth?: number
  maskHeight?: number
  maskOriginalWidth?: number
  maskOriginalHeight?: number
  maskWasResized?: boolean
  actualParams?: Partial<ImageRequestParams>
  revisedPrompts?: string[]
  /** Legacy v1 field retained only for IndexedDB migration. */
  mode?: 'generate' | 'edit'
}
