import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'

import {
  MAX_GROUP_DESCRIPTION_LENGTH,
  type GroupMonitoringFormValues,
} from '../lib/form-schema'

export function MonitorConfigFields(props: {
  form: UseFormReturn<GroupMonitoringFormValues>
  index: number
  availableGroups: string[]
  availableModelsByGroup: Record<string, string[]>
  disabled: boolean
}) {
  const { t } = useTranslation()
  const monitor = props.form.watch(`groups.${props.index}`)
  const errors = props.form.formState.errors.groups?.[props.index]
  const prefix = `monitor-${monitor.id}`
  const allowList = monitor.visible_to_groups
  const sharedModels = (
    props.availableModelsByGroup[monitor.groups[0]] ?? []
  ).filter((model) =>
    monitor.groups.every((group) =>
      props.availableModelsByGroup[group]?.includes(model)
    )
  )
  const missingGroups = monitor.groups.filter(
    (group) => !props.availableGroups.includes(group)
  )
  const missingModels = monitor.models.filter(
    (model) => !sharedModels.includes(model)
  )
  const groupOptions = [
    ...new Set([...props.availableGroups, ...monitor.groups]),
  ]
  const modelOptions = [...new Set([...sharedModels, ...monitor.models])]
  const userGroupOptions = [
    ...new Set([...props.availableGroups, ...(allowList ?? [])]),
  ]
  let visibilityHint = t('Visible to all logged-in users')
  if (allowList?.length) {
    visibilityHint = t('Visible to administrators and the selected user groups')
  } else if (allowList) {
    visibilityHint = t('Visible to administrators only')
  }

  return (
    <div className='grid min-w-0 gap-4 p-4 lg:grid-cols-2'>
      <Field data-invalid={Boolean(errors?.name)}>
        <FieldLabel htmlFor={`${prefix}-name`}>{t('Monitor name')}</FieldLabel>
        <Input
          id={`${prefix}-name`}
          maxLength={120}
          aria-invalid={Boolean(errors?.name)}
          {...props.form.register(`groups.${props.index}.name`)}
        />
        <FieldDescription>
          {t('The public card title, independent of pricing group names.')}
        </FieldDescription>
        <FieldError errors={[errors?.name]} />
      </Field>
      <Field data-invalid={Boolean(errors?.description)}>
        <FieldLabel htmlFor={`${prefix}-description`}>
          {t('Description')}
        </FieldLabel>
        <Input
          id={`${prefix}-description`}
          maxLength={MAX_GROUP_DESCRIPTION_LENGTH}
          {...props.form.register(`groups.${props.index}.description`)}
        />
        <FieldError errors={[errors?.description]} />
      </Field>
      <Field data-invalid={Boolean(errors?.groups)}>
        <FieldLabel htmlFor={`${prefix}-groups`}>
          {t('Source groups')}
        </FieldLabel>
        <MultiSelect
          id={`${prefix}-groups`}
          disabled={props.disabled}
          options={groupOptions.map((group) => ({
            label: group,
            value: group,
          }))}
          selected={monitor.groups}
          maxVisibleChips={8}
          onChange={(values) =>
            props.form.setValue(`groups.${props.index}.groups`, values, {
              shouldDirty: true,
              shouldValidate: true,
            })
          }
          placeholder={t('Select source groups')}
          emptyText={t('No groups available')}
        />
        {missingGroups.length > 0 && (
          <FieldDescription className='text-destructive'>
            {t('Unavailable source groups: {{groups}}', {
              groups: missingGroups.join(', '),
            })}
          </FieldDescription>
        )}
        <FieldError errors={[errors?.groups]} />
      </Field>
      <Field data-invalid={Boolean(errors?.models)}>
        <FieldLabel htmlFor={`${prefix}-models`}>
          {t('Monitored models')}
        </FieldLabel>
        <MultiSelect
          id={`${prefix}-models`}
          disabled={props.disabled || monitor.groups.length === 0}
          options={modelOptions.map((model) => ({
            label: model,
            value: model,
          }))}
          selected={monitor.models}
          maxVisibleChips={8}
          onChange={(values) =>
            props.form.setValue(`groups.${props.index}.models`, values, {
              shouldDirty: true,
              shouldValidate: true,
            })
          }
          placeholder={t('Select models to monitor')}
          emptyText={t('No shared models available')}
        />
        <FieldDescription>
          {t(
            'Only models available in every selected source group can be added.'
          )}
        </FieldDescription>
        {missingModels.length > 0 && (
          <FieldDescription className='text-destructive'>
            {t('Models not shared by all source groups: {{models}}', {
              models: missingModels.join(', '),
            })}
          </FieldDescription>
        )}
        <FieldError errors={[errors?.models]} />
      </Field>
      <div className='space-y-3 border-t pt-4 lg:col-span-2'>
        <Field orientation='horizontal'>
          <Switch
            id={`${prefix}-restricted`}
            checked={allowList !== null}
            disabled={props.disabled}
            onCheckedChange={(checked) =>
              props.form.setValue(
                `groups.${props.index}.visible_to_groups`,
                checked ? [] : null,
                { shouldDirty: true, shouldValidate: true }
              )
            }
          />
          <FieldLabel htmlFor={`${prefix}-restricted`}>
            {t('Restrict visibility')}
          </FieldLabel>
        </Field>
        <Field data-invalid={Boolean(errors?.visible_to_groups)}>
          {allowList !== null && (
            <>
              <FieldLabel htmlFor={`${prefix}-visible`}>
                {t('Visible to user groups')}
              </FieldLabel>
              <MultiSelect
                id={`${prefix}-visible`}
                disabled={props.disabled}
                options={userGroupOptions.map((group) => ({
                  label: group,
                  value: group,
                }))}
                selected={allowList}
                maxVisibleChips={8}
                onChange={(values) =>
                  props.form.setValue(
                    `groups.${props.index}.visible_to_groups`,
                    values,
                    { shouldDirty: true, shouldValidate: true }
                  )
                }
                placeholder={t('Select user groups')}
                emptyText={t('No groups available')}
              />
            </>
          )}
          <FieldDescription>{visibilityHint}</FieldDescription>
          <FieldError errors={[errors?.visible_to_groups]} />
        </Field>
      </div>
    </div>
  )
}
