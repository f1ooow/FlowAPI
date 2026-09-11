/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'

import {
  getGroupMonitoringAdmin,
  updateGroupMonitoringAdmin,
} from '../../group-monitoring/api'
import { MonitoringSettingsPanel } from '../../group-monitoring/components/monitoring-settings-panel'
import type { GroupMonitoringSetting } from '../../group-monitoring/types'
import { SettingsSection } from '../components/settings-section'

export function GroupMonitoringSettingsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ['group-monitoring', 'admin'],
    queryFn: getGroupMonitoringAdmin,
  })
  const mutation = useMutation({
    mutationFn: (setting: GroupMonitoringSetting) =>
      updateGroupMonitoringAdmin(setting),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: ['group-monitoring', 'admin'],
        }),
        queryClient.invalidateQueries({
          queryKey: ['group-monitoring', 'summary'],
        }),
      ])
      toast.success(t('Monitoring settings saved'))
    },
    onError: () => toast.error(t('Failed to save monitoring settings')),
  })
  const settingsUnavailable = query.isError || !query.data
  let settingsContent: ReactNode
  if (query.isLoading) {
    settingsContent = <Skeleton className='h-24 rounded-xl' />
  } else if (settingsUnavailable) {
    settingsContent = (
      <div className='flex items-center justify-between gap-3 rounded-xl border p-4'>
        <span className='text-muted-foreground inline-flex items-center gap-2 text-sm'>
          <Activity className='size-4' aria-hidden='true' />
          {t('Failed to load group monitoring settings')}
        </span>
        <Button variant='outline' size='sm' onClick={() => query.refetch()}>
          {t('Retry')}
        </Button>
      </div>
    )
  } else {
    const data = query.data
    settingsContent = (
      <MonitoringSettingsPanel
        availableGroups={data.available_groups}
        availableModelsByGroup={data.available_models_by_group}
        storageBucketMinutes={data.storage_bucket_minutes}
        setting={data.setting}
        saving={mutation.isPending}
        onSave={(setting) => mutation.mutate(setting)}
      />
    )
  }

  return (
    <SettingsSection title={t('Group availability monitoring')}>
      <div className='space-y-4'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Availability is aggregated from real user traffic over the last 24 hours, latency over the last hour. No probe requests are sent, so a model without calls in the window shows as no data.'
          )}
        </p>
        {settingsContent}
      </div>
    </SettingsSection>
  )
}
