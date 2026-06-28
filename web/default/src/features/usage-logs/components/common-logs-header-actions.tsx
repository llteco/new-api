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
import { useMemo } from 'react'
import { getRouteApi } from '@tanstack/react-router'
import { Eye, EyeOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { LOG_TYPE_ALL_VALUE } from '../constants'
import { getDefaultTimeRange } from '../lib/utils'
import type { CommonLogFilters } from '../types'
import { CommonLogsExportButton } from './common-logs-export-button'
import { CommonLogsStats } from './common-logs-stats'
import { useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

/**
 * Page-header actions for the Common Logs view: live usage stats, CSV
 * export, and a toggle for masking sensitive values (token names,
 * usernames, group names, and the quota figure shown in stats). All
 * controls live in the page header so the toolbar below stays focused
 * on filter inputs and form actions only.
 */
export function CommonLogsHeaderActions() {
  const { t } = useTranslation()
  const { sensitiveVisible, setSensitiveVisible } = useUsageLogsContext()
  const searchParams = route.useSearch()

  // Build the same filter shape the toolbar applies so the export
  // button honours the current selection (time range + filters + type).
  const exportFilters = useMemo<CommonLogFilters>(() => {
    const { start, end } = getDefaultTimeRange()
    return {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      channel: searchParams.channel || undefined,
      model: searchParams.model || undefined,
      token: searchParams.token || undefined,
      group: searchParams.group || undefined,
      username: searchParams.username || undefined,
      requestId: searchParams.requestId || undefined,
      upstreamRequestId: searchParams.upstreamRequestId || undefined,
    }
  }, [
    searchParams.channel,
    searchParams.endTime,
    searchParams.group,
    searchParams.model,
    searchParams.requestId,
    searchParams.startTime,
    searchParams.token,
    searchParams.upstreamRequestId,
    searchParams.username,
  ])

  const exportLogType = useMemo(() => {
    const rawType = Array.isArray(searchParams.type)
      ? searchParams.type[0]
      : searchParams.type
    if (typeof rawType !== 'string' || rawType === LOG_TYPE_ALL_VALUE) {
      return 0
    }
    const parsed = Number(rawType)
    return Number.isFinite(parsed) ? parsed : 0
  }, [searchParams.type])

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <CommonLogsStats />
      <CommonLogsExportButton filters={exportFilters} logType={exportLogType} />
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon'
              onClick={() => setSensitiveVisible(!sensitiveVisible)}
              aria-label={sensitiveVisible ? t('Hide') : t('Show')}
              className='text-muted-foreground hover:text-foreground size-7'
            />
          }
        >
          {sensitiveVisible ? <Eye /> : <EyeOff />}
        </TooltipTrigger>
        <TooltipContent>
          {sensitiveVisible ? t('Hide') : t('Show')}
        </TooltipContent>
      </Tooltip>
    </div>
  )
}
