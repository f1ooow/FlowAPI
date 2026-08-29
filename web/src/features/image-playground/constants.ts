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
import type {
  ImageOutputFormat,
  ImageQuality,
  ImageSize,
  ImageRequestParams,
} from './types'

export const IMAGE_PLAYGROUND_MODEL = 'gpt-image-2'
export const IMAGE_PLAYGROUND_MAX_OUTPUTS = 4
export const IMAGE_PLAYGROUND_MAX_INPUT_IMAGES = 16

export const IMAGE_SIZE_OPTIONS: Array<{ value: ImageSize; labelKey: string }> =
  [
    { value: 'auto', labelKey: 'Auto' },
    { value: '1024x1024', labelKey: 'Square · 1024 x 1024' },
    { value: '1024x1536', labelKey: 'Portrait · 1024 x 1536' },
    { value: '1536x1024', labelKey: 'Landscape · 1536 x 1024' },
  ]

export const IMAGE_QUALITY_OPTIONS: Array<{
  value: ImageQuality
  labelKey: string
}> = [
  { value: 'auto', labelKey: 'Auto' },
  { value: 'low', labelKey: 'Low' },
  { value: 'medium', labelKey: 'Medium' },
  { value: 'high', labelKey: 'High' },
]

export const IMAGE_FORMAT_OPTIONS: Array<{
  value: ImageOutputFormat
  labelKey: string
}> = [
  { value: 'png', labelKey: 'PNG' },
  { value: 'jpeg', labelKey: 'JPEG' },
  { value: 'webp', labelKey: 'WebP' },
]

export const DEFAULT_IMAGE_PARAMS: ImageRequestParams = {
  size: 'auto',
  quality: 'auto',
  output_format: 'png',
  n: 1,
}

export const IMAGE_PLAYGROUND_DB_NAME = 'flowapi-image-playground'
export const IMAGE_PLAYGROUND_DB_VERSION = 1
export const IMAGE_PLAYGROUND_TASK_STORE = 'tasks'
export const IMAGE_PLAYGROUND_IMAGE_STORE = 'images'
