/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Link } from '@tanstack/react-router'
import { GripVertical } from 'lucide-react'
import {
  useRef,
  useState,
  type DragEvent,
  type KeyboardEvent,
  type PointerEvent,
} from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type {
  OverviewPanelId,
  OverviewPanelLayoutItem,
  OverviewPanelSpan,
} from '@/features/dashboard/lib/overview-panels'
import { cn } from '@/lib/utils'

const spanClassNames: Record<OverviewPanelSpan, string> = {
  1: 'col-span-1',
  2: 'col-span-2',
  3: 'col-span-3',
}

type ResizeState = {
  panelId: OverviewPanelId
  pointerId: number
  startX: number
  startSpan: OverviewPanelSpan
  columnStep: number
  lastSpan: OverviewPanelSpan
}

type OverviewLayoutPreviewProps = {
  items: OverviewPanelLayoutItem[]
  panelNames: Record<OverviewPanelId, string>
  onMove: (panelId: OverviewPanelId, targetPanelId: OverviewPanelId) => void
  onMoveByOffset: (panelId: OverviewPanelId, offset: -1 | 1) => void
  onSpanChange: (panelId: OverviewPanelId, span: OverviewPanelSpan) => void
}

function clampSpan(value: number): OverviewPanelSpan {
  return Math.min(3, Math.max(1, value)) as OverviewPanelSpan
}

/**
 * Renders the enabled overview panels on a three-column editing grid.
 */
export function OverviewLayoutPreview(props: OverviewLayoutPreviewProps) {
  const { t } = useTranslation()
  const gridRef = useRef<HTMLDivElement>(null)
  const draggedPanelIdRef = useRef<OverviewPanelId | null>(null)
  const resizeStateRef = useRef<ResizeState | null>(null)
  const [draggedPanelId, setDraggedPanelId] = useState<OverviewPanelId | null>(
    null
  )

  const handleDragStart = (
    event: DragEvent<HTMLButtonElement>,
    panelId: OverviewPanelId
  ) => {
    draggedPanelIdRef.current = panelId
    setDraggedPanelId(panelId)
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', panelId)

    const card = event.currentTarget.closest<HTMLElement>('[data-layout-card]')
    if (card) event.dataTransfer.setDragImage(card, 24, 24)
  }

  const handleDragEnd = () => {
    draggedPanelIdRef.current = null
    setDraggedPanelId(null)
  }

  const handleDragEnter = (targetPanelId: OverviewPanelId) => {
    const panelId = draggedPanelIdRef.current
    if (panelId && panelId !== targetPanelId) {
      props.onMove(panelId, targetPanelId)
    }
  }

  const handleDragKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    panelId: OverviewPanelId
  ) => {
    if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') {
      event.preventDefault()
      props.onMoveByOffset(panelId, -1)
    }
    if (event.key === 'ArrowRight' || event.key === 'ArrowDown') {
      event.preventDefault()
      props.onMoveByOffset(panelId, 1)
    }
  }

  const handleResizeStart = (
    event: PointerEvent<HTMLDivElement>,
    item: OverviewPanelLayoutItem
  ) => {
    const grid = gridRef.current
    if (!grid) return

    const gap = Number.parseFloat(getComputedStyle(grid).columnGap) || 0
    const columnWidth = (grid.getBoundingClientRect().width - gap * 2) / 3
    resizeStateRef.current = {
      panelId: item.id,
      pointerId: event.pointerId,
      startX: event.clientX,
      startSpan: item.span,
      columnStep: columnWidth + gap,
      lastSpan: item.span,
    }
    event.currentTarget.setPointerCapture?.(event.pointerId)
  }

  const handleResizeMove = (event: PointerEvent<HTMLDivElement>) => {
    const resizeState = resizeStateRef.current
    if (!resizeState || resizeState.pointerId !== event.pointerId) return

    const span = clampSpan(
      resizeState.startSpan +
        Math.round(
          (event.clientX - resizeState.startX) / resizeState.columnStep
        )
    )
    if (span === resizeState.lastSpan) return

    resizeState.lastSpan = span
    props.onSpanChange(resizeState.panelId, span)
  }

  const handleResizeEnd = (event: PointerEvent<HTMLDivElement>) => {
    if (resizeStateRef.current?.pointerId === event.pointerId) {
      resizeStateRef.current = null
    }
  }

  const handleResizeKeyDown = (
    event: KeyboardEvent<HTMLDivElement>,
    item: OverviewPanelLayoutItem
  ) => {
    if (event.key === 'ArrowLeft') {
      event.preventDefault()
      props.onSpanChange(item.id, clampSpan(item.span - 1))
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault()
      props.onSpanChange(item.id, clampSpan(item.span + 1))
    }
  }

  if (props.items.length === 0) {
    return (
      <div className='text-muted-foreground flex min-h-32 items-center justify-center rounded-xl border border-dashed px-4 text-sm'>
        {t('Only enabled panels appear on the overview page.')}
      </div>
    )
  }

  return (
    <div className='bg-muted/15 overflow-x-auto rounded-xl border p-3 sm:p-4'>
      <div
        ref={gridRef}
        role='list'
        aria-label={t('Overview content order')}
        className='grid min-w-2xl grid-cols-3 gap-3'
      >
        {props.items.map((item) => {
          const panelName = props.panelNames[item.id]
          return (
            <div
              key={item.id}
              role='listitem'
              data-layout-card
              data-panel-id={item.id}
              data-panel-span={item.span}
              onDragEnter={() => handleDragEnter(item.id)}
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => event.preventDefault()}
              className={cn(
                'bg-card relative flex min-h-32 min-w-0 flex-col rounded-xl border p-3 pr-6 shadow-xs transition-[border-color,opacity,box-shadow]',
                spanClassNames[item.span],
                draggedPanelId === item.id && 'border-primary/50 opacity-60'
              )}
            >
              <div className='flex min-w-0 items-center gap-2'>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  draggable
                  className='text-muted-foreground cursor-grab touch-none active:cursor-grabbing'
                  aria-label={t('Drag {{group}} to reorder', {
                    group: panelName,
                  })}
                  onDragStart={(event) => handleDragStart(event, item.id)}
                  onDragEnd={handleDragEnd}
                  onKeyDown={(event) => handleDragKeyDown(event, item.id)}
                >
                  <GripVertical aria-hidden='true' />
                </Button>
                <span className='min-w-0 flex-1 truncate text-sm font-medium'>
                  {panelName}
                </span>
                <Badge variant='outline' className='tabular-nums'>
                  {item.span} / 3
                </Badge>
              </div>

              <div className='mt-auto flex items-end justify-between gap-3 pt-6'>
                <div
                  className='flex flex-1 flex-col gap-1.5'
                  aria-hidden='true'
                >
                  <span className='bg-muted h-2 w-2/3 rounded-full' />
                  <span className='bg-muted h-2 w-1/2 rounded-full' />
                </div>
                <Button
                  variant='ghost'
                  size='sm'
                  render={
                    <Link
                      to='/system-settings/content/$section'
                      params={{ section: item.id }}
                    />
                  }
                >
                  {t('Configure')}
                </Button>
              </div>

              <div
                role='separator'
                tabIndex={0}
                aria-label={t('Resize {{name}}', { name: panelName })}
                aria-orientation='vertical'
                aria-valuemin={1}
                aria-valuemax={3}
                aria-valuenow={item.span}
                onPointerDown={(event) => handleResizeStart(event, item)}
                onPointerMove={handleResizeMove}
                onPointerUp={handleResizeEnd}
                onPointerCancel={handleResizeEnd}
                onKeyDown={(event) => handleResizeKeyDown(event, item)}
                className='group focus-visible:ring-ring absolute inset-y-2 right-0 flex w-5 cursor-col-resize touch-none items-center justify-center rounded-sm outline-none focus-visible:ring-2'
              >
                <span className='bg-border group-hover:bg-primary/50 group-focus-visible:bg-primary h-12 w-1 rounded-full transition-colors' />
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
