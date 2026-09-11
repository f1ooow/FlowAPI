/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Activity, Search, Settings2 } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getGroupMonitoringSummary } from './api'
import { GroupStatusSection } from './components/group-status-section'

// perf_metrics flushes every few minutes, so anything faster than a minute only
// re-renders identical data.
const SUMMARY_REFETCH_INTERVAL_MS = 60_000

export function GroupMonitoring() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === ROLE.SUPER_ADMIN
  const [search, setSearch] = useState('')

  const summaryQuery = useQuery({
    queryKey: ['group-monitoring', 'summary'],
    queryFn: getGroupMonitoringSummary,
    refetchInterval: SUMMARY_REFETCH_INTERVAL_MS,
  })

  const allGroups = useMemo(
    () => summaryQuery.data?.groups ?? [],
    [summaryQuery.data]
  )

  const visibleGroups = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase()
    if (!keyword) return allGroups
    return allGroups
      .map((group) => {
        if (group.group_name.toLocaleLowerCase().includes(keyword)) return group
        return {
          ...group,
          models: group.models.filter((model) =>
            model.model_name.toLocaleLowerCase().includes(keyword)
          ),
        }
      })
      .filter((group) => group.models.length > 0)
  }, [allGroups, search])

  const allModels = allGroups.flatMap((group) => group.models)
  const monitoredCount = allModels.length
  const operationalCount = allModels.filter(
    (model) => model.state === 'healthy'
  ).length
  const issueCount = allModels.filter(
    (model) => model.state === 'down' || model.state === 'degraded'
  ).length

  let monitoringContent: ReactNode
  if (summaryQuery.isLoading) {
    monitoringContent = (
      <div className='space-y-3'>
        {[0, 1, 2].map((item) => (
          <Skeleton key={item} className='h-36 rounded-lg' />
        ))}
      </div>
    )
  } else if (summaryQuery.isError) {
    monitoringContent = (
      <Empty className='min-h-64 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <Activity aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load monitoring data')}</EmptyTitle>
          <EmptyDescription>
            <Button
              variant='outline'
              size='sm'
              onClick={() => summaryQuery.refetch()}
            >
              {t('Retry')}
            </Button>
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else if (visibleGroups.length > 0) {
    monitoringContent = (
      <div className='space-y-8'>
        {visibleGroups.map((group) => (
          <GroupStatusSection
            key={group.group_name}
            group={group}
            bucketMinutes={summaryQuery.data?.bucket_minutes ?? 5}
            windowHours={summaryQuery.data?.window_hours ?? 24}
          />
        ))}
      </div>
    )
  } else {
    let emptyDescription = t('Monitoring targets have not been configured')
    if (search) {
      emptyDescription = t('No groups or models match your search')
    } else if (summaryQuery.data?.enabled === false) {
      emptyDescription = t('Group monitoring is disabled')
    }
    monitoringContent = (
      <Empty className='min-h-64 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <Activity aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No monitored models')}</EmptyTitle>
          <EmptyDescription>{emptyDescription}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-3'>
          <span className='truncate'>{t('Group Monitoring')}</span>
          <span className='text-muted-foreground hidden items-center gap-3 text-xs font-normal sm:inline-flex'>
            <span className='text-emerald-600 dark:text-emerald-400'>
              {t('{{count}} operational', { count: operationalCount })}
            </span>
            <span>{t('{{count}} with issues', { count: issueCount })}</span>
            <span>{t('{{count}} monitored', { count: monitoredCount })}</span>
          </span>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        {isRoot && (
          <Button
            variant='outline'
            size='sm'
            render={
              <Link
                to='/system-settings/models/$section'
                params={{ section: 'group-monitoring' }}
              />
            }
          >
            <Settings2 aria-hidden='true' />
            {t('Configure monitoring')}
          </Button>
        )}
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'Built from real traffic over the last {{hours}} hours. A model with no calls in the window shows as no data, not as an outage.',
                { hours: summaryQuery.data?.window_hours ?? 24 }
              )}
            </p>
            <div className='relative w-full sm:max-w-xs'>
              <Search
                className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2'
                aria-hidden='true'
              />
              <Input
                className='pl-8'
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t('Search groups or models')}
                aria-label={t('Search groups or models')}
              />
            </div>
          </div>

          {monitoringContent}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
