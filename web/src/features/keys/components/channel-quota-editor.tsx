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
import { Plus, Trash2 } from 'lucide-react'
import { useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { getChannels } from '@/features/channels/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'

import type { TokenChannelQuotaInput } from '../types'

type ChannelQuotaEditorProps = {
  value: TokenChannelQuotaInput[]
  onChange: (value: TokenChannelQuotaInput[]) => void
  disabled?: boolean
}

export function ChannelQuotaEditor(props: ChannelQuotaEditorProps) {
  const { t } = useTranslation()
  const { data: channelsData } = useQuery({
    queryKey: ['channel-quota-editor-channels'],
    queryFn: () => getChannels({ p: 1, page_size: 999, status: 'enabled' }),
    staleTime: 60_000,
  })

  const channels = useMemo(
    () => channelsData?.data?.items ?? [],
    [channelsData]
  )

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaStep = tokensOnly ? 1 : 0.01
  const quotaPlaceholder = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })

  const rows = props.value

  const rowKeysRef = useRef<number[]>([])
  const keyCounterRef = useRef(0)
  if (rowKeysRef.current.length !== rows.length) {
    const next: number[] = []
    for (let i = 0; i < rows.length; i++) {
      next.push(rowKeysRef.current[i] ?? ++keyCounterRef.current)
    }
    rowKeysRef.current = next
  }
  const rowKeys = rowKeysRef.current

  const updateRow = (index: number, patch: Partial<TokenChannelQuotaInput>) => {
    const next = rows.map((row, i) => (i === index ? { ...row, ...patch } : row))
    props.onChange(next)
  }

  const removeRow = (index: number) => {
    props.onChange(rows.filter((_, i) => i !== index))
  }

  const addRow = () => {
    props.onChange([...rows, { channel_id: 0, reset_quota: 0 }])
  }

  return (
    <div className='flex flex-col gap-3'>
      {rows.map((row, index) => {
        const usedElsewhere = rows
          .filter((_, i) => i !== index)
          .map((v) => v.channel_id)
        return (
        <div
          key={rowKeys[index]}
          className='grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-2'
        >
          <Select
            value={String(row.channel_id)}
            onValueChange={(val) =>
              updateRow(index, { channel_id: Number(val) })
            }
            disabled={props.disabled}
          >
            <SelectTrigger>
              <SelectValue placeholder={t('Channel')} />
            </SelectTrigger>
            <SelectContent>
              {channels.map((channel) => (
                <SelectItem
                  key={channel.id}
                  value={String(channel.id)}
                  disabled={usedElsewhere.includes(channel.id)}
                >
                  {channel.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Input
            type='number'
            min='0'
            step={quotaStep}
            disabled={props.disabled}
            placeholder={quotaPlaceholder}
            value={quotaUnitsToDollars(row.reset_quota) || 0}
            onChange={(e) =>
              updateRow(index, {
                reset_quota: parseQuotaFromDollars(
                  Number.parseFloat(e.target.value) || 0
                ),
              })
            }
          />

          <Button
            type='button'
            variant='ghost'
            size='icon'
            disabled={props.disabled}
            onClick={() => removeRow(index)}
            aria-label={t('Remove')}
          >
            <Trash2 className='size-4' />
          </Button>
        </div>
        )
      })}

      <Button
        type='button'
        variant='outline'
        size='sm'
        className='w-fit'
        disabled={props.disabled}
        onClick={addRow}
      >
        <Plus className='size-4' />
        {t('Add channel quota')}
      </Button>
    </div>
  )
}
