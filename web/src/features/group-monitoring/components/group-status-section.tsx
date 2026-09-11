/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import type { GroupMonitoringGroupSummary } from '../types'
import { ModelStatusCard } from './model-status-card'

/**
 * A monitored group renders as a section: the group name and its
 * administrator-provided description at the top, then one card per model.
 */
export function GroupStatusSection(props: {
  group: GroupMonitoringGroupSummary
  bucketMinutes: number
  windowHours: number
}) {
  const { t } = useTranslation()
  const issueCount = props.group.models.filter(
    (model) => model.state === 'down' || model.state === 'degraded'
  ).length

  return (
    <section className='space-y-3' aria-label={props.group.group_name}>
      <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
        <h2 className='min-w-0 truncate text-base font-medium'>
          {props.group.group_name}
        </h2>
        <span className='text-muted-foreground text-xs'>
          {t('{{count}} models', { count: props.group.models.length })}
        </span>
        {issueCount > 0 && (
          <Badge
            variant='outline'
            className='text-amber-600 dark:text-amber-400'
          >
            {t('{{count}} with issues', { count: issueCount })}
          </Badge>
        )}
      </div>
      {props.group.description && (
        <p className='text-muted-foreground text-sm'>
          {props.group.description}
        </p>
      )}
      {props.group.models.length === 0 ? (
        <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
          {t('No models configured for this group')}
        </div>
      ) : (
        <div className='space-y-3'>
          {props.group.models.map((model) => (
            <ModelStatusCard
              key={model.model_name}
              model={model}
              bucketMinutes={props.bucketMinutes}
              windowHours={props.windowHours}
            />
          ))}
        </div>
      )}
    </section>
  )
}
