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
export const MASK_WORKING_MAX_EDGE = 1920
export const MASK_WORKING_MULTIPLE = 16

export interface MaskWorkingSize {
  width: number
  height: number
  scale: number
  wasResized: boolean
}

export interface PreparedMaskTarget extends MaskWorkingSize {
  blob: Blob
  dataUrl: string
  originalWidth: number
  originalHeight: number
  wasConvertedToPng: boolean
}

export interface MaskSaveResult {
  maskBlob: Blob
  maskDataUrl: string
  width: number
  height: number
}

function floorToMultiple(value: number, multiple: number): number {
  return Math.max(multiple, Math.floor(value / multiple) * multiple)
}

export function calculateMaskWorkingSize(
  width: number,
  height: number,
  maxEdge = MASK_WORKING_MAX_EDGE,
  multiple = MASK_WORKING_MULTIPLE
): MaskWorkingSize {
  if (width <= 0 || height <= 0) throw new Error('Invalid image dimensions')
  const longestEdge = Math.max(width, height)
  const scale = longestEdge > maxEdge ? maxEdge / longestEdge : 1
  return {
    width: floorToMultiple(width * scale, multiple),
    height: floorToMultiple(height * scale, multiple),
    scale,
    wasResized:
      scale !== 1 || width % multiple !== 0 || height % multiple !== 0,
  }
}

function loadImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error('Unable to load image'))
    image.src = src
  })
}

function canvasToBlob(
  canvas: HTMLCanvasElement,
  type = 'image/png'
): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob)
      else reject(new Error('Unable to encode image'))
    }, type)
  })
}

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () =>
      reject(reader.error || new Error('Unable to read image'))
    reader.readAsDataURL(blob)
  })
}

function readPngDimensions(
  buffer: ArrayBuffer
): { width: number; height: number } | null {
  if (buffer.byteLength < 24) return null
  const bytes = new Uint8Array(buffer, 0, 8)
  const signature = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]
  if (!signature.every((value, index) => bytes[index] === value)) return null

  const view = new DataView(buffer)
  const width = view.getUint32(16)
  const height = view.getUint32(20)
  return width > 0 && height > 0 ? { width, height } : null
}

async function getBlobDimensions(
  blob: Blob
): Promise<{ width: number; height: number }> {
  const pngDimensions = readPngDimensions(await blob.arrayBuffer())
  if (pngDimensions) return pngDimensions
  const image = await loadImage(await blobToDataUrl(blob))
  return { width: image.naturalWidth, height: image.naturalHeight }
}

export async function prepareMaskTargetDataUrl(
  dataUrl: string
): Promise<PreparedMaskTarget> {
  const image = await loadImage(dataUrl)
  const size = calculateMaskWorkingSize(image.naturalWidth, image.naturalHeight)
  const sourceIsPng = /^data:image\/png(?:[;,]|$)/i.test(dataUrl)
  if (!size.wasResized && sourceIsPng) {
    const response = await fetch(dataUrl)
    const blob = await response.blob()
    return {
      ...size,
      blob,
      dataUrl,
      originalWidth: image.naturalWidth,
      originalHeight: image.naturalHeight,
      wasConvertedToPng: false,
    }
  }

  const canvas = document.createElement('canvas')
  canvas.width = size.width
  canvas.height = size.height
  const context = canvas.getContext('2d')
  if (!context) throw new Error('Canvas is not supported by this browser')
  context.imageSmoothingEnabled = true
  context.imageSmoothingQuality = 'high'
  context.drawImage(image, 0, 0, size.width, size.height)
  const blob = await canvasToBlob(canvas)
  return {
    ...size,
    blob,
    dataUrl: await blobToDataUrl(blob),
    originalWidth: image.naturalWidth,
    originalHeight: image.naturalHeight,
    wasConvertedToPng: true,
  }
}

export async function createBlankMask(
  width: number,
  height: number
): Promise<MaskSaveResult> {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const context = canvas.getContext('2d')
  if (!context) throw new Error('Canvas is not supported by this browser')
  context.fillStyle = '#fff'
  context.fillRect(0, 0, width, height)
  const maskBlob = await canvasToBlob(canvas)
  return { maskBlob, maskDataUrl: await blobToDataUrl(maskBlob), width, height }
}

export async function validateMaskDimensions(
  maskBlob: Blob,
  width: number,
  height: number
): Promise<void> {
  const dimensions = await getBlobDimensions(maskBlob)
  if (dimensions.width !== width || dimensions.height !== height) {
    throw new Error(
      `Mask dimensions ${dimensions.width}x${dimensions.height} do not match image ${width}x${height}`
    )
  }
}

export async function validateMaskMatchesImage(
  maskBlob: Blob,
  imageBlob: Blob
): Promise<void> {
  const [maskDimensions, imageDimensions] = await Promise.all([
    getBlobDimensions(maskBlob),
    getBlobDimensions(imageBlob),
  ])
  if (
    maskDimensions.width !== imageDimensions.width ||
    maskDimensions.height !== imageDimensions.height
  ) {
    throw new Error(
      `Mask dimensions ${maskDimensions.width}x${maskDimensions.height} do not match image ${imageDimensions.width}x${imageDimensions.height}`
    )
  }
}

export async function createMaskPreviewDataUrl(
  imageDataUrl: string,
  maskDataUrl: string
): Promise<string> {
  const [image, mask] = await Promise.all([
    loadImage(imageDataUrl),
    loadImage(maskDataUrl),
  ])
  if (
    image.naturalWidth !== mask.naturalWidth ||
    image.naturalHeight !== mask.naturalHeight
  ) {
    throw new Error('Mask dimensions do not match the edited image')
  }

  const canvas = document.createElement('canvas')
  canvas.width = image.naturalWidth
  canvas.height = image.naturalHeight
  const context = canvas.getContext('2d', { willReadFrequently: true })
  if (!context) throw new Error('Canvas is not supported by this browser')
  context.drawImage(image, 0, 0)

  const maskCanvas = document.createElement('canvas')
  maskCanvas.width = mask.naturalWidth
  maskCanvas.height = mask.naturalHeight
  const maskContext = maskCanvas.getContext('2d', {
    willReadFrequently: true,
  })
  if (!maskContext) throw new Error('Canvas is not supported by this browser')
  maskContext.drawImage(mask, 0, 0)

  const maskPixels = maskContext.getImageData(
    0,
    0,
    maskCanvas.width,
    maskCanvas.height
  )
  const overlay = context.createImageData(canvas.width, canvas.height)
  for (let index = 0; index < maskPixels.data.length; index += 4) {
    const editStrength = 255 - maskPixels.data[index + 3]
    overlay.data[index] = 37
    overlay.data[index + 1] = 99
    overlay.data[index + 2] = 235
    overlay.data[index + 3] = Math.round(editStrength * 0.58)
  }

  const overlayCanvas = document.createElement('canvas')
  overlayCanvas.width = canvas.width
  overlayCanvas.height = canvas.height
  const overlayContext = overlayCanvas.getContext('2d')
  if (!overlayContext) {
    throw new Error('Canvas is not supported by this browser')
  }
  overlayContext.putImageData(overlay, 0, 0)
  context.drawImage(overlayCanvas, 0, 0)
  return canvas.toDataURL('image/png')
}
