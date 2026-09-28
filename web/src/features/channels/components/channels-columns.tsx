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
/* eslint-disable react-refresh/only-export-components */
import type { ColumnDef } from '@tanstack/react-table'
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  ListOrdered,
  Shuffle,
  SlidersHorizontal,
} from 'lucide-react'
import { useMemo, useContext } from 'react'
import { useTranslation } from 'react-i18next'

import { BadgeListCell } from '@/components/data-table'
import { ProviderBadge } from '@/components/provider-badge'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { TruncatedText } from '@/components/truncated-text'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampToDate } from '@/lib/format'
import { truncateText } from '@/lib/utils'

import { MODEL_FETCHABLE_TYPES } from '../constants'
import { useChannelPassthrough } from '../hooks/use-channel-passthrough'
import {
  formatRelativeTime,
  formatResponseTime,
  getChannelQuotaColumnTitle,
  getChannelTypeIcon,
  getChannelTypeLabel,
  getResponseTimeConfig,
  isMultiKeyChannel,
  parseModelsList,
  parseChannelSettings,
  isTagAggregateRow,
  type TagRow,
} from '../lib'
import { parseUpstreamUpdateMeta } from '../lib/upstream-update-utils'
import type { Channel } from '../types'
import { ChannelGroupsCell } from './channel-groups-cell'
import { ChannelQuotaValueCell } from './channel-quota-summary-cell'
import { ChannelQuotaSummaryHeader } from './channel-quota-summary-header'
import { ChannelRoutingValueCell } from './channel-routing-cell'
import { ChannelRowActionsLayoutContext } from './channel-row-actions-context'
import { ChannelTypeLogo } from './channel-type-logo'
import { useChannels } from './channels-provider'
import { DataTableRowActions } from './data-table-row-actions'
import { DataTableTagRowActions } from './data-table-tag-row-actions'

function parseIonetMeta(otherInfo: string | null | undefined): null | {
  source?: string
  deployment_id?: string
} {
  if (!otherInfo) {
    return null
  }
  try {
    const parsed = JSON.parse(otherInfo)
    if (parsed && typeof parsed === 'object') {
      return parsed
    }
  } catch {
    return null
  }
  return null
}

/**
 * Upstream update tags (+N / -N) shown on channel name for model-fetchable channels
 */
function UpstreamUpdateTags({ channel }: { channel: Channel }) {
  const { upstream, setCurrentRow } = useChannels()
  if (!MODEL_FETCHABLE_TYPES.has(channel.type)) {
    return null
  }

  const meta = parseUpstreamUpdateMeta(channel.settings)
  if (!meta.enabled) {
    return null
  }

  const addCount = meta.pendingAddModels.length
  const removeCount = meta.pendingRemoveModels.length
  if (addCount === 0 && removeCount === 0) {
    return null
  }

  return (
    <div className='flex items-center gap-0.5'>
      {addCount > 0 && (
        <StatusBadge
          label={`+${addCount}`}
          variant='success'
          size='sm'
          copyable={false}
          className='cursor-pointer'
          onClick={(e: React.MouseEvent) => {
            e.stopPropagation()
            setCurrentRow(channel)
            upstream.openModal(
              channel,
              meta.pendingAddModels,
              meta.pendingRemoveModels,
              'add'
            )
          }}
        />
      )}
      {removeCount > 0 && (
        <StatusBadge
          label={`-${removeCount}`}
          variant='danger'
          size='sm'
          copyable={false}
          className='cursor-pointer'
          onClick={(e: React.MouseEvent) => {
            e.stopPropagation()
            setCurrentRow(channel)
            upstream.openModal(
              channel,
              meta.pendingAddModels,
              meta.pendingRemoveModels,
              'remove'
            )
          }}
        />
      )}
    </div>
  )
}

const SENSITIVE_MASK = '••••'

/**
 * Rotation mode marker of a multi-key channel.
 */
function ChannelMultiKeyModeIcon(props: { channel: Channel }) {
  const { t } = useTranslation()
  if (!isMultiKeyChannel(props.channel)) {
    return null
  }

  const multiKeyMode = props.channel.channel_info?.multi_key_mode ?? 'random'
  const MultiKeyModeIcon = multiKeyMode === 'random' ? Shuffle : ListOrdered
  const multiKeyTooltip =
    multiKeyMode === 'random'
      ? t('Multi-key: Random rotation')
      : t('Multi-key: Polling rotation')

  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger
          render={
            <span className='border-border bg-muted text-primary inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md border' />
          }
        >
          <MultiKeyModeIcon className='h-3 w-3' />
        </TooltipTrigger>
        <TooltipContent side='top'>{multiKeyTooltip}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

/**
 * Badge of a channel created from an IO.NET deployment; opens the deployment.
 */
function ChannelIonetBadge(props: { channel: Channel }) {
  const { t } = useTranslation()
  const ionetMeta = parseIonetMeta(props.channel.other_info)
  if (ionetMeta?.source !== 'ionet') {
    return null
  }

  const deploymentId =
    typeof ionetMeta.deployment_id === 'string'
      ? ionetMeta.deployment_id
      : undefined

  return (
    <TooltipProvider delay={100}>
      <Tooltip>
        <TooltipTrigger
          render={
            <span
              className='flex cursor-pointer items-center gap-1.5 text-xs font-medium'
              onClick={(e) => {
                e.stopPropagation()
                if (!deploymentId) {
                  return
                }
                const targetUrl = `/models/deployments?dFilter=${encodeURIComponent(String(deploymentId))}`
                window.open(targetUrl, '_blank', 'noopener')
              }}
            />
          }
        >
          <StatusBadge
            label='IO.NET'
            variant='purple'
            size='sm'
            copyable={false}
            className='cursor-pointer'
          />
        </TooltipTrigger>
        <TooltipContent side='top'>
          <div className='max-w-xs space-y-1'>
            <div className='text-xs'>{t('From IO.NET deployment')}</div>
            {deploymentId && (
              <div className='text-muted-foreground font-mono text-xs'>
                {t('Deployment ID')}: {deploymentId}
              </div>
            )}
            <div className='text-muted-foreground text-xs'>
              {t('Click to open deployment')}
            </div>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

/**
 * Name cell of a regular channel row. The table has no type column, so table
 * view leads with the type logo and carries the multi-key and IO.NET markers;
 * the card header already shows them next to the type label.
 */
function ChannelNameCell(props: {
  channel: Channel
  globalBodyPassthrough: boolean
  globalHeaderPassthrough: boolean
}) {
  const { t } = useTranslation()
  const layout = useContext(ChannelRowActionsLayoutContext)
  const { sensitiveVisible } = useChannels()
  const showTypeMarkers = layout === 'table'
  const settings = parseChannelSettings(props.channel.setting)
  const isPassThrough =
    props.globalBodyPassthrough || settings.pass_through_body_enabled === true
  const hasParamOverride = Boolean(props.channel.param_override?.trim())
  const typeName = t(getChannelTypeLabel(props.channel.type))

  return (
    <div className='flex max-w-full min-w-0 flex-nowrap items-center gap-2'>
      <div className='flex max-w-full min-w-0 flex-col gap-1'>
        {/* The name and its few fixed-size markers share one row that never
          wraps; the groups live in a column of their own. */}
        <div className='flex max-w-full min-w-0 flex-nowrap items-center gap-1.5'>
          {showTypeMarkers && (
            <TooltipProvider delay={300}>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <span
                      role='img'
                      aria-label={typeName}
                      className='inline-flex shrink-0'
                    />
                  }
                >
                  <ChannelTypeLogo type={props.channel.type} size={16} />
                </TooltipTrigger>
                <TooltipContent side='top'>{typeName}</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
          <TruncatedText
            text={sensitiveVisible ? props.channel.name : SENSITIVE_MASK}
            className='shrink font-medium'
            maxWidth='max-w-full min-w-0'
          />
          {showTypeMarkers && (
            <ChannelMultiKeyModeIcon channel={props.channel} />
          )}
          {showTypeMarkers && <ChannelIonetBadge channel={props.channel} />}
          {isPassThrough && (
            <TooltipProvider delay={100}>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <AlertTriangle className='h-3.5 w-3.5 flex-shrink-0 text-amber-500' />
                  }
                />
                <TooltipContent side='top'>
                  {props.globalBodyPassthrough
                    ? t('Request body passthrough is enabled globally')
                    : t(
                        'Request body pass-through is enabled. The request body will be sent directly to the upstream without any conversion.'
                      )}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
          {props.globalHeaderPassthrough && (
            <TooltipProvider delay={100}>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <SlidersHorizontal className='text-info h-3.5 w-3.5 flex-shrink-0' />
                  }
                />
                <TooltipContent side='top'>
                  {t('Request header passthrough is enabled globally')}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
          {hasParamOverride && (
            <TooltipProvider delay={100}>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <SlidersHorizontal className='text-info h-3.5 w-3.5 flex-shrink-0' />
                  }
                />
                <TooltipContent side='top'>
                  {t('Override request parameters')}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
          <UpstreamUpdateTags channel={props.channel} />
        </div>
        {props.channel.remark && (
          <TooltipProvider delay={200}>
            <Tooltip>
              <TooltipTrigger
                render={<span className='text-muted-foreground text-xs' />}
              >
                {truncateText(props.channel.remark, 40)}
              </TooltipTrigger>
              <TooltipContent side='bottom' className='max-w-xs'>
                {props.channel.remark}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
      </div>
    </div>
  )
}

/**
 * Generate channels columns configuration
 */
export function useChannelsColumns(
  options: {
    enableSelection?: boolean
  } = {}
): ColumnDef<Channel>[] {
  const { t, i18n } = useTranslation()
  const { sensitiveVisible, quotaSummaryScope, setQuotaSummaryScope } =
    useChannels()
  const enableSelection = options.enableSelection ?? true
  const { data: passthrough } = useChannelPassthrough()
  const globalBodyPassthrough =
    passthrough?.data?.pass_through_request_enabled === true
  const globalHeaderPassthrough =
    passthrough?.data?.pass_through_headers_enabled === true
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // Stable columns and cell renderers prevent react-table from invalidating
  // the whole row model on unrelated parent renders.
  return useMemo<ColumnDef<Channel>[]>(
    () => [
      // Checkbox column
      ...(enableSelection
        ? [
            {
              id: 'select',
              header: ({ table }) => (
                <Checkbox
                  checked={table.getIsAllPageRowsSelected()}
                  indeterminate={table.getIsSomePageRowsSelected()}
                  onCheckedChange={(value) =>
                    table.toggleAllPageRowsSelected(!!value)
                  }
                  aria-label={t('Select all')}
                />
              ),
              cell: ({ row }) => {
                const isTagRow = isTagAggregateRow(row.original)

                // Don't show checkbox for tag rows
                if (isTagRow) {
                  return null
                }

                return (
                  <Checkbox
                    checked={row.getIsSelected()}
                    onCheckedChange={(value) => row.toggleSelected(!!value)}
                    aria-label={t('Select row')}
                  />
                )
              },
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              size: 40,
            } satisfies ColumnDef<Channel>,
          ]
        : []),

      // ID column
      {
        accessorKey: 'id',
        header: t('ID'),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const id = row.getValue('id') as number
          return <TableId value={sensitiveVisible ? id : SENSITIVE_MASK} />
        },
        size: 80,
      },
      // Name column (in table view it leads with the channel type logo)
      {
        accessorKey: 'name',
        header: t('Name'),
        meta: { mobileTitle: true },
        cell: ({ row }) => {
          const isTagRow = isTagAggregateRow(row.original)
          const name = row.getValue('name') as string

          // Tag row with expand/collapse
          if (isTagRow) {
            const tag = (row.original as TagRow).tag || name
            const childrenCount = (row.original as TagRow).children?.length || 0

            return (
              <div className='flex items-center gap-2'>
                <Button
                  variant='ghost'
                  size='sm'
                  className='h-6 w-6 p-0'
                  onClick={row.getToggleExpandedHandler()}
                >
                  {row.getIsExpanded() ? (
                    <ChevronDown className='h-4 w-4' />
                  ) : (
                    <ChevronRight className='h-4 w-4' />
                  )}
                </Button>
                <div className='flex items-center gap-1.5'>
                  <span className='font-semibold'>Tag：{tag}</span>
                  <StatusBadge
                    label={`${childrenCount} channels`}
                    variant='blue'
                    size='sm'
                    copyable={false}
                  />
                </div>
              </div>
            )
          }

          return (
            <ChannelNameCell
              channel={row.original}
              globalBodyPassthrough={globalBodyPassthrough}
              globalHeaderPassthrough={globalHeaderPassthrough}
            />
          )
        },
        size: 240,
        minSize: 160,
      },

      // Groups of a channel, editable from the cell. The cell's width is a
      // constant, so this column is exactly as wide with one group as with a
      // hundred. A tag row has no groups of its own. A new id keeps a
      // visibility state saved for the old "group" column from applying.
      {
        id: 'groups',
        accessorKey: 'group',
        header: t('Groups'),
        meta: { mobileHidden: true },
        cell: ({ row }) =>
          isTagAggregateRow(row.original) ? null : (
            <ChannelGroupsCell channel={row.original} />
          ),
        enableSorting: false,
        size: 160,
        // The cell's constant `w-24` plus the cell padding.
        minSize: 112,
      },

      // Channel type. Never shown as a table column (the name cell carries the
      // type logo there), but it still backs the type filter and renders the
      // type badge in the card header. A new id keeps a visibility state saved
      // for the old "type" column from bringing it back.
      {
        id: 'channel_type',
        accessorKey: 'type',
        header: t('Type'),
        cell: ({ row }) => {
          const isTagRow = isTagAggregateRow(row.original)

          if (isTagRow) {
            return (
              <StatusBadge
                label={t('Tag Aggregate')}
                variant='blue'
                size='sm'
                copyable={false}
                className='-ml-1.5'
              />
            )
          }

          const typeName = t(getChannelTypeLabel(row.original.type))
          const iconName = getChannelTypeIcon(row.original.type)

          return (
            <div className='flex max-w-full min-w-0 items-center gap-2 overflow-hidden'>
              <ChannelMultiKeyModeIcon channel={row.original} />
              <TooltipProvider delay={300}>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <div className='max-w-full min-w-0 overflow-hidden' />
                    }
                  >
                    <ProviderBadge
                      iconKey={`${iconName}.Color`}
                      iconSize={18}
                      label={typeName}
                      colorText={false}
                      copyable={false}
                      showDot={false}
                      className='max-w-full min-w-0 overflow-hidden'
                    />
                  </TooltipTrigger>
                  <TooltipContent side='top'>{typeName}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
              <ChannelIonetBadge channel={row.original} />
            </div>
          )
        },
        filterFn: (row, id, value) => {
          if (!value || value.length === 0 || value.includes('all')) {
            return true
          }
          return value.includes(String(row.getValue(id)))
        },
        enableSorting: false,
        enableHiding: false,
      },

      // Status and groups are shown by the row switch and the groups column;
      // these hidden columns only back the toolbar filters. New ids keep saved
      // visibility of the old columns from reviving them.
      {
        id: 'channel_status',
        accessorKey: 'status',
        header: t('Status'),
        enableSorting: false,
        enableHiding: false,
      },
      {
        id: 'channel_group',
        accessorKey: 'group',
        header: t('Groups'),
        enableSorting: false,
        enableHiding: false,
      },

      // Models column
      {
        accessorKey: 'models',
        header: t('Models'),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const models = row.getValue('models') as string
          const modelArray = parseModelsList(models)
          return (
            <BadgeListCell
              items={modelArray.map((model) => (
                <StatusBadge
                  key={model}
                  label={model}
                  autoColor={model}
                  size='sm'
                  className='font-mono'
                />
              ))}
            />
          )
        },
        size: 200,
        enableSorting: false,
      },

      // Tag column
      {
        accessorKey: 'tag',
        header: t('Tag'),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const tag = row.getValue('tag') as string | null
          if (!tag) {
            return <span className='text-muted-foreground text-xs'>-</span>
          }

          return (
            <StatusBadge
              label={tag}
              autoColor={tag}
              size='sm'
              className='-ml-1.5'
            />
          )
        },
        size: 120,
        enableSorting: false,
      },

      // Priority, weight and cost ratio as one inline-editable chip each. A
      // column of their own, right-aligned under a right-aligned header (see
      // the table's numeric column classes), so a chip never shares its cell
      // with its neighbours and never wraps. Keyed on priority so the
      // server-side priority sort keeps working.
      {
        accessorKey: 'priority',
        header: t('Priority'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <ChannelRoutingValueCell channel={row.original} field='priority' />
        ),
        // Room for the sort icon beside the title.
        size: 96,
        minSize: 80,
      },
      {
        id: 'weight',
        accessorKey: 'weight',
        header: t('Weight'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <ChannelRoutingValueCell channel={row.original} field='weight' />
        ),
        enableSorting: false,
        size: 72,
        minSize: 56,
      },
      {
        id: 'cost_ratio',
        accessorKey: 'cost_ratio',
        header: t('Cost ratio'),
        meta: { mobileHidden: true },
        cell: ({ row }) => (
          <ChannelRoutingValueCell channel={row.original} field='costRatio' />
        ),
        enableSorting: false,
        size: 92,
        minSize: 72,
      },

      // Cost, consumption and profit over the scope picked in the cost header
      // menu, one right-aligned column each. The table sizes each column, so
      // the figures of every row end on the same x without the cells
      // measuring each other.
      {
        id: 'quota_cost',
        header: () => (
          <ChannelQuotaSummaryHeader
            scope={quotaSummaryScope}
            onScopeChange={setQuotaSummaryScope}
          />
        ),
        cell: ({ row }) => (
          <ChannelQuotaValueCell
            channel={row.original}
            scope={quotaSummaryScope}
            field='cost'
          />
        ),
        // Room for the range menu icon beside the title.
        size: 120,
        minSize: 96,
        enableSorting: false,
        enableHiding: false,
      },
      {
        id: 'quota_used',
        header: getChannelQuotaColumnTitle('used', quotaSummaryScope, t),
        cell: ({ row }) => (
          <ChannelQuotaValueCell
            channel={row.original}
            scope={quotaSummaryScope}
            field='used'
          />
        ),
        size: 104,
        minSize: 80,
        enableSorting: false,
        enableHiding: false,
      },
      {
        id: 'quota_profit',
        header: getChannelQuotaColumnTitle('profit', quotaSummaryScope, t),
        cell: ({ row }) => (
          <ChannelQuotaValueCell
            channel={row.original}
            scope={quotaSummaryScope}
            field='profit'
          />
        ),
        size: 104,
        minSize: 80,
        enableSorting: false,
        enableHiding: false,
      },

      // Response Time column
      {
        accessorKey: 'response_time',
        header: t('Response'),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const responseTime = row.getValue('response_time') as number
          const config = getResponseTimeConfig(responseTime)

          // `max-w-none`: the badge is pulled left by its own padding, so the
          // table sizes the column from a box 6px narrower than the badge. A
          // `max-w-full` cap would then clip the tail ("24.15…") whenever the
          // column sits at its minimum width; uncapped, the badge still ends
          // inside the cell.
          return (
            <StatusBadge
              label={formatResponseTime(responseTime, t)}
              variant={config.variant}
              size='sm'
              copyable={false}
              className='-ml-1.5 max-w-none'
            />
          )
        },
        size: 110,
      },

      // Test Time column
      {
        accessorKey: 'test_time',
        header: t('Last Tested'),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const testTime = row.getValue('test_time') as number

          // For invalid timestamps, show "Never" badge
          if (!testTime || testTime === 0) {
            return <span className='text-muted-foreground text-xs'>-</span>
          }

          const timeText = formatRelativeTime(testTime, locale)
          const fullDate = formatTimestampToDate(testTime)

          // For valid timestamps, show tooltip with full date
          return (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger
                  render={
                    // Uncapped for the same reason as the response badge.
                    <StatusBadge
                      label={timeText}
                      variant='neutral'
                      size='sm'
                      copyable={false}
                      className='-ml-1.5 max-w-none cursor-pointer'
                    />
                  }
                />
                <TooltipContent side='top'>
                  <p className='font-mono text-sm'>{fullDate}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )
        },
        size: 120,
        enableSorting: false,
      },

      // Actions column
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => {
          // Check if this is a tag row (has children)
          const isTagRow = isTagAggregateRow(row.original)

          if (isTagRow) {
            return (
              <DataTableTagRowActions
                // eslint-disable-next-line @typescript-eslint/no-explicit-any
                row={row as any}
              />
            )
          }

          return <DataTableRowActions row={row} />
        },
        enableSorting: false,
        enableHiding: false,
        meta: { pinned: 'right' as const },
      },
    ],
    [
      enableSelection,
      t,
      locale,
      sensitiveVisible,
      quotaSummaryScope,
      setQuotaSummaryScope,
      globalBodyPassthrough,
      globalHeaderPassthrough,
    ]
  )
}
