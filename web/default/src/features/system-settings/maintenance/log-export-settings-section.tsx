/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useMemo, useRef, useState } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'
import { startLogAutoExportTask, getSystemTask } from '../api'
import type { SystemTaskResponse } from '../types'

const logExportSchema = z.object({
  log_export_setting: z.object({
    enabled: z.boolean(),
    interval_minutes: z.coerce.number().min(1).max(1440),
    weekday: z.coerce.number().min(0).max(6),
    hour: z.coerce.number().min(0).max(23),
    minute: z.coerce.number().min(0).max(59),
    duration_days: z.coerce.number().min(1).max(365),
    output_dir: z.string(),
  }),
})

type LogExportFormInput = z.input<typeof logExportSchema>
type LogExportFormValues = z.output<typeof logExportSchema>

type FlatLogExportDefaults = {
  'log_export_setting.enabled': boolean
  'log_export_setting.interval_minutes': number
  'log_export_setting.weekday': number
  'log_export_setting.hour': number
  'log_export_setting.minute': number
  'log_export_setting.duration_days': number
  'log_export_setting.output_dir': string
}

type LogExportSettingsSectionProps = {
  defaultValues: FlatLogExportDefaults
}

const buildFormDefaults = (defaults: FlatLogExportDefaults): LogExportFormInput => ({
  log_export_setting: {
    enabled: defaults['log_export_setting.enabled'] ?? true,
    interval_minutes: defaults['log_export_setting.interval_minutes'] ?? 60,
    weekday: defaults['log_export_setting.weekday'] ?? 5,
    hour: defaults['log_export_setting.hour'] ?? 18,
    minute: defaults['log_export_setting.minute'] ?? 0,
    duration_days: defaults['log_export_setting.duration_days'] ?? 7,
    output_dir: defaults['log_export_setting.output_dir'] ?? '',
  },
})

const normalizeDefaults = (defaults: FlatLogExportDefaults): FlatLogExportDefaults => ({
  'log_export_setting.enabled': defaults['log_export_setting.enabled'] ?? true,
  'log_export_setting.interval_minutes': defaults['log_export_setting.interval_minutes'] ?? 60,
  'log_export_setting.weekday': defaults['log_export_setting.weekday'] ?? 5,
  'log_export_setting.hour': defaults['log_export_setting.hour'] ?? 18,
  'log_export_setting.minute': defaults['log_export_setting.minute'] ?? 0,
  'log_export_setting.duration_days': defaults['log_export_setting.duration_days'] ?? 7,
  'log_export_setting.output_dir': defaults['log_export_setting.output_dir'] ?? '',
})

const normalizeFormValues = (values: LogExportFormValues): FlatLogExportDefaults => ({
  'log_export_setting.enabled': values.log_export_setting.enabled,
  'log_export_setting.interval_minutes': values.log_export_setting.interval_minutes,
  'log_export_setting.weekday': values.log_export_setting.weekday,
  'log_export_setting.hour': values.log_export_setting.hour,
  'log_export_setting.minute': values.log_export_setting.minute,
  'log_export_setting.duration_days': values.log_export_setting.duration_days,
  'log_export_setting.output_dir': values.log_export_setting.output_dir ?? '',
})

const WEEKDAY_OPTIONS = [
  { value: 0, label: 'Sunday' },
  { value: 1, label: 'Monday' },
  { value: 2, label: 'Tuesday' },
  { value: 3, label: 'Wednesday' },
  { value: 4, label: 'Thursday' },
  { value: 5, label: 'Friday' },
  { value: 6, label: 'Saturday' },
]

export function LogExportSettingsSection({
  defaultValues,
}: LogExportSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<FlatLogExportDefaults>(normalizeDefaults(defaultValues))
  const baselineSerializedRef = useRef<string>(JSON.stringify(normalizeDefaults(defaultValues)))

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<LogExportFormInput, unknown, LogExportFormValues>({
    resolver: zodResolver(logExportSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  useEffect(() => {
    const normalized = normalizeDefaults(defaultValues)
    const serialized = JSON.stringify(normalized)
    if (serialized === baselineSerializedRef.current) return
    baselineRef.current = normalized
    baselineSerializedRef.current = serialized
    form.reset(buildFormDefaults(normalized))
  }, [defaultValues, form])

  const onSubmit = async (values: LogExportFormValues) => {
    const flat = normalizeFormValues(values)
    const baseline = baselineRef.current

    const keys = Object.keys(flat) as Array<keyof FlatLogExportDefaults>
    for (const key of keys) {
      const value = flat[key]
      const baseValue = baseline[key]
      if (value !== baseValue) {
        await updateOption.mutateAsync({ key, value })
      }
    }

    baselineRef.current = flat
    baselineSerializedRef.current = JSON.stringify(flat)
  }

  const isEnabled = form.watch('log_export_setting.enabled')

  const [isExporting, setIsExporting] = useState(false)
  const [exportTask, setExportTask] = useState<SystemTaskResponse | null>(null)

  const exportTaskId = exportTask?.data?.task_id
  const exportTaskActive =
    exportTask?.data?.status === 'pending' || exportTask?.data?.status === 'running'

  useEffect(() => {
    if (!exportTaskId || !exportTaskActive) return

    let cancelled = false
    const interval = window.setInterval(async () => {
      try {
        const res = await getSystemTask(exportTaskId)
        if (cancelled || !res.success || !res.data) return

        setExportTask(res)
        if (res.data.status !== 'pending' && res.data.status !== 'running') {
          if (res.data.status === 'succeeded') {
            toast.success(t('Log export task completed.'))
          } else if (res.data.status === 'failed') {
            toast.error(res.data.error || t('Log export task failed.'))
          }
        }
      } catch {
        /* keep polling */
      }
    }, 1000)

    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [exportTaskActive, exportTaskId, t])

  const handleManualExport = async () => {
    setIsExporting(true)
    try {
      const res = await startLogAutoExportTask()
      if (!res.success) {
        throw new Error(res.message || t('Failed to start log export.'))
      }
      if (!res.data) {
        throw new Error(t('Failed to start log export.'))
      }
      setExportTask(res)
      toast.success(t('Log export task started.'))
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t('Failed to start log export.')
      toast.error(message)
    } finally {
      setIsExporting(false)
    }
  }

  return (
    <SettingsSection title={t('Log Auto Export')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel={t('Save log export settings')}
          />
          <FormField
            control={form.control}
            name='log_export_setting.enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Auto Export')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Automatically export consumption logs to JSON on a scheduled basis.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          {isEnabled && (
            <div className='space-y-4'>
              <FormField
                control={form.control}
                name='log_export_setting.weekday'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Weekday')}</FormLabel>
                    <Select
                      value={String(field.value)}
                      onValueChange={(value) => field.onChange(Number(value))}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectGroup>
                          {WEEKDAY_OPTIONS.map((option) => (
                            <SelectItem key={option.value} value={String(option.value)}>
                              {t(option.label)}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='grid grid-cols-2 gap-4'>
                <FormField
                  control={form.control}
                  name='log_export_setting.hour'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Hour')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          max={23}
                          {...field}
                          value={field.value as number}
                          {...safeNumberFieldProps}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='log_export_setting.minute'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Minute')}</FormLabel>
                      <FormControl>
                        <Input
                          type='number'
                          min={0}
                          max={59}
                          {...field}
                          value={field.value as number}
                          {...safeNumberFieldProps}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name='log_export_setting.duration_days'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Duration (Days)')}</FormLabel>
                    <FormDescription>
                      {t('Number of days of logs to export each time.')}
                    </FormDescription>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        max={365}
                        {...field}
                        value={field.value as number}
                        {...safeNumberFieldProps}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='log_export_setting.interval_minutes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Check Interval (Minutes)')}</FormLabel>
                    <FormDescription>
                      {t('How often to check if the scheduled export is due.')}
                    </FormDescription>
                    <FormControl>
                      <Input
                        type='number'
                        min={1}
                        max={1440}
                        {...field}
                        value={field.value as number}
                        {...safeNumberFieldProps}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='log_export_setting.output_dir'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Output Directory')}</FormLabel>
                    <FormDescription>
                      {t(
                        'Leave empty to use the default log directory. Relative paths are resolved from the working directory.'
                      )}
                    </FormDescription>
                    <FormControl>
                      <Input
                        type='text'
                        placeholder={t('e.g., /var/log/new-api/exports')}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className='flex items-center gap-3 pt-2'>
                <Button
                  type='button'
                  variant='outline'
                  onClick={handleManualExport}
                  disabled={isExporting || exportTaskActive}
                >
                  {isExporting || exportTaskActive
                    ? t('Exporting...')
                    : t('Export Now')}
                </Button>
                {exportTask?.data?.status === 'failed' && exportTask.data.error && (
                  <span className='text-destructive text-sm'>
                    {exportTask.data.error}
                  </span>
                )}
              </div>
            </div>
          )}
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
