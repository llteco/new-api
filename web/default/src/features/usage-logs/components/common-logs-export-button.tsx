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
import { useCallback, useState } from 'react'
import { Download, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useIsAdmin } from '@/hooks/use-admin'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getLogsExportURL } from '../api'
import type { CommonLogFilters } from '../types'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/utils'

interface CommonLogsExportButtonProps {
  filters: CommonLogFilters
  logType: number
}

function toStringOrUndefined(value: unknown): string | undefined {
  if (value === undefined || value === null) return undefined
  const str = String(value).trim()
  return str === '' ? undefined : str
}

/**
 * Export the current Common Logs filter selection as a CSV file. The
 * button triggers a direct browser download so the session cookies are
 * sent with the request and the response is handled as an attachment.
 */
export function CommonLogsExportButton(props: CommonLogsExportButtonProps) {
  const { t } = useTranslation()
  const isAdmin = useIsAdmin()
  const [exporting, setExporting] = useState(false)

  const handleExport = useCallback(async () => {
    const { start, end } = getDefaultTimeRange()
    const filterParams: CommonLogFilters = {
      startTime: props.filters.startTime ?? start,
      endTime: props.filters.endTime ?? end,
      model: props.filters.model,
      token: props.filters.token,
      group: props.filters.group,
      username: isAdmin ? props.filters.username : undefined,
      channel: isAdmin ? props.filters.channel : undefined,
      requestId: props.filters.requestId,
      upstreamRequestId: props.filters.upstreamRequestId,
    }
    const searchParams = buildSearchParams(filterParams, 'common')
    const channelValue = toStringOrUndefined(searchParams.channel)
    const exportParams = {
      type: props.logType,
      start_timestamp: searchParams.startTime
        ? Math.floor(Number(searchParams.startTime) / 1000)
        : undefined,
      end_timestamp: searchParams.endTime
        ? Math.floor(Number(searchParams.endTime) / 1000)
        : undefined,
      model_name: toStringOrUndefined(searchParams.model),
      token_name: toStringOrUndefined(searchParams.token),
      group: toStringOrUndefined(searchParams.group),
      username: isAdmin ? toStringOrUndefined(searchParams.username) : undefined,
      channel:
        isAdmin && channelValue ? Number(channelValue) : undefined,
      request_id: toStringOrUndefined(searchParams.requestId),
      upstream_request_id: toStringOrUndefined(searchParams.upstreamRequestId),
    }

    const url = getLogsExportURL(exportParams, isAdmin)
    setExporting(true)
    try {
      const link = document.createElement('a')
      link.href = url
      link.rel = 'noopener'
      // Let the browser pick a filename from Content-Disposition when
      // present; otherwise fall back to a sensible local name.
      link.download = ''
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      toast.success(t('Export'))
    } catch (err) {
      const message =
        err instanceof Error ? err.message : String(err)
      toast.error(`${t('Export failed')}: ${message}`)
    } finally {
      // Give the browser a beat to start the download before flipping
      // back to the idle state. Streaming responses can outlive the
      // click handler, but the visual feedback is enough for the user.
      window.setTimeout(() => setExporting(false), 800)
    }
  }, [props.filters, props.logType, isAdmin, t])

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={handleExport}
            disabled={exporting}
            aria-label={t('Export to CSV')}
            className='text-muted-foreground hover:text-foreground size-7'
          />
        }
      >
        {exporting ? (
          <Loader2 className='size-4 animate-spin' />
        ) : (
          <Download className='size-4' />
        )}
      </TooltipTrigger>
      <TooltipContent>{t('Export to CSV')}</TooltipContent>
    </Tooltip>
  )
}
