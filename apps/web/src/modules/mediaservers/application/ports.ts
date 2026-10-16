import { createPortContext } from '@/shared/kernel/ports'

import type { Settings } from '../domain/types'

export interface MediaServersPorts {
  settings(): Promise<Settings>
}

export const [MediaServersContext, useMediaServersPorts] =
  createPortContext<MediaServersPorts>('MediaServers')
