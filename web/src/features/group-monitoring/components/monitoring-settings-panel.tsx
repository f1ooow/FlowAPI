/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the License,
or (at your option) any later version.
*/

import { zodResolver } from '@hookform/resolvers/zod'
import { Save } from 'lucide-react'
import { useEffect } from 'react'
import { useFieldArray, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'

import {
  groupMonitoringFormSchema,
  MAX_GROUP_DESCRIPTION_LENGTH,
  type GroupMonitoringFormValues,
} from '../lib/form-schema'
import {
  GROUP_MONITORING_BUCKET_MINUTE_OPTIONS,
  type GroupMonitoringBucketMinutes,
  type GroupMonitoringSetting,
} from '../types'

type MonitoringSettingsPanelProps = {
  availableGroups: string[]
  availableModelsByGroup: Record<string, string[]>
  /**
   * Physical perf_metrics bucket width. A timeline bucket narrower than this
   * cannot be produced, so those options are disabled.
   */
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
    : GROUP_MONITORING_BUCKET_MINUTE_OPTIONS[0]
  return {
    enabled: setting.enabled,
    bucket_minutes: bucketMinutes,
    groups: (setting.groups ?? []).map((group) => ({
      group: group.group,
      description: group.description ?? '',
      // Keep null as null: it is "visible to everyone", which an empty array
      // would silently turn into "administrators only".
      visible_to_groups: group.visible_to_groups
        ? [...group.visible_to_groups]
        : null,
      models: [...(group.models ?? [])],
    })),
  }
}

export function MonitoringSettingsPanel(props: MonitoringSettingsPanelProps) {
  const { t } = useTranslation()
  const form = useForm<GroupMonitoringFormValues>({
    resolver: zodResolver(groupMonitoringFormSchema),
    defaultValues: toFormValues(props.setting),
  })
  const groups = useFieldArray({ control: form.control, name: 'groups' })
  const watchedGroups = form.watch('groups')

  useEffect(() => {
    form.reset(toFormValues(props.setting))
  }, [form, props.setting])

  // A group that was configured before it disappeared from the group ratio
  // settings would otherwise be invisible here while still being submitted,
  // and the API rejects unknown groups. Listing it keeps it uncheckable-away.
  const listedGroups = [
    ...props.availableGroups,
    ...watchedGroups
      .map((group) => group.group)
      .filter((group) => !props.availableGroups.includes(group)),
  ]

  const toggleGroup = (groupName: string, monitored: boolean) => {
    const current = form.getValues('groups')
    const index = current.findIndex((group) => group.group === groupName)
    if (!monitored) {
      if (index >= 0) groups.remove(index)
      return
    }
    if (index >= 0) return
    // Keep the stored order aligned with the order shown here, which is also
    // the order the monitoring page renders the sections in.
    const position = listedGroups.indexOf(groupName)
    const insertAt = current.filter(
      (group) => listedGroups.indexOf(group.group) < position
    ).length
    groups.insert(insertAt, {
      group: groupName,
      description: '',
      visible_to_groups: null,
      models: props.availableModelsByGroup[groupName]?.slice(0, 1) ?? [],
    })
  }

  return (
    <form
      className='space-y-6'
      onSubmit={form.handleSubmit((values) => props.onSave(values))}
    >
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
              'Each timeline bar covers this much time. Buckets narrower than the global perf metrics bucket ({{minutes}} minutes) cannot be served and are disabled.',
              { minutes: props.storageBucketMinutes }
            )}
          </FieldDescription>
          <FieldError errors={[form.formState.errors.bucket_minutes]} />
        </Field>
        <Field orientation='horizontal' className='min-h-9 sm:pt-7'>
          <Switch
            id='monitoring-enabled'
            checked={form.watch('enabled')}
            onCheckedChange={(checked) =>
              form.setValue('enabled', checked, { shouldDirty: true })
            }
          />
          <FieldLabel
            htmlFor='monitoring-enabled'
            className='whitespace-nowrap'
          >
            {t('Enable monitoring')}
          </FieldLabel>
        </Field>
      </div>

      <div className='space-y-3'>
        <div>
          <FieldLabel>{t('Monitored groups')}</FieldLabel>
          <FieldDescription>
            {t(
              'Pick the groups to show on the monitoring page, then choose which models to track inside each one.'
            )}
          </FieldDescription>
          <FieldDescription>
            {t(
              'Administrators always see every monitored group, regardless of the visibility restrictions below.'
            )}
          </FieldDescription>
        </div>

        {listedGroups.length === 0 ? (
          <div className='text-muted-foreground rounded-lg border border-dashed p-5 text-center text-sm'>
            {t('No groups available')}
          </div>
        ) : (
          <div className='space-y-3'>
            {listedGroups.map((groupName) => {
              const index = watchedGroups.findIndex(
                (group) => group.group === groupName
              )
              const monitored = index >= 0
              const models = props.availableModelsByGroup[groupName] ?? []
              const groupErrors = form.formState.errors.groups?.[index]
              // null is "no restriction"; an array (even an empty one) means
              // the allow list decides.
              const allowList = watchedGroups[index]?.visible_to_groups ?? null
              // A user group that disappeared from the group ratio settings is
              // still listed while selected, otherwise it could never be
              // removed and every save would be rejected as unknown.
              const userGroupOptions = [
                ...props.availableGroups,
                ...(allowList ?? []).filter(
                  (group) => !props.availableGroups.includes(group)
                ),
              ]
              let visibilityHint = t('Visible to all logged-in users')
              if (allowList?.length) {
                visibilityHint = t(
                  'Visible to administrators and the selected user groups'
                )
              } else if (allowList) {
                visibilityHint = t('Visible to administrators only')
              }

              return (
                <div key={groupName} className='rounded-lg border p-3'>
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <div className='flex min-w-0 items-center gap-2'>
                      <Checkbox
                        id={`monitoring-group-${groupName}`}
                        checked={monitored}
                        onCheckedChange={(checked) =>
                          toggleGroup(groupName, checked === true)
                        }
                      />
                      <FieldLabel
                        htmlFor={`monitoring-group-${groupName}`}
                        className='min-w-0 truncate'
                      >
                        {groupName}
                      </FieldLabel>
                    </div>
                    {monitored && (
                      <div className='flex items-center gap-2'>
                        <Switch
                          id={`monitoring-restricted-${groupName}`}
                          checked={allowList !== null}
                          onCheckedChange={(checked) =>
                            form.setValue(
                              `groups.${index}.visible_to_groups`,
                              // Turning the restriction on starts from an empty
                              // allow list, which is "administrators only".
                              checked ? [] : null,
                              { shouldDirty: true, shouldValidate: true }
                            )
                          }
                        />
                        <FieldLabel
                          htmlFor={`monitoring-restricted-${groupName}`}
                          className='text-muted-foreground font-normal'
                        >
                          {t('Restrict visibility')}
                        </FieldLabel>
                      </div>
                    )}
                  </div>

                  {monitored && (
                    <div className='mt-3 space-y-3'>
                      <div className='grid gap-3 lg:grid-cols-2'>
                        <Field data-invalid={Boolean(groupErrors?.models)}>
                          <FieldLabel
                            htmlFor={`monitoring-models-${groupName}`}
                          >
                            {t('Monitored models')}
                          </FieldLabel>
                          <MultiSelect
                            id={`monitoring-models-${groupName}`}
                            options={models.map((model) => ({
                              label: model,
                              value: model,
                            }))}
                            selected={watchedGroups[index]?.models ?? []}
                            onChange={(values) =>
                              form.setValue(`groups.${index}.models`, values, {
                                shouldDirty: true,
                                shouldValidate: true,
                              })
                            }
                            placeholder={t('Select models to monitor')}
                            emptyText={t('No models available')}
                            maxVisibleChips={8}
                          />
                          <FieldError errors={[groupErrors?.models]} />
                        </Field>

                        <Field data-invalid={Boolean(groupErrors?.description)}>
                          <FieldLabel
                            htmlFor={`monitoring-description-${groupName}`}
                          >
                            {t('Description')}
                          </FieldLabel>
                          <Input
                            id={`monitoring-description-${groupName}`}
                            maxLength={MAX_GROUP_DESCRIPTION_LENGTH}
                            placeholder={t(
                              'Shown above this group on the monitoring page'
                            )}
                            {...form.register(`groups.${index}.description`)}
                          />
                          <FieldError errors={[groupErrors?.description]} />
                        </Field>
                      </div>

                      <Field
                        data-invalid={Boolean(groupErrors?.visible_to_groups)}
                      >
                        {allowList !== null && (
                          <>
                            <FieldLabel
                              htmlFor={`monitoring-visible-groups-${groupName}`}
                            >
                              {t('Visible to user groups')}
                            </FieldLabel>
                            <MultiSelect
                              id={`monitoring-visible-groups-${groupName}`}
                              options={userGroupOptions.map((group) => ({
                                label: group,
                                value: group,
                              }))}
                              selected={allowList}
                              onChange={(values) =>
                                form.setValue(
                                  `groups.${index}.visible_to_groups`,
                                  values,
                                  { shouldDirty: true, shouldValidate: true }
                                )
                              }
                              placeholder={t('Select user groups')}
                              emptyText={t('No groups available')}
                              maxVisibleChips={8}
                            />
                          </>
                        )}
                        <FieldDescription>{visibilityHint}</FieldDescription>
                        <FieldError
                          errors={[groupErrors?.visible_to_groups?.root]}
                        />
                      </Field>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}
        <FieldError errors={[form.formState.errors.groups?.root]} />
      </div>

      <div className='flex justify-end border-t pt-4'>
        <Button type='submit' disabled={props.saving}>
          <Save aria-hidden='true' />
          {props.saving ? t('Saving...') : t('Save settings')}
        </Button>
      </div>
    </form>
  )
}
