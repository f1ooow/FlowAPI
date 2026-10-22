/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import type {
  GroupMonitoringGroupSummary,
  GroupMonitoringModelSummary,
  GroupMonitoringRange,
  GroupMonitoringThresholds,
} from '../types'
import { ModelStatusCard } from './model-status-card'
import { ModelTrendSheet } from './model-trend-sheet'

export function GroupStatusSection(props: {
  group: GroupMonitoringGroupSummary
  bucketMinutes: number
  windowHours: number
  range: GroupMonitoringRange
  thresholds: GroupMonitoringThresholds
}) {
  const { t } = useTranslation()
  const [selectedModel, setSelectedModel] =
    useState<GroupMonitoringModelSummary | null>(null)

  return (
    <section role='listitem' className='min-w-0 border-b pb-5 last:border-b-0'>
      <header className='min-w-0 pb-3'>
        <h2 className='text-base font-semibold [overflow-wrap:anywhere]'>
          {props.group.group_name}
        </h2>
        {props.group.description && (
          <p className='text-muted-foreground mt-1 text-sm'>
            {props.group.description}
          </p>
        )}
      </header>
      <div
        role='list'
        aria-label={t('Models')}
        className='grid min-w-0 gap-x-6 gap-y-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'
      >
        {props.group.models.length === 0 ? (
          <div className='text-muted-foreground border-t border-dashed pt-4 text-sm sm:col-span-2 lg:col-span-3 xl:col-span-4'>
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
      </div>
      <ModelTrendSheet
        model={selectedModel}
        open={selectedModel !== null}
        onOpenChange={(open) => {
          if (!open) setSelectedModel(null)
        }}
        range={props.range}
        bucketMinutes={props.bucketMinutes}
      />
    </section>
  )
}
