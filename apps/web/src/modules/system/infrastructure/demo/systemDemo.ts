/* eslint @typescript-eslint/require-await: off */
import type { SystemPorts } from '@/modules/system/application/ports'

export function createSystemDemo(): SystemPorts {
  return { health: async () => ({ status: 'ok', checks: [], jellyfin: { state: 'unconfigured' } }) }
}
