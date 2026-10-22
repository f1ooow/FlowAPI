/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/

import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowDown, ArrowUp, Plus, Save, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'

import {
  groupMonitoringFormSchema,
  MAX_MONITORED_GROUPS,
  type GroupMonitoringFormValues,
} from '../lib/form-schema'
import {
  GROUP_MONITORING_BUCKET_MINUTE_OPTIONS,
  type GroupMonitoringBucketMinutes,
  type GroupMonitoringSetting,
} from '../types'
import { MonitorConfigFields } from './monitor-config-fields'

type MonitoringSettingsPanelProps = {
  availableGroups: string[]
  availableModelsByGroup: Record<string, string[]>
  storageBucketMinutes: number
  setting: GroupMonitoringSetting
  saving: boolean
  onSave: (setting: GroupMonitoringSetting) => void
}

function toFormValues(
  setting: GroupMonitoringSetting
): GroupMonitoringFormValues {
  const bucketMinutes = GROUP_MONITORING_BUCKET_MINUTE_OPTIONS.includes(
    setting.bucket_minutes as GroupMonitoringBucketMinutes
  )
    ? (setting.bucket_minutes as GroupMonitoringBucketMinutes)
    : 5
  return { ...setting, bucket_minutes: bucketMinutes }
}

export function MonitoringSettingsPanel(props: MonitoringSettingsPanelProps) {
  const { t } = useTranslation()
  const form = useForm<GroupMonitoringFormValues>({
    resolver: zodResolver(groupMonitoringFormSchema),
    defaultValues: toFormValues(props.setting),
  })
  const monitors = useFieldArray({
    control: form.control,
    name: 'groups',
    keyName: 'formKey',
  })
  const [deleteIndex, setDeleteIndex] = useState<number | null>(null)
  const watched = form.watch('groups')

  useEffect(() => {
    form.reset(toFormValues(props.setting))
  }, [form, props.setting])

  const save = (values: GroupMonitoringFormValues) => {
    let invalid = false
    values.groups.forEach((monitor, index) => {
      if (
        monitor.groups.some((group) => !props.availableGroups.includes(group))
      ) {
        form.setError(`groups.${index}.groups`, {
          message: t('Remove unavailable source groups before saving'),
        })
        invalid = true
      }
      if (
        monitor.models.some((model) =>
          monitor.groups.some(
            (group) => !props.availableModelsByGroup[group]?.includes(model)
          )
        )
      ) {
        form.setError(`groups.${index}.models`, {
          message: t('Choose models available in every source group'),
        })
        invalid = true
      }
      if (
        monitor.visible_to_groups?.some(
          (group) => !props.availableGroups.includes(group)
        )
      ) {
        form.setError(`groups.${index}.visible_to_groups`, {
          message: t('Remove unavailable user groups before saving'),
        })
        invalid = true
      }
    })
    if (!invalid) props.onSave(values)
  }

  return (
    <form onSubmit={form.handleSubmit(save)}>
      <fieldset disabled={props.saving} className='min-w-0 space-y-6'>
        <div className='grid gap-4 sm:grid-cols-[minmax(0,20rem)_auto] sm:items-start'>
          <Field data-invalid={Boolean(form.formState.errors.bucket_minutes)}>
            <FieldLabel htmlFor='monitoring-bucket-minutes'>
              {t('Timeline bucket')}
            </FieldLabel>
            <NativeSelect
              id='monitoring-bucket-minutes'
              value={String(form.watch('bucket_minutes'))}
              onChange={(event) =>
                form.setValue(
                  'bucket_minutes',
                  Number(event.target.value) as GroupMonitoringBucketMinutes,
                  { shouldDirty: true, shouldValidate: true }
                )
              }
            >
              {GROUP_MONITORING_BUCKET_MINUTE_OPTIONS.map((minutes) => (
                <NativeSelectOption
                  key={minutes}
                  value={String(minutes)}
                  disabled={minutes < props.storageBucketMinutes}
                >
                  {t('{{minutes}} minutes', { minutes })}
                </NativeSelectOption>
              ))}
            </NativeSelect>
            <FieldDescription>
              {t(
                'Timeline resolution cannot be finer than storage ({{minutes}} minutes).',
                { minutes: props.storageBucketMinutes }
              )}
            </FieldDescription>
            <FieldError errors={[form.formState.errors.bucket_minutes]} />
          </Field>
          <Field orientation='horizontal' className='min-h-9 sm:pt-7'>
            <Switch
              id='monitoring-enabled'
              checked={form.watch('enabled')}
              disabled={props.saving}
              onCheckedChange={(checked) =>
                form.setValue('enabled', checked, { shouldDirty: true })
              }
            />
            <FieldLabel htmlFor='monitoring-enabled'>
              {t('Enable monitoring')}
            </FieldLabel>
          </Field>
        </div>

        <div className='space-y-4'>
          <div className='flex flex-wrap items-start justify-between gap-3'>
            <div className='space-y-1'>
              <h3 className='font-medium'>{t('Monitors')}</h3>
              <p className='text-muted-foreground max-w-2xl text-sm'>
                {t(
                  'Give each monitor a name, combine source groups, and select their shared models. Source groups are never shown to regular users.'
                )}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              disabled={
                props.saving || monitors.fields.length >= MAX_MONITORED_GROUPS
              }
              onClick={() =>
                monitors.append({
                  id: crypto.randomUUID(),
                  name: '',
                  groups: [],
                  models: [],
                  description: '',
                  visible_to_groups: null,
                })
              }
            >
              <Plus aria-hidden='true' />
              {t('Add monitor')}
            </Button>
          </div>
          {monitors.fields.length === 0 && (
            <p className='text-muted-foreground rounded-xl border border-dashed p-6 text-center text-sm'>
              {t(
                'No monitors configured. Add a monitor to combine related groups.'
              )}
            </p>
          )}
          {monitors.fields.map((field, index) => (
            <section key={field.formKey} className='min-w-0 rounded-xl border'>
              <div className='bg-muted/30 flex items-center justify-between gap-3 rounded-t-xl border-b px-4 py-3'>
                <h4 className='min-w-0 font-medium [overflow-wrap:anywhere]'>
                  {watched[index]?.name || t('New monitor')}
                </h4>
                <div className='flex shrink-0 gap-1'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={props.saving || index === 0}
                    aria-label={t('Move monitor up')}
                    onClick={() => monitors.move(index, index - 1)}
                  >
                    <ArrowUp aria-hidden='true' />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={
                      props.saving || index === monitors.fields.length - 1
                    }
                    aria-label={t('Move monitor down')}
                    onClick={() => monitors.move(index, index + 1)}
                  >
                    <ArrowDown aria-hidden='true' />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={props.saving}
                    aria-label={t('Delete monitor')}
                    onClick={() => setDeleteIndex(index)}
                  >
                    <Trash2 aria-hidden='true' />
                  </Button>
                </div>
              </div>
              <MonitorConfigFields
                form={form}
                index={index}
                availableGroups={props.availableGroups}
                availableModelsByGroup={props.availableModelsByGroup}
                disabled={props.saving}
              />
            </section>
          ))}
          <FieldError errors={[form.formState.errors.groups?.root]} />
        </div>
        <div className='flex justify-end border-t pt-4'>
          <Button type='submit' disabled={props.saving}>
            <Save aria-hidden='true' />
            {props.saving ? t('Saving...') : t('Save settings')}
          </Button>
        </div>
      </fieldset>
      <AlertDialog
        open={deleteIndex !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteIndex(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete monitor?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This removes the monitor configuration only. Historical request data is not deleted.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('Cancel')}</AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              onClick={() => {
                if (deleteIndex !== null) monitors.remove(deleteIndex)
                setDeleteIndex(null)
              }}
            >
              {t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </form>
  )
}
