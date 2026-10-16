import type { ApiClient } from '@/shared/api/client'
import { unwrap } from '@/shared/api/result'

import type { MediaServersPorts } from '../../application/ports'

export function createMediaServersHttp(client: ApiClient): MediaServersPorts {
  return { settings: () => unwrap(client.GET('/settings')) }
}
