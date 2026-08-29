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
export type MaskBrushMode = 'brush' | 'eraser'

export function getMaskCompositeOperation(
  mode: MaskBrushMode
): GlobalCompositeOperation {
  return mode === 'brush' ? 'destination-out' : 'source-over'
}

export function renderMaskPreview(
  maskCanvas: HTMLCanvasElement,
  previewCanvas: HTMLCanvasElement
): void {
  const context = previewCanvas.getContext('2d')
  if (!context) return

  context.clearRect(0, 0, previewCanvas.width, previewCanvas.height)
  context.globalCompositeOperation = 'source-over'
  context.fillStyle = 'rgba(37, 99, 235, 0.52)'
  context.fillRect(0, 0, previewCanvas.width, previewCanvas.height)
  context.globalCompositeOperation = 'destination-out'
  context.drawImage(maskCanvas, 0, 0)
  context.globalCompositeOperation = 'source-over'
}
