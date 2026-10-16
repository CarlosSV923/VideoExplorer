import { useQuery } from '@tanstack/react-query'

import { useMediaServersPorts } from './ports'

export const settingsKey = ['settings'] as const

export function useSettings() {
  const ports = useMediaServersPorts()
  return useQuery({ queryKey: settingsKey, queryFn: () => ports.settings() })
}
