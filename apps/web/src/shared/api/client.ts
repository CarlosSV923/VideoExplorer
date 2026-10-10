import createClient, { type Client } from 'openapi-fetch'

import type { components, paths } from './schema'

/** Typed client for the Go API. Only the HTTP adapters (infrastructure/http) use it. */
export type ApiClient = Client<paths>

export type ApiSchemas = components['schemas']

/**
 * Same-origin `/api`: Vite proxies it in development and the Go binary serves
 * both the SPA and the API in production.
 */
export function createApiClient(
  options: { baseUrl?: string; fetch?: typeof fetch } = {},
): ApiClient {
  return createClient<paths>({
    baseUrl: options.baseUrl ?? '/api',
    ...(options.fetch ? { fetch: options.fetch } : {}),
  })
}
