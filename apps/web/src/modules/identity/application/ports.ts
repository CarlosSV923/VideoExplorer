import { createPortContext } from '@/shared/kernel/ports'

import type { Session } from '../domain/types'

export interface IdentityPorts {
  session(): Promise<Session>
  login(password: string): Promise<void>
  logout(): Promise<void>
}

export const [IdentityContext, useIdentityPorts] = createPortContext<IdentityPorts>('Identity')
