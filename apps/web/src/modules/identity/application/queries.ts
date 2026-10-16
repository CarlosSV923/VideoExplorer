import { useQuery } from '@tanstack/react-query'

import { useIdentityPorts } from './ports'

export const sessionKey = ['session'] as const

export function useSession() {
  const ports = useIdentityPorts()
  return useQuery({ queryKey: sessionKey, queryFn: () => ports.session(), retry: false })
}

export function sessionQuery(ports: ReturnType<typeof useIdentityPorts>) {
  return { queryKey: sessionKey, queryFn: () => ports.session(), retry: false as const }
}
