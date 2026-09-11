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
import type { ColumnDef, RowSelectionState } from '@tanstack/react-table'
import { Search } from 'lucide-react'
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePagination,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useDebounce } from '@/hooks/use-debounce'

import type { UpstreamChannel } from '../types'
import {
  CHANNEL_STATUS_CONFIG,
  DEFAULT_ENDPOINT,
  ENDPOINT_OPTIONS,
  MODELS_DEV_PRESET_ID,
  OFFICIAL_CHANNEL_ID,
} from './constants'

type ChannelSelectorDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channels: UpstreamChannel[]
  selectedChannelIds: number[]
  onSelectedChannelIdsChange: (ids: number[]) => void
  channelEndpoints: Record<number, string>
  onChannelEndpointsChange: (endpoints: Record<number, string>) => void
  onConfirm: (selectedIds: number[]) => void
}

// Synthesized presets from `controller/ratio_sync.go` always carry stable
// negative IDs, so matching by ID alone is reliable and self-documenting.
function isOfficialChannel(channel: UpstreamChannel): boolean {
  return (
    channel.id === OFFICIAL_CHANNEL_ID || channel.id === MODELS_DEV_PRESET_ID
  )
}

type SyncEndpointEditor = {
  endpoints: Record<number, string>
  /**
   * "custom" is a UI mode, not an endpoint value, so it cannot be encoded by
   * writing a sentinel into `endpoints` (that map is sent to the backend).
   * Tracking the mode separately is what keeps an empty custom endpoint from
   * being mistaken for the `/api/pricing` preset and snapping the select back
   * on the next render.
   */
  customModeChannelIds: Set<number>
  onEndpointTypeChange: (channelId: number, value: string) => void
  onEndpointChange: (channelId: number, endpoint: string) => void
}

/**
 * The endpoint editor state is delivered by context rather than captured in
 * the column definitions on purpose. `flexRender` renders `columnDef.cell` as
 * a component type, so rebuilding the `columns` array on every keystroke would
 * hand React a brand new type and remount the whole cell — the custom endpoint
 * input would lose focus after each character. Keeping `columns` stable and
 * re-rendering this cell through context avoids that.
 */
const SyncEndpointEditorContext = createContext<SyncEndpointEditor | null>(null)

function SyncEndpointCell({ channelId }: { channelId: number }) {
  const { t } = useTranslation()
  const editor = use(SyncEndpointEditorContext)
  if (!editor) return null

  const storedEndpoint = editor.endpoints[channelId]
  const presetEndpoint = storedEndpoint || DEFAULT_ENDPOINT
  const matchesPreset = ENDPOINT_OPTIONS.some(
    (option) => option.value === presetEndpoint
  )
  const isCustom = editor.customModeChannelIds.has(channelId) || !matchesPreset
  const endpointType = isCustom ? 'custom' : presetEndpoint

  return (
    <div className='flex min-w-0 items-center gap-2'>
      <Select
        items={ENDPOINT_OPTIONS.map((option) => ({
          value: option.value,
          label: option.label,
        }))}
        value={endpointType}
        onValueChange={(v) =>
          v !== null && editor.onEndpointTypeChange(channelId, v)
        }
      >
        <SelectTrigger className='h-8 w-32' aria-label={t('Sync Endpoint')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            {ENDPOINT_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {isCustom && (
        <Input
          value={storedEndpoint ?? ''}
          onChange={(e) => editor.onEndpointChange(channelId, e.target.value)}
          placeholder={t('/your/endpoint')}
          aria-label={t('Custom sync endpoint')}
          className='h-8 min-w-0 flex-1 font-mono text-xs'
        />
      )}
    </div>
  )
}

export function ChannelSelectorDialog({
  open,
  onOpenChange,
  channels,
  selectedChannelIds,
  onSelectedChannelIdsChange,
  channelEndpoints,
  onChannelEndpointsChange,
  onConfirm,
}: ChannelSelectorDialogProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search, 200)
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})
  const [customModeChannelIds, setCustomModeChannelIds] = useState<Set<number>>(
    () => new Set()
  )

  useEffect(() => {
    if (!selectedChannelIds.length) {
      setRowSelection({})
      return
    }

    const availableChannelIds = new Set(channels.map((channel) => channel.id))
    const newSelection: RowSelectionState = {}

    selectedChannelIds.forEach((id) => {
      if (availableChannelIds.has(id)) {
        newSelection[id.toString()] = true
      }
    })

    setRowSelection(newSelection)
  }, [selectedChannelIds, channels])

  const updateEndpoint = useCallback(
    (channelId: number, endpoint: string) => {
      onChannelEndpointsChange({
        ...channelEndpoints,
        [channelId]: endpoint,
      })
    },
    [channelEndpoints, onChannelEndpointsChange]
  )

  const selectEndpointType = useCallback(
    (channelId: number, value: string) => {
      setCustomModeChannelIds((prev) => {
        const isCustom = value === 'custom'
        if (prev.has(channelId) === isCustom) return prev
        const next = new Set(prev)
        if (isCustom) {
          next.add(channelId)
        } else {
          next.delete(channelId)
        }
        return next
      })
      updateEndpoint(channelId, value === 'custom' ? '' : value)
    },
    [updateEndpoint]
  )

  const endpointEditor = useMemo<SyncEndpointEditor>(
    () => ({
      endpoints: channelEndpoints,
      customModeChannelIds,
      onEndpointTypeChange: selectEndpointType,
      onEndpointChange: updateEndpoint,
    }),
    [channelEndpoints, customModeChannelIds, selectEndpointType, updateEndpoint]
  )

  const columns = useMemo<ColumnDef<UpstreamChannel>[]>(
    () => [
      {
        id: 'select',
        size: 44,
        minSize: 44,
        header: ({ table }) => (
          <Checkbox
            checked={table.getIsAllPageRowsSelected()}
            indeterminate={table.getIsSomePageRowsSelected()}
            onCheckedChange={(value) =>
              table.toggleAllPageRowsSelected(!!value)
            }
            aria-label='Select all'
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(!!value)}
            aria-label='Select row'
          />
        ),
        enableSorting: false,
        enableHiding: false,
      },
      {
        accessorKey: 'name',
        header: t('Name'),
        size: 300,
        minSize: 220,
        cell: ({ row }) => {
          const name = row.getValue('name') as string
          const channel = row.original
          const isOfficial = isOfficialChannel(channel)

          return (
            <div className='flex items-center gap-2'>
              <span className='font-medium'>{name}</span>
              {isOfficial && (
                <StatusBadge
                  label={t('Official')}
                  variant='success'
                  size='sm'
                  copyable={false}
                />
              )}
            </div>
          )
        },
      },
      {
        accessorKey: 'base_url',
        header: t('Base URL'),
        size: 340,
        minSize: 260,
        cell: ({ row }) => {
          const url = row.getValue('base_url') as string
          return (
            <span
              className='text-muted-foreground block max-w-xs truncate font-mono text-xs'
              title={url}
            >
              {url}
            </span>
          )
        },
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        size: 140,
        minSize: 120,
        cell: ({ row }) => {
          const status = row.getValue('status') as number
          const config =
            CHANNEL_STATUS_CONFIG[status as keyof typeof CHANNEL_STATUS_CONFIG]

          if (!config) {
            return (
              <StatusBadge
                label={t('Unknown')}
                variant='neutral'
                size='sm'
                copyable={false}
              />
            )
          }

          return (
            <StatusBadge
              label={t(config.label)}
              variant={config.variant}
              size='sm'
              copyable={false}
            />
          )
        },
      },
      {
        id: 'endpoint',
        header: t('Sync Endpoint'),
        size: 460,
        minSize: 360,
        cell: ({ row }) => <SyncEndpointCell channelId={row.original.id} />,
      },
    ],
    [t]
  )

  const filteredChannels = useMemo(() => {
    if (!debouncedSearch.trim()) return channels

    const searchLower = debouncedSearch.toLowerCase()
    return channels.filter(
      (ch) =>
        ch.name.toLowerCase().includes(searchLower) ||
        ch.base_url.toLowerCase().includes(searchLower)
    )
  }, [channels, debouncedSearch])

  const sortedChannels = useMemo(() => {
    return [...filteredChannels].sort((a, b) => {
      const aIsOfficial = isOfficialChannel(a)
      const bIsOfficial = isOfficialChannel(b)
      if (aIsOfficial && !bIsOfficial) return -1
      if (!aIsOfficial && bIsOfficial) return 1
      return 0
    })
  }, [filteredChannels])

  const { table } = useDataTable({
    data: sortedChannels,
    columns,
    rowSelection,
    getRowId: (row) => row.id.toString(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    initialPagination: { pageIndex: 0, pageSize: 10 },
    withSortedRowModel: false,
    withFacetedRowModel: false,
  })

  const handleConfirm = () => {
    const selectedRows = table.getSelectedRowModel().rows
    const selectedIds = selectedRows.map((row) => row.original.id)
    onSelectedChannelIdsChange(selectedIds)
    onOpenChange(false)
    onConfirm(selectedIds)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Select Sync Channels')}
      description={t(
        'Choose public pricing sources or upstream channels to compare model pricing'
      )}
      contentClassName='flex max-h-[90vh] max-w-[calc(100%-2rem)] flex-col sm:max-w-[90vw] xl:max-w-[1400px]'
      contentHeight='min(72vh, 720px)'
      bodyClassName='flex h-full min-h-0 flex-col overflow-hidden'
      footer={
        <>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={handleConfirm}>{t('Confirm Selection')}</Button>
        </>
      }
    >
      <SyncEndpointEditorContext value={endpointEditor}>
        <div className='flex h-full min-h-0 flex-col gap-4 overflow-hidden'>
          <div className='flex shrink-0 items-center gap-2'>
            <div className='relative flex-1'>
              <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2' />
              <Input
                placeholder={t('Search by name or URL...')}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className='ps-9'
              />
            </div>
          </div>

          <DataTableView
            table={table}
            containerClassName='min-h-0 flex-1 rounded-md'
            tableContainerClassName='h-full min-h-0'
            tableHeaderClassName='[background-color:var(--table-header)]'
            splitHeaderScrollClassName='h-full'
            bodyContainerClassName='[scrollbar-gutter:stable]'
            splitHeader
            getColumnClassName={(columnId, part) => {
              if (columnId === 'select') return 'w-11 text-center align-middle'
              if (columnId === 'status') {
                return part === 'header' ? 'h-11 align-middle' : 'align-middle'
              }
              return part === 'header' ? 'h-11 align-middle' : 'align-middle'
            }}
            emptyContent={t('No channels found')}
            emptyCellClassName='h-24 text-center'
          />

          <div className='shrink-0'>
            <DataTablePagination table={table} />
          </div>
        </div>
      </SyncEndpointEditorContext>
    </Dialog>
  )
}
