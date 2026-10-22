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
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { getGroupMonitoringSummary } from './api'
import { GroupStatusSection } from './components/group-status-section'
import type { GroupMonitoringRange, GroupMonitoringSummary } from './types'

const SUMMARY_REFETCH_INTERVAL_MS = 60_000
const EMPTY_GROUPS: GroupMonitoringSummary['groups'] = []
const MONITOR_LIST_CLASS = 'grid min-w-0 grid-cols-1 gap-3'

export function GroupMonitoring() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isRoot = user?.role === ROLE.SUPER_ADMIN
  const [search, setSearch] = useState('')
  const [range, setRange] = useState<GroupMonitoringRange>('24h')
  const summaryQuery = useQuery({
    queryKey: ['group-monitoring', 'summary', range],
    queryFn: () => getGroupMonitoringSummary(range),
    refetchInterval: SUMMARY_REFETCH_INTERVAL_MS,
  })

  const allGroups = summaryQuery.data?.groups ?? EMPTY_GROUPS
  const visibleGroups = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase()
    if (!keyword) return allGroups
    return allGroups
      .map((group) => {
        if (
          `${group.group_name} ${group.description}`
            .toLocaleLowerCase()
            .includes(keyword)
        ) {
          return group
        }
        return {
          ...group,
          models: group.models.filter((model) =>
            model.model_name.toLocaleLowerCase().includes(keyword)
          ),
        }
      })
      .filter(
        (group) =>
          group.models.length > 0 ||
          `${group.group_name} ${group.description}`
            .toLocaleLowerCase()
            .includes(keyword)
      )
  }, [allGroups, search])

  let monitoringContent: ReactNode
  if (summaryQuery.isLoading) {
    monitoringContent = (
      <div className={MONITOR_LIST_CLASS}>
        {[0, 1, 2, 3].map((item) => (
          <Skeleton key={item} className='h-48 rounded-lg' />
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
  } else if (visibleGroups.length > 0 && summaryQuery.data) {
    const summary = summaryQuery.data
    monitoringContent = (
      <div
        role='list'
        aria-label={t('Group Monitoring')}
        className={MONITOR_LIST_CLASS}
      >
        {visibleGroups.map((group) => (
          <GroupStatusSection
            key={group.id}
            group={group}
            bucketMinutes={summary.bucket_minutes}
            windowHours={range === '24h' ? 24 : 72}
            range={range}
            thresholds={summary.thresholds}
          />
        ))}
      </div>
    )
  } else {
    let description = t('Monitoring targets have not been configured')
    if (search) {
      description = t('No monitors or models match your search')
    } else if (summaryQuery.data?.enabled === false) {
      description = t('Group monitoring is disabled')
    }
    monitoringContent = (
      <Empty className='min-h-64 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <Activity aria-hidden='true' />
          </EmptyMedia>
          <EmptyTitle>{t('No monitored models')}</EmptyTitle>
          <EmptyDescription>{description}</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Group Monitoring')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <NativeSelect
          aria-label={t('Monitoring range')}
          value={range}
          onChange={(event) =>
            setRange(event.target.value as GroupMonitoringRange)
          }
          className='w-auto min-w-36'
        >
          <NativeSelectOption value='24h'>
            {t('Last 24 hours')}
          </NativeSelectOption>
          <NativeSelectOption value='72h'>
            {t('Last 72 hours')}
          </NativeSelectOption>
        </NativeSelect>
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
          <div className='flex justify-end'>
            <div className='relative w-full sm:max-w-xs'>
              <Search
                className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2'
                aria-hidden='true'
              />
              <Input
                className='pl-8'
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t('Search monitors or models')}
                aria-label={t('Search monitors or models')}
              />
            </div>
          </div>
          {monitoringContent}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
