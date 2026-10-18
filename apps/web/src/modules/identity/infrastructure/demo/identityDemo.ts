/* eslint @typescript-eslint/require-await: off */
import type { IdentityPorts } from '@/modules/identity/application/ports'

export function createIdentityDemo(): IdentityPorts {
  return {
    session: async () => ({ expiresAt: '2999-01-01T00:00:00.000Z' }),
    login: async () => undefined,
    logout: async () => undefined,
  }
}
