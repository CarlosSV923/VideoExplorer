import type { ApiClient } from '@/shared/api/client'
import { unwrap, unwrapVoid } from '@/shared/api/result'

import type { IdentityPorts } from '../../application/ports'

export function createIdentityHttp(client: ApiClient): IdentityPorts {
  return {
    session: () => unwrap(client.GET('/auth/session')),
    login: (password) => unwrapVoid(client.POST('/auth/login', { body: { password } })),
    logout: () => unwrapVoid(client.POST('/auth/logout')),
  }
}
