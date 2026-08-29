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

import { getMaskCompositeOperation, renderMaskPreview } from '../mask-canvas'

describe('mask canvas semantics', () => {
  test('brush adds transparent edit regions and eraser restores protected regions', () => {
    expect(getMaskCompositeOperation('brush')).toBe('destination-out')
    expect(getMaskCompositeOperation('eraser')).toBe('source-over')
  })

  test('preview paints selected edit regions blue instead of exposing the white mask', () => {
    const operations: string[] = []
    const context = {
      clearRect: vi.fn(() => operations.push('clear')),
      drawImage: vi.fn(() => operations.push('subtract-mask')),
      fillRect: vi.fn(() => operations.push('fill-blue')),
      fillStyle: '',
      globalCompositeOperation: 'source-over',
    }
    const previewCanvas = {
      width: 100,
      height: 80,
      getContext: vi.fn(() => context),
    }
    const maskCanvas = {} as HTMLCanvasElement

    renderMaskPreview(maskCanvas, previewCanvas as unknown as HTMLCanvasElement)

    expect(context.fillStyle).toBe('rgba(37, 99, 235, 0.52)')
    expect(operations).toEqual(['clear', 'fill-blue', 'subtract-mask'])
    expect(context.drawImage).toHaveBeenCalledWith(maskCanvas, 0, 0)
  })
})
