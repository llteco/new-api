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
import { getDashboardChartColors } from '@/features/dashboard/lib/charts'
import type {
  TokenDimensionStat,
  TokenStatDimension,
  TokenStatGranularity,
  TokenStatsResponse,
  TokenTimePoint,
} from '@/features/dashboard/types'

type TFunction = (key: string) => string

export interface ProcessedTokenStatsData {
  spec_bar: Record<string, unknown>
  spec_line: Record<string, unknown>
}

function formatInt(value: number) {
  return Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(value)
}

function formatBucketLabel(timestamp: number, granularity: TokenStatGranularity) {
  const date = new Date(timestamp * 1000)
  if (granularity === 'hour') {
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    const hour = String(date.getHours()).padStart(2, '0')
    return `${month}-${day} ${hour}:00`
  }
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${month}-${day}`
}

export function processTokenStats(
  response: TokenStatsResponse | null | undefined,
  _dimension: TokenStatDimension,
  granularity: TokenStatGranularity,
  t: TFunction
): ProcessedTokenStatsData {
  const tt: TFunction = t ?? ((x) => x)

  const empty = () => ({
    spec_bar: {
      type: 'bar',
      data: [{ id: 'tokenStatsBar', values: [] }],
      xField: 'tokens',
      yField: 'name',
      seriesField: 'tokenType',
      stack: true,
      direction: 'horizontal',
      title: {
        visible: true,
        text: tt('Top Token Consumers'),
        subtext: tt('No data available'),
      },
      legends: { visible: true, selectMode: 'single' },
      background: { fill: 'transparent' },
    } as Record<string, unknown>,
    spec_line: {
      type: 'line',
      data: [{ id: 'tokenStatsLine', values: [] }],
      xField: 'time',
      yField: 'tokens',
      seriesField: 'name',
      title: {
        visible: true,
        text: tt('Token Distribution Trend'),
        subtext: tt('No data available'),
      },
      legends: { visible: true, selectMode: 'single' },
      background: { fill: 'transparent' },
    } as Record<string, unknown>,
  })

  if (!response || !response.items || response.items.length === 0) {
    return empty()
  }

  const items = [...response.items]
  const colors = getDashboardChartColors(items.length)
  const promptLabel = tt('Prompt')
  const completionLabel = tt('Completion')

  const barValues = items.flatMap((item) => [
    { name: item.name, tokenType: promptLabel, tokens: item.prompt_tokens },
    { name: item.name, tokenType: completionLabel, tokens: item.completion_tokens },
  ])

  const specBar = {
    type: 'bar',
    data: [{ id: 'tokenStatsBar', values: barValues }],
    xField: 'tokens',
    yField: 'name',
    seriesField: 'tokenType',
    stack: true,
    direction: 'horizontal',
    title: {
      visible: true,
      text: tt('Top Token Consumers'),
    },
    axes: [
      {
        orient: 'bottom',
        title: { visible: true, text: tt('Total Tokens') },
        label: {
          formatMethod: (val: number) => formatInt(val),
        },
      },
      { orient: 'left', type: 'band' },
    ],
    legends: { visible: true, selectMode: 'single' },
    color: { type: 'ordinal', range: colors.slice(0, 2) },
    tooltip: [
      {
        title: (datum: { name?: string }) => datum.name ?? '',
        content: [
          {
            key: (datum: { tokenType?: string }) => datum.tokenType ?? '',
            value: (datum: { tokens?: number }) =>
              datum.tokens !== undefined ? formatInt(datum.tokens) : '',
          },
        ],
      },
    ],
    background: { fill: 'transparent' },
  } as Record<string, unknown>

  // Convert timeseries (array of arrays per name) into flat values.
  const lineValues = buildLineValues(response.timeseries, granularity)

  const seriesNames = items.map((item) => item.name)
  const nameColorMap = seriesNames.reduce<Record<string, string>>(
    (acc, name, idx) => {
      acc[name] = colors[idx % colors.length]
      return acc
    },
    {}
  )

  const specLine = {
    type: 'line',
    data: [{ id: 'tokenStatsLine', values: lineValues }],
    xField: 'time',
    yField: 'tokens',
    seriesField: 'name',
    point: { visible: lineValues.length <= 60 },
    title: {
      visible: true,
      text: tt('Token Distribution Trend'),
    },
    axes: [
      {
        orient: 'left',
        title: { visible: true, text: tt('Total Tokens') },
        label: { formatMethod: (val: number) => formatInt(val) },
      },
      { orient: 'bottom', type: 'band' },
    ],
    legends: { visible: true, selectMode: 'single' },
    color: {
      type: 'ordinal',
      domain: seriesNames,
      range: seriesNames.map((name) => nameColorMap[name]),
    },
    tooltip: [
      {
        title: (datum: { time?: string }) => datum.time ?? '',
        content: [
          {
            key: (datum: { name?: string }) => datum.name ?? '',
            value: (datum: { tokens?: number }) =>
              datum.tokens !== undefined ? formatInt(datum.tokens) : '',
          },
        ],
      },
    ],
    background: { fill: 'transparent' },
  } as Record<string, unknown>

  return { spec_bar: specBar, spec_line: specLine }
}

function buildLineValues(
  timeseries: TokenTimePoint[][],
  granularity: TokenStatGranularity
) {
  if (!timeseries || timeseries.length === 0) return []
  const values: Array<{ time: string; name: string; tokens: number }> = []
  for (const series of timeseries) {
    for (const point of series) {
      values.push({
        time: formatBucketLabel(point.timestamp, granularity),
        name: point.name,
        tokens: point.tokens,
      })
    }
  }
  return values
}

export function summarizeTokenTotal(items: TokenDimensionStat[]) {
  return items.reduce(
    (acc, item) => {
      acc.prompt += item.prompt_tokens
      acc.completion += item.completion_tokens
      acc.total += item.total_tokens
      acc.count += item.count
      acc.quota += item.quota
      return acc
    },
    { prompt: 0, completion: 0, total: 0, count: 0, quota: 0 }
  )
}
