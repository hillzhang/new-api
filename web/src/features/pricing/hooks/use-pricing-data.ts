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
import { useMemo } from 'react'

import { useStatus } from '@/hooks/use-status'

import { getPricing } from '../api'

export function usePricingData() {
  const { status } = useStatus()

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['pricing'],
    queryFn: getPricing,
    staleTime: 5 * 60 * 1000,
  })

  // Ensure rates never reach zero to prevent division errors
  const priceRate = useMemo(
    () => Math.max((status?.price as number) ?? 1, 0.001),
    [status?.price]
  )
  const usdExchangeRate = useMemo(
    () => Math.max((status?.usd_exchange_rate as number) ?? priceRate, 0.001),
    [status?.usd_exchange_rate, priceRate]
  )

  const models = useMemo(() => {
    if (!data?.data || !data?.vendors) return []

    const vendorMap = new Map(data.vendors.map((v) => [v.id, v]))

    return data.data.map((model) => {
      const vendor = model.vendor_id
        ? vendorMap.get(model.vendor_id)
        : undefined

      const effectiveGroupRatio: Record<string, number> = {}
      if (data.group_ratio) {
        for (const [groupName, baseRatio] of Object.entries(data.group_ratio)) {
          let multiplier = 1.0
          const groupModels = data.group_model_ratio?.[groupName]
          if (groupModels) {
            const trimmedModel = model.model_name.trim()
            if (typeof groupModels[trimmedModel] === 'number') {
              multiplier = groupModels[trimmedModel]
            } else {
              let wildcardMatch = 1.0
              let wildcardFound = false
              let longestMatchLen = -1

              for (const [pattern, ratio] of Object.entries(groupModels)) {
                const trimmedPattern = pattern.trim()
                if (trimmedPattern === '*') {
                  if (longestMatchLen < 0) {
                    wildcardMatch = ratio
                    wildcardFound = true
                  }
                  continue
                }
                if (trimmedPattern.endsWith('*')) {
                  const prefix = trimmedPattern.slice(0, -1)
                  if (
                    trimmedModel.startsWith(prefix) &&
                    prefix.length > longestMatchLen
                  ) {
                    longestMatchLen = prefix.length
                    wildcardMatch = ratio
                    wildcardFound = true
                  }
                }
              }

              if (wildcardFound) {
                multiplier = wildcardMatch
              }
            }
          }
          effectiveGroupRatio[groupName] = baseRatio * multiplier
        }
      }

      return {
        ...model,
        key: model.model_name,
        vendor_name: vendor?.name,
        vendor_icon: vendor?.icon,
        vendor_description: vendor?.description,
        group_ratio: effectiveGroupRatio,
      }
    })
  }, [data])

  return {
    models,
    vendors: data?.vendors ?? [],
    groupRatio: data?.group_ratio ?? {},
    groupModelRatio: data?.group_model_ratio ?? {},
    usableGroup: data?.usable_group ?? {},
    endpointMap: data?.supported_endpoint ?? {},
    autoGroups: data?.auto_groups ?? [],
    isLoading,
    error,
    refetch,
    priceRate,
    usdExchangeRate,
  }
}
