/** The adapter set the composition root wires in: the Go API or the in-browser demo. */
export type DataSource = 'http' | 'demo'

/**
 * Resolves VITE_DATA_SOURCE. Anything other than an explicit "demo" falls back
 * to the HTTP adapters, which is what production runs.
 */
export function resolveDataSource(raw: string | undefined): DataSource {
  return raw === 'demo' ? 'demo' : 'http'
}

export const dataSource: DataSource = resolveDataSource(import.meta.env.VITE_DATA_SOURCE)
