import { createPortContext } from '@/shared/kernel/ports'

import type { ImageKind, LibraryTitle, TitleSearch } from '../domain/types'

export interface MetadataPorts {
  searchTitles(query: string, year?: number): Promise<TitleSearch>
  loadTitleMetadata(titleId: number): Promise<LibraryTitle>
  refreshTitleMetadata(titleId: number): Promise<LibraryTitle>
  imageUrl(kind: ImageKind, file: string): string
}

export const [MetadataContext, useMetadataPorts] = createPortContext<MetadataPorts>('Metadata')
