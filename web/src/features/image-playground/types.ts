export type PlaygroundMode = 'generate' | 'edit'

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
  role: 'input' | 'output'
}

export interface PlaygroundTask {
  id: string
  mode: PlaygroundMode
  prompt: string
  model: string
  params: ImageRequestParams
  tokenId: number
  tokenName: string
  createdAt: number
  completedAt: number
  inputImages: PlaygroundImage[]
  outputImages: PlaygroundImage[]
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
  role: 'input' | 'output'
  createdAt: number
}

export interface StoredTask {
  id: string
  mode: PlaygroundMode
  prompt: string
  model: string
  params: ImageRequestParams
  tokenId: number
  tokenName: string
  createdAt: number
  completedAt: number
  inputImageIds: string[]
  outputImageIds: string[]
}
