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
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CodeBlock } from '@/components/ai-elements/code-block'
import { sideDrawerContentClassName } from '@/components/drawer-layout'
import { Badge } from '@/components/ui/badge'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatTimestampToDate } from '@/lib/format'

import { chatLogsQueryKeys, getChatLogDetail } from '../api'

function tryFormat(body: string): string {
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
}

export interface ChatLogDetailSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  id: number | null
}

function MetaItem({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className='flex min-w-0 flex-col gap-0.5'>
      <span className='text-muted-foreground text-xs'>{label}</span>
      <span className='truncate text-sm font-medium'>{value}</span>
    </div>
  )
}

export function ChatLogDetailSheet(props: ChatLogDetailSheetProps) {
  const { t } = useTranslation()

  const query = useQuery({
    queryKey: chatLogsQueryKeys.detail(props.id ?? 0),
    queryFn: () => getChatLogDetail(props.id as number),
    enabled: props.open && props.id != null,
  })

  const record = query.data?.data
  const unavailable = !query.isLoading && (query.isError || !query.data?.success)

  let body: React.ReactNode = null
  if (query.isLoading) {
    body = (
      <div className='text-muted-foreground flex items-center justify-center gap-2 py-12'>
        <Loader2 className='size-5 animate-spin' />
        <span className='text-sm'>{t('Loading...')}</span>
      </div>
    )
  } else if (unavailable) {
    body = (
      <p className='text-muted-foreground py-12 text-center text-sm'>
        {query.data?.message || t('No data')}
      </p>
    )
  } else if (record) {
    body = (
      <div className='flex flex-col gap-4'>
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
          <MetaItem label={t('Model')} value={record.model_name} />
          <MetaItem label={t('Token')} value={record.token_id} />
          <MetaItem label='ID' value={record.id} />
          <MetaItem label={t('User ID')} value={record.user_id} />
          <MetaItem label={t('Channel ID')} value={record.channel_id} />
          <MetaItem label={t('Status Code')} value={record.status_code} />
          <MetaItem label={t('Use Time')} value={`${record.use_time}s`} />
          <MetaItem
            label={t('Created At')}
            value={formatTimestampToDate(record.created_at)}
          />
          <div className='flex min-w-0 flex-col gap-0.5'>
            <span className='text-muted-foreground text-xs'>{t('Stream')}</span>
            <Badge variant={record.is_stream ? 'default' : 'outline'}>
              {record.is_stream ? t('Yes') : t('No')}
            </Badge>
          </div>
          {record.truncated ? (
            <div className='flex min-w-0 flex-col gap-0.5'>
              <span className='text-muted-foreground text-xs'>
                {t('Truncated')}
              </span>
              <Badge variant='destructive'>{t('Truncated')}</Badge>
            </div>
          ) : null}
        </div>

        <div className='flex flex-col gap-1.5'>
          <span className='text-foreground text-sm font-medium'>
            {t('Request Body')}
          </span>
          <CodeBlock
            code={tryFormat(record.request_body || '')}
            language='json'
            showToolbar
            maxExpandedLines={30}
          />
        </div>

        <div className='flex flex-col gap-1.5'>
          <span className='text-foreground text-sm font-medium'>
            {t('Response Body')}
          </span>
          <CodeBlock
            code={tryFormat(record.response_body || '')}
            language='json'
            showToolbar
            maxExpandedLines={30}
          />
        </div>
      </div>
    )
  }

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent
        side='right'
        className={sideDrawerContentClassName('sm:max-w-3xl')}
      >
        <SheetHeader>
          <SheetTitle>{t('Chat Logs')}</SheetTitle>
          <SheetDescription>
            {t(
              'Conversation detail is only available when the token has chat-log enabled and the standalone database is configured.'
            )}
          </SheetDescription>
        </SheetHeader>

        <div className='flex-1 overflow-y-auto px-4 pb-6'>{body}</div>
      </SheetContent>
    </Sheet>
  )
}
