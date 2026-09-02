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
import { Input } from '@/components/ui/input'
import { formatTimestampToDate } from '@/lib/format'

import { chatSessionsQueryKeys, getChatSessions } from '../api'
import type { ChatSessionMeta } from '../types'
import { SessionDetailSheet } from './session-detail-sheet'

const route = getRouteApi('/_authenticated/chat-logs/')

export function ChatSessionsTable() {
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
      {
        columnId: 'user_id',
        searchKey: 'user_id',
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
    value: userIdFilter,
    inputValue: userIdInput,
    onChange: onUserIdChange,
    onCompositionStart: onUserIdCompositionStart,
    onCompositionEnd: onUserIdCompositionEnd,
    resetInput: resetUserIdInput,
  } = useDebouncedColumnFilter({
    columnFilters,
    columnId: 'user_id',
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

  const columns = useMemo<ColumnDef<ChatSessionMeta>[]>(
    () => [
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
        accessorKey: 'turn_count',
        header: t('Turns'),
        meta: { mobileHidden: true },
        cell: ({ row }) => row.getValue('turn_count') as number,
        size: 90,
      },
      {
        accessorKey: 'message_count',
        header: t('Messages'),
        meta: { mobileHidden: true },
        cell: ({ row }) => row.getValue('message_count') as number,
        size: 110,
      },
      {
        accessorKey: 'created_at',
        header: t('Created At'),
        meta: { mobileHidden: true },
        cell: ({ row }) =>
          formatTimestampToDate(row.getValue('created_at') as number),
        size: 170,
      },
      {
        accessorKey: 'last_active_at',
        header: t('Last Active'),
        cell: ({ row }) =>
          formatTimestampToDate(row.getValue('last_active_at') as number),
        size: 170,
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

  const queryParams = {
    token_id: tokenIdFilter ? Number(tokenIdFilter) : undefined,
    user_id: userIdFilter ? Number(userIdFilter) : undefined,
    model_name: modelNameFilter || undefined,
    page: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
  }

  const { data, isLoading, isFetching } = useQuery({
    queryKey: chatSessionsQueryKeys.list(queryParams),
    queryFn: () => getChatSessions(queryParams),
    placeholderData: (prev) => prev,
  })

  const sessions = data?.data ?? []
  const totalCount = data?.total ?? 0

  const { table } = useDataTable({
    data: sessions,
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
        skeletonKeyPrefix='chat-session-skeleton'
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
                type='number'
                placeholder={t('User ID')}
                value={userIdInput}
                onChange={onUserIdChange}
                onCompositionStart={onUserIdCompositionStart}
                onCompositionEnd={onUserIdCompositionEnd}
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
            !!tokenIdFilter || !!userIdFilter || !!modelNameFilter,
          onReset: () => {
            resetTokenIdInput()
            resetUserIdInput()
            resetModelNameInput()
          },
          hideViewOptions: true,
        }}
      />

      <SessionDetailSheet
        open={detailOpen}
        onOpenChange={setDetailOpen}
        id={detailId}
      />
    </>
  )
}
