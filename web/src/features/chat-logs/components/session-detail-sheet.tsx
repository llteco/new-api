/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { CodeBlock } from '@/components/ai-elements/code-block'
import { sideDrawerContentClassName } from '@/components/drawer-layout'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { formatTimestampToDate } from '@/lib/format'

import { chatSessionsQueryKeys, getChatSessionDetail } from '../api'
import { buildTranscript } from '../lib/transcript'

export interface SessionDetailSheetProps {
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

interface KeyedCode {
  key: string
  code: string
}

/**
 * Messages are arbitrary JSON without ids; derive stable unique keys from
 * content (with an occurrence suffix for exact duplicates).
 */
function toKeyedCodes(messages: unknown[]): KeyedCode[] {
  const seen = new Map<string, number>()
  return messages.map((message) => {
    const code = JSON.stringify(message, null, 2)
    const n = seen.get(code) ?? 0
    seen.set(code, n + 1)
    return { key: n === 0 ? code : `${code}#${n}`, code }
  })
}

export function SessionDetailSheet(props: SessionDetailSheetProps) {
  const { t } = useTranslation()

  const query = useQuery({
    queryKey: chatSessionsQueryKeys.detail(props.id ?? 0),
    queryFn: () => getChatSessionDetail(props.id as number),
    enabled: props.open && props.id != null,
  })

  const record = query.data?.data
  const turns = useMemo(
    () => (record ? buildTranscript(record.turns) : []),
    [record]
  )
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
      <div className='flex flex-col gap-6'>
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
          <MetaItem
            label={t('Model')}
            value={<span className='font-mono text-xs'>{record.session.model_name}</span>}
          />
          <MetaItem label={t('Token')} value={record.session.token_id} />
          <MetaItem label={t('User ID')} value={record.session.user_id} />
          <MetaItem label={t('Turns')} value={record.session.turn_count} />
          <MetaItem label={t('Messages')} value={record.session.message_count} />
          <MetaItem
            label={t('Created At')}
            value={formatTimestampToDate(record.session.created_at)}
          />
          <MetaItem
            label={t('Last Active')}
            value={formatTimestampToDate(record.session.last_active_at)}
          />
        </div>

        {turns.length === 0 ? (
          <p className='text-muted-foreground py-6 text-center text-sm'>
            {t('No data')}
          </p>
        ) : (
          <div className='flex flex-col gap-6'>
            {turns.map((view) => (
              <div key={view.turn.id} className='flex flex-col gap-2'>
                <div className='flex items-center gap-2'>
                  <span className='text-foreground text-sm font-medium'>
                    {t('Turn {{index}}', { index: view.turn.turn_index + 1 })}
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {formatTimestampToDate(view.turn.created_at)}
                  </span>
                </div>
                {toKeyedCodes(view.displayMessages).map((item) => (
                  <CodeBlock
                    key={item.key}
                    code={item.code}
                    language='json'
                    showToolbar
                    maxExpandedLines={20}
                  />
                ))}
                <CodeBlock
                  code={view.responseText}
                  language='json'
                  showToolbar
                  maxExpandedLines={20}
                />
              </div>
            ))}
          </div>
        )}
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
          <SheetTitle>{t('Transcript')}</SheetTitle>
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
