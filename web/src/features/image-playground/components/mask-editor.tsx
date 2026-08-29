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
  getMaskCompositeOperation,
  renderMaskPreview,
  type MaskBrushMode,
} from '../lib/mask-canvas'
import {
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

interface MaskSnapshot {
  data: ImageData
}

export function MaskEditor(props: MaskEditorProps) {
  const maskCanvasRef = useRef<HTMLCanvasElement>(null)
  const previewCanvasRef = useRef<HTMLCanvasElement>(null)
  const historyRef = useRef<MaskSnapshot[]>([])
  const redoRef = useRef<MaskSnapshot[]>([])
  const drawingRef = useRef(false)
  const lastPointRef = useRef<{ x: number; y: number } | null>(null)
  const [target, setTarget] = useState<PreparedMaskTarget | null>(null)
  const [mode, setMode] = useState<MaskBrushMode>('brush')
  const [brushSize, setBrushSize] = useState(64)
  const [isSaving, setIsSaving] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void prepareMaskTargetDataUrl(props.image.src)
      .then((prepared) => {
        if (cancelled) return
        setTarget(prepared)
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
    const maskCanvas = maskCanvasRef.current
    const previewCanvas = previewCanvasRef.current
    if (!target || !maskCanvas || !previewCanvas) return

    maskCanvas.width = target.width
    maskCanvas.height = target.height
    previewCanvas.width = target.width
    previewCanvas.height = target.height
    const context = maskCanvas.getContext('2d')
    if (!context) return

    context.globalCompositeOperation = 'source-over'
    context.fillStyle = '#fff'
    context.fillRect(0, 0, target.width, target.height)
    historyRef.current = [
      { data: context.getImageData(0, 0, target.width, target.height) },
    ]
    redoRef.current = []
    renderMaskPreview(maskCanvas, previewCanvas)
  }, [target])

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
    const maskCanvas = maskCanvasRef.current
    const previewCanvas = previewCanvasRef.current
    const context = maskCanvas?.getContext('2d')
    if (!maskCanvas || !previewCanvas || !context) return
    const rect = maskCanvas.getBoundingClientRect()
    const point = {
      x: ((event.clientX - rect.left) / rect.width) * maskCanvas.width,
      y: ((event.clientY - rect.top) / rect.height) * maskCanvas.height,
    }
    const previousPoint = lastPointRef.current ?? point
    context.save()
    context.globalCompositeOperation = getMaskCompositeOperation(mode)
    context.strokeStyle = mode === 'brush' ? '#000' : '#fff'
    context.fillStyle = mode === 'brush' ? '#000' : '#fff'
    context.lineCap = 'round'
    context.lineJoin = 'round'
    context.lineWidth = brushSize
    context.beginPath()
    context.moveTo(previousPoint.x, previousPoint.y)
    context.lineTo(point.x, point.y)
    context.stroke()
    context.beginPath()
    context.arc(point.x, point.y, brushSize / 2, 0, Math.PI * 2)
    context.fill()
    context.restore()
    lastPointRef.current = point
    renderMaskPreview(maskCanvas, previewCanvas)
  }

  const pushHistory = () => {
    const canvas = maskCanvasRef.current
    const context = canvas?.getContext('2d')
    if (!canvas || !context) return
    historyRef.current.push({
      data: context.getImageData(0, 0, canvas.width, canvas.height),
    })
    if (historyRef.current.length > 30) historyRef.current.shift()
    redoRef.current = []
  }

  const undo = () => {
    const canvas = maskCanvasRef.current
    const previewCanvas = previewCanvasRef.current
    const context = canvas?.getContext('2d')
    if (
      !canvas ||
      !previewCanvas ||
      !context ||
      historyRef.current.length < 2
    ) {
      return
    }
    const current = historyRef.current.pop()
    if (current) redoRef.current.push(current)
    const previous = historyRef.current.at(-1)
    if (previous) {
      context.putImageData(previous.data, 0, 0)
      renderMaskPreview(canvas, previewCanvas)
    }
  }

  const redo = () => {
    const canvas = maskCanvasRef.current
    const previewCanvas = previewCanvasRef.current
    const context = canvas?.getContext('2d')
    const next = redoRef.current.pop()
    if (!canvas || !previewCanvas || !context || !next) return
    context.putImageData(next.data, 0, 0)
    historyRef.current.push(next)
    renderMaskPreview(canvas, previewCanvas)
  }

  const clearMask = () => {
    const canvas = maskCanvasRef.current
    const previewCanvas = previewCanvasRef.current
    const context = canvas?.getContext('2d')
    if (!canvas || !previewCanvas || !context) return
    context.globalCompositeOperation = 'source-over'
    context.fillStyle = '#fff'
    context.fillRect(0, 0, canvas.width, canvas.height)
    renderMaskPreview(canvas, previewCanvas)
    pushHistory()
  }

  const save = async () => {
    const canvas = maskCanvasRef.current
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
              画笔选择需要修改的区域，蓝色区域会提交给 GPT Image
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
              src={target.dataUrl}
              alt='Mask source'
              className='block max-h-[calc(100vh-190px)] max-w-[calc(100vw-48px)] object-contain'
            />
            <canvas
              ref={previewCanvasRef}
              className='pointer-events-none absolute inset-0 h-full w-full'
              aria-hidden='true'
            />
            <canvas
              ref={maskCanvasRef}
              className='absolute inset-0 h-full w-full cursor-crosshair touch-none opacity-0'
              aria-label='Mask drawing area'
              onPointerDown={(event) => {
                drawingRef.current = true
                lastPointRef.current = null
                event.currentTarget.setPointerCapture(event.pointerId)
                drawAt(event)
              }}
              onPointerMove={(event) => {
                if (drawingRef.current) drawAt(event)
              }}
              onPointerUp={() => {
                drawingRef.current = false
                lastPointRef.current = null
                pushHistory()
              }}
              onPointerCancel={() => {
                drawingRef.current = false
                lastPointRef.current = null
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
          onClick={() => setMode('brush')}
          aria-pressed={mode === 'brush'}
          className={`inline-flex h-9 items-center gap-2 rounded-lg px-3 text-sm ${mode === 'brush' ? 'bg-blue-50 text-blue-700' : 'text-slate-600 hover:bg-slate-100'}`}
        >
          <HugeiconsIcon icon={PaintBrush01Icon} size={17} /> 画笔
        </button>
        <button
          type='button'
          onClick={() => setMode('eraser')}
          aria-pressed={mode === 'eraser'}
          className={`inline-flex h-9 items-center gap-2 rounded-lg px-3 text-sm ${mode === 'eraser' ? 'bg-blue-50 text-blue-700' : 'text-slate-600 hover:bg-slate-100'}`}
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
