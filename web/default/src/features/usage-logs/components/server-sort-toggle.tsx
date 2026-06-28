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
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

const route = getRouteApi('/_authenticated/usage-logs/$section')

interface ServerSortToggleProps {
  checked: boolean
}

/**
 * Checkbox that toggles between client-side sorting (current page only)
 * and server-side sorting (entire dataset). The state is persisted in
 * the URL search params so it survives page refreshes and is
 * shareable.
 */
export function ServerSortToggle(props: ServerSortToggleProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const searchParams = route.useSearch()
  const section = route.useParams().section

  const [draft, setDraft] = useState(props.checked)

  // Keep the draft synced with URL-driven prop changes so navigating
  // back/forward updates the checkbox.
  useEffect(() => {
    setDraft(props.checked)
  }, [props.checked])

  const label = t('Sort entire dataset')

  const tooltipContent = useMemo(
    () =>
      draft
        ? t(
            'Server-side sorting is enabled. Column clicks sort the entire dataset across all pages.'
          )
        : t(
            'Client-side sorting only reorders rows on the current page. Enable to sort across the entire dataset.'
          ),
    [draft, t]
  )

  const handleChange = (checked: boolean) => {
    setDraft(checked)
    navigate({
      to: '/usage-logs/$section',
      params: { section },
      search: {
        ...searchParams,
        serverSort: checked,
        page: 1,
      },
    })
  }

  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <div className='flex items-center gap-1.5'>
            <Checkbox
              checked={draft}
              onCheckedChange={(value) => handleChange(value === true)}
              id='server-sort-toggle'
              className='size-3.5'
            />
            <label
              htmlFor='server-sort-toggle'
              className='text-muted-foreground cursor-pointer select-none text-xs font-medium'
            >
              {label}
            </label>
            <Info className='text-muted-foreground/50 size-3' />
          </div>
        }
      />
      <TooltipContent side='bottom' className='max-w-xs'>
        {tooltipContent}
      </TooltipContent>
    </Tooltip>
  )
}
