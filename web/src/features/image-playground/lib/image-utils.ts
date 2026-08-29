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
import type { PlaygroundImage, PlaygroundTask } from '../types'

export function getImageExtension(
  format: PlaygroundTask['params']['output_format']
): string {
  return format === 'jpeg' ? 'jpg' : format
}

export function downloadImage(image: PlaygroundImage, filename: string): void {
  const anchor = document.createElement('a')
  anchor.href = image.src
  anchor.download = filename
  anchor.rel = 'noreferrer'
  anchor.click()
}

export function downloadTaskImages(task: PlaygroundTask): void {
  const extension = getImageExtension(task.params.output_format)
  task.outputImages.forEach((image, index) => {
    downloadImage(image, `flow-api-${task.id}-${index + 1}.${extension}`)
  })
}

export async function copyImageToClipboard(
  image: PlaygroundImage
): Promise<void> {
  if (!navigator.clipboard?.write || typeof ClipboardItem === 'undefined') {
    throw new Error('Image clipboard is not supported in this browser')
  }
  let blob = image.blob
  if (!blob && image.src) {
    const response = await fetch(image.src, { cache: 'no-store' })
    if (!response.ok) throw new Error('Unable to load this image for copying')
    blob = await response.blob()
  }
  if (!blob) throw new Error('This image is not available for copying')
  await navigator.clipboard.write([
    new ClipboardItem({ [blob.type || 'image/png']: blob }),
  ])
}

export function getImageDimensions(
  blob: Blob
): Promise<{ width: number; height: number }> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(blob)
    const image = new Image()
    image.onload = () => {
      URL.revokeObjectURL(url)
      resolve({ width: image.naturalWidth, height: image.naturalHeight })
    }
    image.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new Error('Unable to read image dimensions'))
    }
    image.src = url
  })
}
