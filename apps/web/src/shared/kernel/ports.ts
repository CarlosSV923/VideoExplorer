import { createContext, use, type Context } from 'react'

/** Creates the context used to inject a module's application ports. */
export function createPortContext<T>(name: string): [Context<T | null>, () => T] {
  const context = createContext<T | null>(null)
  context.displayName = name
  const usePorts = () => {
    const ports = use(context)
    if (!ports) throw new Error(`${name} ports are not provided`)
    return ports
  }
  return [context, usePorts]
}
