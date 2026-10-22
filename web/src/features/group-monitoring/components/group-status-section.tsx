/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { EyeOff } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import type {
  GroupMonitoringGroupSummary,
  GroupMonitoringModelSummary,
  GroupMonitoringRange,
  GroupMonitoringThresholds,
} from '../types'
import { ModelStatusCard } from './model-status-card'
import { ModelTrendSheet } from './model-trend-sheet'

function formatRate(value: number, hasData: boolean) {
  return hasData ? `${value.toFixed(2)}%` : '—'
}

export function GroupStatusSection(props: {
  group: GroupMonitoringGroupSummary
  bucketMinutes: number
  windowHours: number
  range: GroupMonitoringRange
  thresholds: GroupMonitoringThresholds
}) {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = (user?.role ?? 0) >= ROLE.ADMIN
  const [selectedModel, setSelectedModel] =
    useState<GroupMonitoringModelSummary | null>(null)
  const issueCount = props.group.models.filter(
    (model) => model.state === 'down' || model.state === 'degraded'
  ).length
  const sources = props.group.source_groups
  let visibilityBadge = ''
  if (props.group.visible_to_groups?.length) {
    visibilityBadge = t('Visible to {{groups}}', {
      groups: props.group.visible_to_groups.join(', '),
    })
  } else if (props.group.visible_to_groups) {
    visibilityBadge = t('Administrators only')
  }

  return (
    <Card role='listitem' className='@container min-w-0 rounded-xl'>
      <CardHeader className='space-y-2 border-b pb-3'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div className='min-w-0 space-y-1'>
            <h2 className='text-base font-semibold [overflow-wrap:anywhere]'>
              {props.group.group_name}
            </h2>
            {props.group.description && (
              <p className='text-muted-foreground text-sm'>
                {props.group.description}
              </p>
            )}
            {isAdmin && sources && sources.length > 0 && (
              <p className='text-muted-foreground text-xs'>
                {t('Sources: {{groups}}', { groups: sources.join(', ') })}
              </p>
            )}
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            {isAdmin && visibilityBadge && (
              <Badge variant='outline' className='text-muted-foreground'>
                <EyeOff aria-hidden='true' />
                {visibilityBadge}
              </Badge>
            )}
            <Badge variant='outline' className='font-mono tabular-nums'>
              {formatRate(
                props.group.availability_rate,
                props.group.request_count > 0
              )}{' '}
              {t('success')}
            </Badge>
            <Badge
              variant='outline'
              className='text-muted-foreground tabular-nums'
            >
              {t('{{count}} requests', {
                count: (props.group.request_count ?? 0).toLocaleString(),
              })}
            </Badge>
            <Badge variant='outline' className='text-muted-foreground'>
              {t('{{count}} models', { count: props.group.models.length })}
            </Badge>
            {issueCount > 0 && (
              <Badge
                variant='outline'
                className='border-red-500/40 text-red-600 dark:text-red-400'
              >
                {t('{{count}} with issues', { count: issueCount })}
              </Badge>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent
        role='list'
        aria-label={t('Models')}
        className='grid gap-3 px-4 pt-0 pb-4 @md:grid-cols-2'
      >
        {props.group.models.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm @md:col-span-2'>
            {t('No models configured for this monitor')}
          </div>
        ) : (
          props.group.models.map((model) => (
            <ModelStatusCard
              key={model.model_name}
              model={model}
              bucketMinutes={props.bucketMinutes}
              windowHours={props.windowHours}
              thresholds={props.thresholds}
              onOpenDetails={setSelectedModel}
            />
          ))
        )}
      </CardContent>
      <ModelTrendSheet
        model={selectedModel}
        open={selectedModel !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedModel(null)
        }}
        range={props.range}
        bucketMinutes={props.bucketMinutes}
      />
    </Card>
  )
}
