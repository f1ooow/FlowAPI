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
import {
  Cancel01Icon,
  EraserIcon,
  PaintBrush01Icon,
  Redo02Icon,
  Undo02Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useRef, useState, type PointerEvent } from 'react'
import { createPortal } from 'react-dom'

import {
  createBlankMask,
  prepareMaskTargetDataUrl,
  type MaskSaveResult,
  type PreparedMaskTarget,
} from '../lib/mask-preprocess'
import type { PlaygroundImage } from '../types'

interface MaskEditorProps {
  image: PlaygroundImage
  onClose: () => void
  onSave: (target: PreparedMaskTarget, mask: MaskSaveResult) => void
}

type BrushMode = 'erase' | 'restore'

interface MaskSnapshot {
  data: ImageData
}

export function MaskEditor(props: MaskEditorProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const sourceRef = useRef<HTMLImageElement>(null)
  const historyRef = useRef<MaskSnapshot[]>([])
  const redoRef = useRef<MaskSnapshot[]>([])
  const drawingRef = useRef(false)
  const [target, setTarget] = useState<PreparedMaskTarget | null>(null)
  const [mode, setMode] = useState<BrushMode>('erase')
  const [brushSize, setBrushSize] = useState(64)
  const [isSaving, setIsSaving] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void prepareMaskTargetDataUrl(props.image.src)
      .then(async (prepared) => {
        if (cancelled) return
        setTarget(prepared)
        const canvas = canvasRef.current
        if (!canvas) return
        canvas.width = prepared.width
        canvas.height = prepared.height
        const context = canvas.getContext('2d')
        if (!context) return
        const blank = await createBlankMask(prepared.width, prepared.height)
        const maskImage = new Image()
        maskImage.onload = () => {
          context.clearRect(0, 0, prepared.width, prepared.height)
          context.drawImage(maskImage, 0, 0)
          historyRef.current = [
            {
              data: context.getImageData(0, 0, prepared.width, prepared.height),
            },
          ]
        }
        maskImage.src = blank.maskDataUrl
      })
      .catch(() => {
        if (!cancelled) {
          setLoadError('Unable to prepare this image for mask editing')
        }
      })
    return () => {
      cancelled = true
    }
  }, [props.image.src])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') props.onClose()
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'z') {
        event.preventDefault()
        if (event.shiftKey) redo()
        else undo()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  })

  const drawAt = (event: PointerEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current
    const context = canvas?.getContext('2d')
    if (!canvas || !context) return
    const rect = canvas.getBoundingClientRect()
    const x = ((event.clientX - rect.left) / rect.width) * canvas.width
    const y = ((event.clientY - rect.top) / rect.height) * canvas.height
    context.save()
    context.globalCompositeOperation =
      mode === 'erase' ? 'destination-out' : 'source-over'
    context.fillStyle = mode === 'erase' ? 'rgba(0,0,0,1)' : '#fff'
    context.beginPath()
    context.arc(x, y, brushSize / 2, 0, Math.PI * 2)
    context.fill()
    context.restore()
  }

  const pushHistory = () => {
    const canvas = canvasRef.current
    const context = canvas?.getContext('2d')
    if (!canvas || !context) return
    historyRef.current.push({
      data: context.getImageData(0, 0, canvas.width, canvas.height),
    })
    if (historyRef.current.length > 30) historyRef.current.shift()
    redoRef.current = []
  }

  const undo = () => {
    const canvas = canvasRef.current
    const context = canvas?.getContext('2d')
    if (!canvas || !context || historyRef.current.length < 2) return
    const current = historyRef.current.pop()
    if (current) redoRef.current.push(current)
    const previous = historyRef.current.at(-1)
    if (previous) context.putImageData(previous.data, 0, 0)
  }

  const redo = () => {
    const canvas = canvasRef.current
    const context = canvas?.getContext('2d')
    const next = redoRef.current.pop()
    if (!canvas || !context || !next) return
    context.putImageData(next.data, 0, 0)
    historyRef.current.push(next)
  }

  const clearMask = () => {
    const canvas = canvasRef.current
    const context = canvas?.getContext('2d')
    if (!canvas || !context) return
    context.clearRect(0, 0, canvas.width, canvas.height)
    pushHistory()
  }

  const save = async () => {
    const canvas = canvasRef.current
    if (!target || !canvas) return
    setIsSaving(true)
    try {
      const blob = await new Promise<Blob | null>((resolve) =>
        canvas.toBlob(resolve, 'image/png')
      )
      if (!blob) throw new Error('Unable to encode mask')
      const maskDataUrl = canvas.toDataURL('image/png')
      props.onSave(target, {
        maskBlob: blob,
        maskDataUrl,
        width: target.width,
        height: target.height,
      })
    } catch {
      setLoadError('Unable to save this mask')
    } finally {
      setIsSaving(false)
    }
  }

  const content = (
    <div
      className='fixed inset-0 z-[100] flex flex-col bg-white text-slate-900'
      role='dialog'
      aria-modal='true'
      aria-label='Edit mask'
    >
      <header className='flex h-16 shrink-0 items-center justify-between border-b border-slate-200 px-5 sm:px-8'>
        <div className='flex items-center gap-3'>
          <button
            type='button'
            onClick={props.onClose}
            className='inline-flex size-9 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100'
            aria-label='Close mask editor'
          >
            <HugeiconsIcon icon={Cancel01Icon} size={20} />
          </button>
          <div>
            <h2 className='text-base font-semibold'>编辑遮罩</h2>
            <p className='text-xs text-slate-500'>
              涂抹需要修改的区域，透明区域会提交给 GPT Image
            </p>
          </div>
        </div>
        <button
          type='button'
          onClick={() => void save()}
          disabled={!target || isSaving || Boolean(loadError)}
          className='rounded-lg bg-blue-600 px-5 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50'
        >
          {isSaving ? '保存中...' : '保存'}
        </button>
      </header>
      <main className='relative flex min-h-0 flex-1 items-center justify-center overflow-auto bg-slate-50 p-5 sm:p-10'>
        {loadError ? <p className='text-sm text-red-600'>{loadError}</p> : null}
        {target ? (
          <div className='relative max-h-full max-w-full overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm'>
            <img
              ref={sourceRef}
              src={target.dataUrl}
              alt='Mask source'
              className='block max-h-[calc(100vh-190px)] max-w-[calc(100vw-48px)] object-contain'
            />
            <canvas
              ref={canvasRef}
              className='absolute inset-0 h-full w-full cursor-crosshair opacity-60'
              onPointerDown={(event) => {
                drawingRef.current = true
                event.currentTarget.setPointerCapture(event.pointerId)
                drawAt(event)
              }}
              onPointerMove={(event) => {
                if (drawingRef.current) drawAt(event)
              }}
              onPointerUp={() => {
                drawingRef.current = false
                pushHistory()
              }}
              onPointerCancel={() => {
                drawingRef.current = false
              }}
            />
          </div>
        ) : (
          <div className='size-24 animate-pulse rounded-xl bg-slate-200' />
        )}
      </main>
      <footer className='flex shrink-0 items-center justify-center gap-2 border-t border-slate-200 bg-white p-3'>
        <button
          type='button'
          onClick={() => setMode('erase')}
          aria-pressed={mode === 'erase'}
          className={`inline-flex h-9 items-center gap-2 rounded-lg px-3 text-sm ${mode === 'erase' ? 'bg-blue-50 text-blue-700' : 'text-slate-600 hover:bg-slate-100'}`}
        >
          <HugeiconsIcon icon={PaintBrush01Icon} size={17} /> 画笔
        </button>
        <button
          type='button'
          onClick={() => setMode('restore')}
          aria-pressed={mode === 'restore'}
          className={`inline-flex h-9 items-center gap-2 rounded-lg px-3 text-sm ${mode === 'restore' ? 'bg-blue-50 text-blue-700' : 'text-slate-600 hover:bg-slate-100'}`}
        >
          <HugeiconsIcon icon={EraserIcon} size={17} /> 橡皮擦
        </button>
        <label className='mx-2 flex items-center gap-2 text-xs text-slate-500'>
          <span>笔刷</span>
          <input
            type='range'
            min='16'
            max='240'
            step='8'
            value={brushSize}
            onChange={(event) => setBrushSize(Number(event.target.value))}
            className='w-28 accent-blue-600'
          />
          <span className='w-8 tabular-nums'>{brushSize}</span>
        </label>
        <button
          type='button'
          onClick={undo}
          className='inline-flex size-9 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100'
          aria-label='Undo'
        >
          <HugeiconsIcon icon={Undo02Icon} size={18} />
        </button>
        <button
          type='button'
          onClick={redo}
          className='inline-flex size-9 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100'
          aria-label='Redo'
        >
          <HugeiconsIcon icon={Redo02Icon} size={18} />
        </button>
        <button
          type='button'
          onClick={clearMask}
          className='rounded-lg px-3 py-2 text-xs text-slate-500 hover:bg-slate-100'
        >
          清空
        </button>
      </footer>
      {target?.wasResized ? (
        <div className='absolute bottom-20 left-1/2 -translate-x-1/2 rounded-full border border-blue-100 bg-white px-4 py-2 text-xs text-slate-600 shadow-sm'>
          已按官方要求调整为 {target.width} x {target.height} PNG
        </div>
      ) : null}
    </div>
  )

  return createPortal(content, document.body)
}
