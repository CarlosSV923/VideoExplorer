import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'

import { useCatalogPorts } from './ports'

export const libraryTitlesKey = ['library', 'titles'] as const
export const libraryTitleKey = (id: number) => ['library', 'titles', id] as const
export const libraryCollectionKey = (id: number) => ['library', 'collections', id] as const
export const unassignedKey = ['unassigned'] as const
export const trashKey = ['trash'] as const
export const scansKey = ['scans'] as const
export const liveQueryOptions = {
  staleTime: 0,
  refetchInterval: false,
  refetchOnWindowFocus: false,
  refetchOnReconnect: false,
} as const

const invalidationBatches = new WeakMap<QueryClient, Set<string>>()
const invalidationTimers = new WeakMap<QueryClient, ReturnType<typeof setTimeout>>()

function invalidateSafely(queryClient: QueryClient, queryKey: readonly unknown[]) {
  void queryClient.invalidateQueries({ queryKey }).catch(() => undefined)
}

export function scheduleCatalogInvalidation(
  queryClient: QueryClient,
  queryKeys: readonly (readonly unknown[])[],
  delayMs = 500,
) {
  const keys = invalidationBatches.get(queryClient) ?? new Set<string>()
  for (const queryKey of queryKeys) keys.add(JSON.stringify(queryKey))
  invalidationBatches.set(queryClient, keys)
  if (invalidationTimers.has(queryClient)) return
  invalidationTimers.set(
    queryClient,
    setTimeout(() => {
      invalidationTimers.delete(queryClient)
      const pending = invalidationBatches.get(queryClient)
      invalidationBatches.delete(queryClient)
      for (const key of pending ?? []) {
        const queryKey = JSON.parse(key) as readonly unknown[]
        invalidateSafely(queryClient, queryKey)
      }
    }, delayMs),
  )
}

export function invalidateCatalogQueries(queryClient: QueryClient) {
  invalidateSafely(queryClient, ['library'])
  invalidateSafely(queryClient, unassignedKey)
  invalidateSafely(queryClient, trashKey)
}

export function useLibraryTitles() {
  const ports = useCatalogPorts()
  return useQuery({ queryKey: libraryTitlesKey, queryFn: () => ports.libraryTitles() })
}

export function useLibraryTitle(id: number) {
  const ports = useCatalogPorts()
  return useQuery({ queryKey: libraryTitleKey(id), queryFn: () => ports.libraryTitle(id) })
}

export function useLibraryCollection(id: number) {
  const ports = useCatalogPorts()
  return useQuery({
    queryKey: libraryCollectionKey(id),
    queryFn: () => ports.libraryCollection(id),
  })
}

export function useUnassigned() {
  const ports = useCatalogPorts()
  return useQuery({
    queryKey: unassignedKey,
    queryFn: () => ports.unassigned(),
    ...liveQueryOptions,
  })
}

export function useInvalidateCatalog() {
  const queryClient = useQueryClient()
  return () => {
    invalidateCatalogQueries(queryClient)
  }
}

export function useCommitAssign(entryId: number, token: string) {
  const ports = useCatalogPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (request: Parameters<typeof ports.commitAssign>[2]) =>
      ports.commitAssign(entryId, token, request),
    onSuccess: () => {
      invalidateCatalogQueries(queryClient)
    },
  })
}

export function useCommitTitleEdit(titleId: number) {
  const ports = useCatalogPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (request: Parameters<typeof ports.commitTitleEdit>[1]) =>
      ports.commitTitleEdit(titleId, request),
    onSuccess: (result) => {
      invalidateCatalogQueries(queryClient)
      invalidateSafely(queryClient, libraryTitleKey(result.titleId))
    },
  })
}

export function useRunScans() {
  const ports = useCatalogPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => ports.runScans(),
    onSuccess: (scans) => queryClient.setQueryData(scansKey, scans),
  })
}

export function useScans() {
  const ports = useCatalogPorts()
  return useQuery({ queryKey: scansKey, queryFn: () => ports.scans() })
}

export function useTrash() {
  const ports = useCatalogPorts()
  return useQuery({ queryKey: trashKey, queryFn: () => ports.trash() })
}

export function useEmptyTrash() {
  const ports = useCatalogPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => ports.emptyTrash(),
    onSuccess: () => {
      invalidateSafely(queryClient, trashKey)
    },
  })
}

export function useDeleteTrashEntry() {
  const ports = useCatalogPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => ports.deleteTrashEntry(id),
    onSuccess: () => {
      invalidateSafely(queryClient, trashKey)
    },
  })
}

export function usePlanTrashRestore() {
  const ports = useCatalogPorts()
  return useMutation({ mutationFn: (id: number) => ports.planTrashRestore(id) })
}

export function useRestoreTrashEntry() {
  const ports = useCatalogPorts()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      id,
      request,
    }: {
      id: number
      request: Parameters<typeof ports.restoreTrashEntry>[1]
    }) => ports.restoreTrashEntry(id, request),
    onSuccess: () => {
      invalidateCatalogQueries(queryClient)
    },
  })
}
