import { useQuery } from '@tanstack/react-query'

import { createPortContext } from '@/shared/kernel/ports'

import type { Health } from '../domain/types'

export interface SystemPorts {
  health(): Promise<Health>
}

export const [SystemContext, useSystemPorts] = createPortContext<SystemPorts>('System')

export const healthKey = ['health'] as const

export function useHealth() {
  const ports = useSystemPorts()
  return useQuery({
    queryKey: healthKey,
    queryFn: () => ports.health(),
    enabled: typeof ports.health === 'function',
    refetchInterval: 60_000,
  })
}
