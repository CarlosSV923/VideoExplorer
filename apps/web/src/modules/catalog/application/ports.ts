import { createPortContext } from '@/shared/kernel/ports'

import type {
  AssignLock,
  AssignPlan,
  AssignRequest,
  AssignResult,
  AssignSuggestions,
  DownloadUrl,
  EditPlan,
  EditRequest,
  EditResult,
  IDTagRequest,
  LibraryFileTechnical,
  LibraryCollection,
  LibraryScope,
  LibraryTitle,
  LibraryTitles,
  ScanStatuses,
  TrashList,
  TrashRestorePlan,
  TrashRestoreRequest,
  TrashRestoreResult,
  UnassignedFileRequest,
  UnassignedSection,
} from '../domain/types'

export interface CatalogPorts {
  libraryTitles(): Promise<LibraryTitles>
  libraryTitle(id: number): Promise<LibraryTitle>
  libraryCollection(id: number): Promise<LibraryCollection>
  fileTechnical(titleId: number, fileId: number): Promise<LibraryFileTechnical>
  downloadTitleUrl(titleId: number): DownloadUrl
  downloadSeasonUrl(titleId: number, season: number): DownloadUrl
  downloadFileUrl(titleId: number, fileId: number): DownloadUrl

  planTitleEdit(titleId: number, request: EditRequest): Promise<EditPlan>
  commitTitleEdit(titleId: number, request: EditRequest): Promise<EditResult>
  planTitleIDTag(titleId: number, request: IDTagRequest): Promise<EditPlan>
  commitTitleIDTag(titleId: number, request: IDTagRequest): Promise<EditResult>
  moveTitleToUnassigned(titleId: number, scope: LibraryScope): Promise<void>
  trashTitleScope(titleId: number, scope: LibraryScope): Promise<void>

  unassigned(): Promise<UnassignedSection>
  takeAssignLock(entryId: number, token?: string): Promise<AssignLock>
  releaseAssignLock(
    entryId: number,
    token: string,
    options?: { keepalive?: boolean },
  ): Promise<void>
  assignSuggestions(entryId: number, token: string): Promise<AssignSuggestions>
  planAssign(entryId: number, token: string, request: AssignRequest): Promise<AssignPlan>
  commitAssign(entryId: number, token: string, request: AssignRequest): Promise<AssignResult>
  trashUnassignedEntry(entryId: number): Promise<void>
  deleteUnassignedEntry(entryId: number): Promise<void>
  trashUnassignedFile(entryId: number, request: UnassignedFileRequest): Promise<void>
  deleteUnassignedFile(entryId: number, request: UnassignedFileRequest): Promise<void>
  downloadUnassignedEntryUrl(entryId: number): DownloadUrl
  downloadUnassignedFileUrl(entryId: number, path: string): DownloadUrl

  scans(): Promise<ScanStatuses>
  runScans(): Promise<ScanStatuses>
  trash(): Promise<TrashList>
  emptyTrash(): Promise<void>
  deleteTrashEntry(id: number): Promise<void>
  planTrashRestore(id: number): Promise<TrashRestorePlan>
  restoreTrashEntry(id: number, request: TrashRestoreRequest): Promise<TrashRestoreResult>
}

export const [CatalogContext, useCatalogPorts] = createPortContext<CatalogPorts>('Catalog')
