import type { ApiClient } from '@/shared/api/client'
import { unwrap } from '@/shared/api/result'

import type { SystemPorts } from '../../application/ports'

export function createSystemHttp(client: ApiClient): SystemPorts {
  return { health: () => unwrap(client.GET('/health')) }
}
