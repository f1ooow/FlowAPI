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
import { describe, expect, test } from 'vitest'

import {
  calculateMaskWorkingSize,
  validateMaskMatchesImage,
} from '../mask-preprocess'

const basePngBytes = Uint8Array.from(
  atob(
    'iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAAHUlEQVR4nGP8////fwYKABMlmkcNGDVg1IDBZAAAa8oEHMu6Z0kAAAAASUVORK5CYII='
  ),
  (value) => value.charCodeAt(0)
)

function pngBlob(width: number, height: number): Blob {
  const bytes = new Uint8Array(basePngBytes)
  const view = new DataView(bytes.buffer)
  view.setUint32(16, width)
  view.setUint32(20, height)
  return new Blob([bytes], { type: 'image/png' })
}

describe('mask working dimensions', () => {
  test('floors non-multiple dimensions to the official 16px grid', () => {
    expect(calculateMaskWorkingSize(1025, 769)).toMatchObject({
      width: 1024,
      height: 768,
      wasResized: true,
    })
  })

  test('scales oversized images before flooring to the grid', () => {
    expect(calculateMaskWorkingSize(4000, 2000)).toMatchObject({
      width: 1920,
      height: 960,
      scale: 0.48,
      wasResized: true,
    })
  })

  test('keeps already valid dimensions unchanged', () => {
    expect(calculateMaskWorkingSize(1024, 1024)).toMatchObject({
      width: 1024,
      height: 1024,
      scale: 1,
      wasResized: false,
    })
  })

  test('rejects a mask whose encoded dimensions differ from its target image', async () => {
    await expect(
      validateMaskMatchesImage(pngBlob(16, 16), pngBlob(32, 16))
    ).rejects.toThrow('Mask dimensions 16x16 do not match image 32x16')
  })

  test('accepts a mask whose encoded dimensions match its target image', async () => {
    await expect(
      validateMaskMatchesImage(pngBlob(16, 16), pngBlob(16, 16))
    ).resolves.toBeUndefined()
  })
})
