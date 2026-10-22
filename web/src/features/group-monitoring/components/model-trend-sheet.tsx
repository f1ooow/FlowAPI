import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { CartesianGrid, Line, LineChart, Tooltip, XAxis, YAxis } from 'recharts'

import { ChartContainer, ChartTooltipContent } from '@/components/ui/chart'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

import type {
  GroupMonitoringModelSummary,
  GroupMonitoringRange,
} from '../types'

function formatTime(unixSeconds: number, range: GroupMonitoringRange) {
  return new Intl.DateTimeFormat(
    undefined,
    range === '24h'
      ? { hour: '2-digit', minute: '2-digit' }
      : { month: 'short', day: 'numeric', hour: '2-digit' }
  ).format(unixSeconds * 1000)
}

export function ModelTrendSheet(props: {
  model: GroupMonitoringModelSummary | null
  open: boolean
  onOpenChange: (open: boolean) => void
  range: GroupMonitoringRange
  bucketMinutes: number
}) {
  const { t } = useTranslation()
  const model = props.model
  const points = useMemo(
    () =>
      (props.model?.buckets ?? []).map((bucket) => ({
        time: formatTime(bucket.ts, props.range),
        successRate:
          bucket.request_count > 0
            ? (bucket.success_count / bucket.request_count) * 100
            : null,
        ttft: bucket.avg_ttft_ms,
        tps: bucket.avg_output_tps,
      })),
    [props.model, props.range]
  )

  const panels = [
    {
      key: 'successRate',
      title: t('Success rate'),
      unit: '%',
      color: 'var(--color-chart-1)',
      domain: [0, 100] as [number, number],
    },
    {
      key: 'ttft',
      title: t('Mean TTFT'),
      unit: t('ms'),
      color: 'var(--color-chart-2)',
    },
    {
      key: 'tps',
      title: t('Mean output speed'),
      unit: t('token/s'),
      color: 'var(--color-chart-3)',
    },
  ] as const

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent
        side='right'
        className='w-full gap-0 overflow-y-auto p-0 sm:max-w-2xl'
      >
        <SheetHeader className='border-b pr-14'>
          <SheetTitle className='font-mono'>
            {props.model?.model_name}
          </SheetTitle>
          <SheetDescription>
            {t(
              '{{hours}} hour trend. Bucket averages are shown as reported; gaps mean no samples.',
              { hours: props.range === '24h' ? 24 : 72 }
            )}
          </SheetDescription>
        </SheetHeader>
        <div className='space-y-6 p-4 sm:p-6'>
          {model && points.length > 0 ? (
            panels.map((panel) => (
              <section
                key={panel.key}
                aria-label={panel.title}
                className='space-y-2'
              >
                <h3 className='text-sm font-medium'>
                  {panel.title}{' '}
                  <span className='text-muted-foreground font-normal'>
                    ({panel.unit})
                  </span>
                </h3>
                <ChartContainer
                  className='aspect-auto h-44 w-full'
                  config={{
                    [panel.key]: { label: panel.title, color: panel.color },
                  }}
                >
                  <LineChart
                    data={points}
                    title={`${panel.title} trend for ${model.model_name}`}
                    margin={{ left: 4, right: 12, top: 8, bottom: 0 }}
                  >
                    <CartesianGrid vertical={false} strokeDasharray='3 3' />
                    <XAxis
                      dataKey='time'
                      minTickGap={32}
                      tickLine={false}
                      axisLine={false}
                    />
                    <YAxis
                      width={42}
                      tickLine={false}
                      axisLine={false}
                      domain={
                        'domain' in panel ? panel.domain : ['auto', 'auto']
                      }
                    />
                    <Tooltip
                      content={<ChartTooltipContent indicator='line' />}
                    />
                    <Line
                      dataKey={panel.key}
                      type='linear'
                      stroke={panel.color}
                      strokeWidth={2}
                      dot={false}
                      connectNulls={false}
                      isAnimationActive={false}
                    />
                  </LineChart>
                </ChartContainer>
              </section>
            ))
          ) : (
            <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
              {t(
                'No trend samples are available for this model and time range.'
              )}
            </div>
          )}
          {points.length > 0 && (
            <p className='text-muted-foreground text-xs'>
              {t(
                'Each point covers {{minutes}} minutes. This is not a live probe.',
                { minutes: props.bucketMinutes }
              )}
            </p>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
