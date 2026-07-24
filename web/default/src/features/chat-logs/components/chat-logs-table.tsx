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
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { Eye } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  useDataTable,
  useDebouncedColumnFilter,
} from '@/components/data-table'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { formatTimestampToDate } from '@/lib/format'

import { chatLogsQueryKeys, getChatLogs } from '../api'
import type { ChatLogMeta } from '../types'
import { ChatLogDetailSheet } from './chat-log-detail-sheet'

const route = getRouteApi('/_authenticated/chat-logs/')

export function ChatLogsTable() {
  const { t } = useTranslation()
  const [detailId, setDetailId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: 10 },
    globalFilter: { enabled: false },
    columnFilters: [
      {
        columnId: 'token_id',
        searchKey: 'token_id',
        type: 'string',
        serialize: (value) => (value ? Number(value) : undefined),
        deserialize: (value) =>
          typeof value === 'number' ? String(value) : '',
      },
      { columnId: 'model_name', searchKey: 'model_name', type: 'string' },
    ],
  })

  const {
    value: tokenIdFilter,
    inputValue: tokenIdInput,
    onChange: onTokenIdChange,
    onCompositionStart: onTokenIdCompositionStart,
    onCompositionEnd: onTokenIdCompositionEnd,
    resetInput: resetTokenIdInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'token_id',
    onColumnFiltersChange,
  })

  const {
    value: modelNameFilter,
    inputValue: modelNameInput,
    onChange: onModelNameChange,
    onCompositionStart: onModelNameCompositionStart,
    onCompositionEnd: onModelNameCompositionEnd,
    resetInput: resetModelNameInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'model_name',
    onColumnFiltersChange,
  })

  const handleViewDetail = (id: number) => {
    setDetailId(id)
    setDetailOpen(true)
  }

  const columns = useMemo<ColumnDef<ChatLogMeta>[]>(
    () => [
      {
        accessorKey: 'created_at',
        header: t('Created At'),
        cell: ({ row }) =>
          formatTimestampToDate(row.getValue('created_at') as number),
        size: 170,
      },
      {
        accessorKey: 'model_name',
        header: t('Model'),
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <span className='font-mono text-xs'>
            {row.getValue('model_name') as string}
          </span>
        ),
        size: 180,
      },
      {
        accessorKey: 'token_id',
        header: t('Token'),
        cell: ({ row }) => row.getValue('token_id') as number,
        size: 90,
      },
      {
        accessorKey: 'user_id',
        header: t('User ID'),
        meta: { mobileHidden: true },
        cell: ({ row }) => row.getValue('user_id') as number,
        size: 90,
      },
      {
        accessorKey: 'channel_id',
        header: t('Channel ID'),
        meta: { mobileHidden: true },
        cell: ({ row }) => row.getValue('channel_id') as number,
        size: 110,
      },
      {
        accessorKey: 'is_stream',
        header: t('Stream'),
        meta: { mobileHidden: true },
        cell: ({ row }) => {
          const stream = row.getValue('is_stream') as boolean
          return (
            <Badge variant={stream ? 'default' : 'outline'}>
              {stream ? t('Yes') : t('No')}
            </Badge>
          )
        },
        size: 90,
      },
      {
        accessorKey: 'truncated',
        header: t('Truncated'),
        meta: { mobileHidden: true },
        cell: ({ row }) =>
          (row.getValue('truncated') as boolean) ? (
            <Badge variant='destructive'>{t('Truncated')}</Badge>
          ) : (
            <span className='text-muted-foreground text-xs'>-</span>
          ),
        size: 110,
      },
      {
        accessorKey: 'status_code',
        header: t('Status Code'),
        meta: { mobileHidden: true },
        cell: ({ row }) => row.getValue('status_code') as number,
        size: 110,
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        cell: ({ row }) => (
          <Button
            variant='ghost'
            size='sm'
            className='gap-1'
            onClick={() => handleViewDetail(row.original.id)}
          >
            <Eye className='size-4' />
            {t('View detail')}
          </Button>
        ),
        enableSorting: false,
        enableHiding: false,
        size: 130,
      },
    ],
    [t]
  )

  const { data, isLoading, isFetching } = useQuery({
    queryKey: chatLogsQueryKeys.list({
      token_id: tokenIdFilter ? Number(tokenIdFilter) : undefined,
      model_name: modelNameFilter || undefined,
      page: pagination.pageIndex + 1,
      page_size: pagination.pageSize,
    }),
    queryFn: () =>
      getChatLogs({
        token_id: tokenIdFilter ? Number(tokenIdFilter) : undefined,
        model_name: modelNameFilter || undefined,
        page: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      }),
    placeholderData: (prev) => prev,
  })

  const logs = data?.data ?? []
  const totalCount = data?.total ?? 0

  const { table } = useDataTable({
    data: logs,
    columns,
    totalCount,
    columnFilters,
    pagination,
    onColumnFiltersChange,
    onPaginationChange,
    manualPagination: true,
    manualFiltering: true,
    enableRowSelection: false,
    ensurePageInRange,
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('No Chat Logs Found')}
        emptyDescription={t('No conversation records available.')}
        skeletonKeyPrefix='chat-log-skeleton'
        toolbarProps={{
          customSearch: null,
          additionalSearch: (
            <>
              <Input
                type='number'
                placeholder={t('Token')}
                value={tokenIdInput}
                onChange={onTokenIdChange}
                onCompositionStart={onTokenIdCompositionStart}
                onCompositionEnd={onTokenIdCompositionEnd}
                className='w-full sm:w-[140px]'
              />
              <Input
                placeholder={t('Model')}
                value={modelNameInput}
                onChange={onModelNameChange}
                onCompositionStart={onModelNameCompositionStart}
                onCompositionEnd={onModelNameCompositionEnd}
                className='w-full sm:w-[180px]'
              />
            </>
          ),
          hasAdditionalFilters:
            !!tokenIdFilter || !!modelNameFilter,
          onReset: () => {
            resetTokenIdInput()
            resetModelNameInput()
          },
          hideViewOptions: true,
        }}
      />

      <ChatLogDetailSheet
        open={detailOpen}
        onOpenChange={setDetailOpen}
        id={detailId}
      />
    </>
  )
}
