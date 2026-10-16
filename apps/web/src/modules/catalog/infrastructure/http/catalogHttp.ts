import type { ApiClient } from '@/shared/api/client'
import { unwrap, unwrapVoid } from '@/shared/api/result'

import type { CatalogPorts } from '../../application/ports'

export function createCatalogHttp(client: ApiClient, baseUrl = '/api'): CatalogPorts {
  const title = (id: number) => ({ params: { path: { id } } })
  const file = (titleId: number, fileId: number) => ({
    params: { path: { id: titleId, fileId } },
  })
  const entry = (id: number) => ({ params: { path: { id } } })
  const lock = (id: number, token?: string) => ({
    ...entry(id),
    ...(token ? { headers: { 'X-Assign-Lock': token } } : {}),
  })
  const url = (path: string) => `${baseUrl}${path}`

  return {
    libraryTitles: () => unwrap(client.GET('/library/titles')),
    libraryTitle: (id) => unwrap(client.GET('/library/titles/{id}', title(id))),
    libraryCollection: (id) => unwrap(client.GET('/library/collections/{id}', title(id))),
    fileTechnical: (titleId, fileId) =>
      unwrap(client.GET('/library/titles/{id}/files/{fileId}/technical', file(titleId, fileId))),
    downloadTitleUrl: (titleId) => url(`/library/titles/${String(titleId)}/download`),
    downloadSeasonUrl: (titleId, season) =>
      url(`/library/titles/${String(titleId)}/seasons/${String(season)}/download`),
    downloadFileUrl: (titleId, fileId) =>
      url(`/library/titles/${String(titleId)}/files/${String(fileId)}/download`),

    planTitleEdit: (id, body) =>
      unwrap(client.POST('/library/titles/{id}/edit/plan', { ...title(id), body })),
    commitTitleEdit: (id, body) =>
      unwrap(client.POST('/library/titles/{id}/edit/commit', { ...title(id), body })),
    planTitleIDTag: (id, body) =>
      unwrap(client.POST('/library/titles/{id}/id-tag/plan', { ...title(id), body })),
    commitTitleIDTag: (id, body) =>
      unwrap(client.POST('/library/titles/{id}/id-tag/commit', { ...title(id), body })),
    moveTitleToUnassigned: (id, body) =>
      unwrapVoid(client.POST('/library/titles/{id}/unassigned', { ...title(id), body })),
    trashTitleScope: (id, body) =>
      unwrapVoid(client.POST('/library/titles/{id}/trash', { ...title(id), body })),

    unassigned: () => unwrap(client.GET('/unassigned')),
    takeAssignLock: (id, token) => unwrap(client.POST('/unassigned/{id}/lock', lock(id, token))),
    releaseAssignLock: (id, token) =>
      unwrapVoid(client.DELETE('/unassigned/{id}/lock', lock(id, token))),
    assignSuggestions: (id, token) =>
      unwrap(client.GET('/unassigned/{id}/suggestions', lock(id, token))),
    planAssign: (id, token, body) =>
      unwrap(client.POST('/unassigned/{id}/plan', { ...lock(id, token), body })),
    commitAssign: (id, token, body) =>
      unwrap(client.POST('/unassigned/{id}/commit', { ...lock(id, token), body })),
    trashUnassignedEntry: (id) => unwrapVoid(client.POST('/unassigned/{id}/trash', entry(id))),
    deleteUnassignedEntry: (id) => unwrapVoid(client.POST('/unassigned/{id}/delete', entry(id))),
    trashUnassignedFile: (id, body) =>
      unwrapVoid(client.POST('/unassigned/{id}/files/trash', { ...entry(id), body })),
    deleteUnassignedFile: (id, body) =>
      unwrapVoid(client.POST('/unassigned/{id}/files/delete', { ...entry(id), body })),
    downloadUnassignedEntryUrl: (id) => url(`/unassigned/${String(id)}/download`),
    downloadUnassignedFileUrl: (id, path) =>
      url(`/unassigned/${String(id)}/files/download?path=${encodeURIComponent(path)}`),

    scans: () => unwrap(client.GET('/scans')),
    runScans: () => unwrap(client.POST('/scans')),
    trash: () => unwrap(client.GET('/trash')),
    emptyTrash: () => unwrapVoid(client.DELETE('/trash')),
    deleteTrashEntry: (id) => unwrapVoid(client.DELETE('/trash/{id}', entry(id))),
    planTrashRestore: (id) => unwrap(client.POST('/trash/{id}/restore/plan', entry(id))),
    restoreTrashEntry: (id, body) =>
      unwrap(client.POST('/trash/{id}/restore', { ...entry(id), body })),
  }
}
