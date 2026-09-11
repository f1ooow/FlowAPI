/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { ChevronDown, ChevronRight, EyeOff } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'

import type {
  GroupMonitoringGroupSummary,
  GroupMonitoringThresholds,
} from '../types'
import { ModelStatusCard } from './model-status-card'

/**
 * A monitored group renders as one collapsible card: the group name and its
 * administrator-provided description in the header, then a responsive grid of
 * model cards inside.
 */
export function GroupStatusSection(props: {
  group: GroupMonitoringGroupSummary
  bucketMinutes: number
  windowHours: number
  thresholds: GroupMonitoringThresholds
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const issueCount = props.group.models.filter(
    (model) => model.state === 'down' || model.state === 'degraded'
  ).length

  return (
    <Card className='rounded-xl'>
      <Collapsible open={open} onOpenChange={setOpen}>
        <CardHeader className='grid grid-cols-[auto_minmax(0,1fr)_auto] items-start gap-3'>
          <CollapsibleTrigger
            render={<Button variant='ghost' size='icon-sm' />}
            aria-label={t('Toggle {{group}}', {
              group: props.group.group_name,
            })}
          >
            {open ? (
              <ChevronDown aria-hidden='true' />
            ) : (
              <ChevronRight aria-hidden='true' />
            )}
          </CollapsibleTrigger>
          <div className='min-w-0'>
            <h2 className='truncate text-base font-semibold'>
              {props.group.group_name}
            </h2>
            {props.group.description && (
              <p className='text-muted-foreground mt-0.5 text-xs'>
                {props.group.description}
              </p>
            )}
          </div>
          <div className='flex shrink-0 flex-wrap items-center justify-end gap-1.5'>
            {/* Only an administrator ever receives a hidden group. */}
            {!props.group.visible_to_users && (
              <Badge variant='outline' className='text-muted-foreground'>
                <EyeOff aria-hidden='true' />
                {t('Hidden from users')}
              </Badge>
            )}
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
        </CardHeader>
        <CollapsibleContent>
          <CardContent>
            {props.group.models.length === 0 ? (
              <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
                {t('No models configured for this group')}
              </div>
            ) : (
              <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
                {props.group.models.map((model) => (
                  <ModelStatusCard
                    key={model.model_name}
                    model={model}
                    bucketMinutes={props.bucketMinutes}
                    windowHours={props.windowHours}
                    thresholds={props.thresholds}
                  />
                ))}
              </div>
            )}
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  )
}
