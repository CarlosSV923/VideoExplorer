import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { useMetadataPorts } from './ports'

export const titleSearchKey = (query: string, year?: number) =>
  ['titles', 'search', query, year ?? null] as const

export function useTitleSearch(query: string, year?: number) {
  const ports = useMetadataPorts()
  return useQuery({
    queryKey: titleSearchKey(query, year),
    queryFn: () => ports.searchTitles(query, year),
    enabled: query.trim().length > 0,
  })
}

export function useRefreshTitleMetadata(titleId: number) {
  const ports = useMetadataPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => ports.refreshTitleMetadata(titleId),
    onSuccess: (title) => {
      queryClient.setQueryData(['library', 'titles', title.id], title)
      void queryClient.invalidateQueries({ queryKey: ['library', 'titles'] })
    },
  })
}
