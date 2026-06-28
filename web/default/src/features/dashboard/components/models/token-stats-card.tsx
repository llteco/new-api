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
import { useQuery } from '@tanstack/react-query'
import { VChart } from '@visactor/react-vchart'
import { PieChart as PieChartIcon, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { computeTimeRange } from '@/lib/time'
import { formatCompactNumber, formatNumber } from '@/lib/format'
import { VCHART_OPTION } from '@/lib/vchart'
import { useTheme } from '@/context/theme-provider'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { getTokenStats } from '@/features/dashboard/api'
import { processTokenStats, summarizeTokenTotal } from '@/features/dashboard/lib/token-stats'
import type {
  DashboardFilters,
  TokenStatDimension,
  TokenStatGranularity,
} from '@/features/dashboard/types'

let themeManagerPromise: Promise<
  (typeof import('@visactor/vchart'))['ThemeManager']
> | null = null

const MAX_INLINE_STAT_CHARS = 9

const TOP_N_OPTIONS = [5, 10, 20, 50]

interface TokenStatsCardProps {
  filters?: DashboardFilters
}

function formatStatNumber(value: number, locale: Intl.LocalesArgument) {
  const fullValue = formatNumber(value, locale)
  const displayValue =
    fullValue.length > MAX_INLINE_STAT_CHARS
      ? formatCompactNumber(value, locale)
      : fullValue
  return { displayValue, fullValue }
}

export function TokenStatsCard(props: TokenStatsCardProps) {
  const { t, i18n } = useTranslation()
  const { resolvedTheme } = useTheme()
  const [themeReady, setThemeReady] = useState(false)
  const themeManagerRef = useRef<
    (typeof import('@visactor/vchart'))['ThemeManager'] | null
  >(null)
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = !!(user?.role && user.role >= 10)

  const [dimension, setDimension] = useState<TokenStatDimension>('model')
  const [granularity, setGranularity] = useState<TokenStatGranularity>('day')
  const [topN, setTopN] = useState<number>(10)

  const timeRange = useMemo(() => {
    const fallbackDays = props.filters?.time_granularity
      ? getPresetRollingDaysForGranularity(props.filters.time_granularity)
      : 7
    return computeTimeRange(
      fallbackDays,
      props.filters?.start_timestamp,
      props.filters?.end_timestamp
    )
  }, [props.filters])

  const params = useMemo(
    () => ({
      start_timestamp: timeRange.start_timestamp,
      end_timestamp: timeRange.end_timestamp,
      dimension,
      granularity,
      top_n: topN,
      username: isAdmin ? props.filters?.username : undefined,
    }),
    [timeRange, dimension, granularity, topN, isAdmin, props.filters?.username]
  )

  useEffect(() => {
    const updateTheme = async () => {
      setThemeReady(false)
      if (!themeManagerPromise) {
        themeManagerPromise = import('@visactor/vchart').then(
          (m) => m.ThemeManager
        )
      }
      const ThemeManager = await themeManagerPromise
      themeManagerRef.current = ThemeManager
      ThemeManager.setCurrentTheme(resolvedTheme === 'dark' ? 'dark' : 'light')
      setThemeReady(true)
    }
    updateTheme()
  }, [resolvedTheme])

  const { data: response, isLoading } = useQuery({
    queryKey: ['dashboard', 'token-stats', params, isAdmin],
    queryFn: () => getTokenStats(params, isAdmin),
    select: (res) => (res.success ? res.data : null),
    staleTime: 60_000,
  })

  const chartData = useMemo(
    () =>
      processTokenStats(
        isLoading ? null : (response ?? null),
        dimension,
        granularity,
        t
      ),
    [response, isLoading, dimension, granularity, t]
  )

  const totals = useMemo(() => {
    if (!response?.items) {
      return { prompt: 0, completion: 0, total: 0, count: 0, quota: 0 }
    }
    return summarizeTokenTotal(response.items)
  }, [response])

  const locale = i18n.resolvedLanguage || i18n.language
  const stats = [
    {
      key: 'total',
      title: t('Total Tokens'),
      value: formatStatNumber(totals.total, locale),
    },
    {
      key: 'prompt',
      title: t('Prompt'),
      value: formatStatNumber(totals.prompt, locale),
    },
    {
      key: 'completion',
      title: t('Completion'),
      value: formatStatNumber(totals.completion, locale),
    },
  ]

  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='flex flex-wrap items-center gap-2 border-b px-3 py-2 sm:px-5 sm:py-3'>
        <PieChartIcon className='text-muted-foreground/60 size-4' />
        <div className='text-sm font-semibold'>{t('Token Stats')}</div>
        <div className='ml-auto flex flex-wrap items-center gap-1.5'>
          <Tabs
            value={dimension}
            onValueChange={(value) => setDimension(value as TokenStatDimension)}
          >
            <TabsList>
              <TabsTrigger value='model' className='px-2.5 text-xs'>
                {t('Token Stats by Model')}
              </TabsTrigger>
              <TabsTrigger value='user' className='px-2.5 text-xs'>
                {t('Token Stats by User')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <Tabs
            value={granularity}
            onValueChange={(value) =>
              setGranularity(value as TokenStatGranularity)
            }
          >
            <TabsList>
              <TabsTrigger value='hour' className='px-2.5 text-xs'>
                {t('Hour')}
              </TabsTrigger>
              <TabsTrigger value='day' className='px-2.5 text-xs'>
                {t('Day')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
          <Tabs
            value={String(topN)}
            onValueChange={(value) => setTopN(Number(value))}
          >
            <TabsList>
              <span className='text-muted-foreground px-2 text-xs font-medium whitespace-nowrap'>
                {t('Top')}
              </span>
              {TOP_N_OPTIONS.map((limit) => (
                <TabsTrigger
                  key={limit}
                  value={String(limit)}
                  className='px-2.5 text-xs'
                >
                  {t('Top {{count}}', { count: limit })}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          {isLoading && (
            <Loader2 className='text-muted-foreground size-4 animate-spin' />
          )}
        </div>
      </div>

      <div className='divide-border/60 grid min-w-0 grid-cols-2 divide-x sm:grid-cols-3'>
        {stats.map((stat) => (
          <div key={stat.key} className='min-w-0 px-3 py-2.5 sm:px-5 sm:py-4'>
            <div className='text-muted-foreground truncate text-xs font-medium tracking-wider uppercase'>
              {stat.title}
            </div>
            {isLoading ? (
              <Skeleton className='mt-2 h-7 w-24' />
            ) : (
              <div
                className='text-foreground mt-1.5 max-w-full truncate font-mono text-lg font-bold tracking-tight tabular-nums sm:text-2xl'
                title={stat.value.fullValue}
              >
                {stat.value.displayValue}
              </div>
            )}
          </div>
        ))}
      </div>

      <div className='grid gap-3 p-2 sm:p-3'>
        <div className='overflow-hidden rounded-lg border'>
          <div className='h-[300px] p-1.5 sm:h-96 sm:p-2'>
            {isLoading || !themeReady ? (
              <Skeleton className='h-full w-full' />
            ) : (
              <VChart
                key={`token-stats-bar-${dimension}-${topN}-${resolvedTheme}`}
                spec={{
                  ...chartData.spec_bar,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
              />
            )}
          </div>
        </div>
        <div className='overflow-hidden rounded-lg border'>
          <div className='h-[300px] p-1.5 sm:h-96 sm:p-2'>
            {isLoading || !themeReady ? (
              <Skeleton className='h-full w-full' />
            ) : (
              <VChart
                key={`token-stats-line-${dimension}-${granularity}-${topN}-${resolvedTheme}`}
                spec={{
                  ...chartData.spec_line,
                  theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                  background: 'transparent',
                }}
                option={VCHART_OPTION}
              />
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function getPresetRollingDaysForGranularity(
  granularity: NonNullable<DashboardFilters['time_granularity']>
): number {
  // Map dashboard granularity to a sensible rolling-day default so the
  // token stats query keeps a bounded window when the dashboard's
  // explicit start/end are missing.
  if (granularity === 'hour') return 1
  if (granularity === 'week') return 30
  return 7
}
