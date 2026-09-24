import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { FieldGroup, FieldSet, FieldLegend } from '@/components/ui/field'
import { FormControl, FormDescription, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import type { ChannelFormValues } from '../../../lib/channel-form'

export function ChannelMediaSettings(props: { channelType: number }) {
  const { t } = useTranslation()
  const form = useFormContext<ChannelFormValues>()
  if (![1, 17, 35, 45, 54, 55, 60, 100, 101, 102, 103].includes(props.channelType)) return null

  return (
    <FieldSet className='px-4 py-3'>
      <FieldLegend>{t('Media provider settings')}</FieldLegend>
      <FieldGroup>
        {[1, 55, 60].includes(props.channelType) && (
          <FormField control={form.control} name='video_protocol' render={({ field }) => (
            <FormItem className='flex items-center justify-between gap-4'>
              <div>
                <FormLabel>{t('Rolldek Wan protocol')}</FormLabel>
                <FormDescription>{t('Enable only for a provider that supports wan-rolldek-v1.')}</FormDescription>
              </div>
              <FormControl><Switch checked={field.value === 'wan-rolldek-v1'} onCheckedChange={(checked) => field.onChange(checked ? 'wan-rolldek-v1' : '')} /></FormControl>
            </FormItem>
          )} />
        )}
        {[17, 54, 100, 101, 102, 103].includes(props.channelType) && (
          <>
            <FormField control={form.control} name='task_submit_path' render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Task submit path')}</FormLabel>
                <FormControl><Input {...field} value={field.value || ''} placeholder='/v1/services/aigc/video-generation/video-synthesis' /></FormControl>
                <FormDescription>{t('Leave blank to use the provider default. Paths must stay on the configured host.')}</FormDescription>
                <FormMessage />
              </FormItem>
            )} />
            <FormField control={form.control} name='task_fetch_path' render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Task query path')}</FormLabel>
                <FormControl><Input {...field} value={field.value || ''} placeholder='/api/v1/tasks/{task_id}' /></FormControl>
                <FormDescription>{t('Use a same-origin path with {task_id} exactly once.')}</FormDescription>
                <FormMessage />
              </FormItem>
            )} />
          </>
        )}
        {props.channelType === 101 && (
          <FormField control={form.control} name='other_info' render={({ field }) => (
            <FormItem>
              <FormLabel>{t('VOD account configuration (JSON)')}</FormLabel>
              <FormControl><Textarea {...field} value={field.value || '{}'} autoComplete='off' spellCheck={false} rows={6} /></FormControl>
              <FormDescription>{t('ASR uploads require vod_ak, vod_sk, vod_space and vod_playback_domain. VOD tasks use the channel key and vod_space. Optional: vod_region, vod_playback_scheme, vod_url_auth_key.')}</FormDescription>
              <FormMessage />
            </FormItem>
          )} />
        )}
        {props.channelType === 103 && <p className='text-muted-foreground text-xs'>{t('Use explicit time model IDs for per-second prices. Token models require token pricing.')}</p>}
      </FieldGroup>
    </FieldSet>
  )
}
