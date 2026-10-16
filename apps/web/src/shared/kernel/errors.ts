/** Errors exposed by every application port, independent of its adapter. */
export type AppErrorKind =
  | 'unauthorized'
  | 'conflict'
  | 'notFound'
  | 'validation'
  | 'badRequest'
  | 'insufficientStorage'
  | 'rateLimited'
  | 'network'
  | 'server'

/**
 * The backend detail is retained for diagnostics only. UI code must translate
 * `kind` and `code`; `message` never contains the backend's English detail.
 */
export class AppError extends Error {
  readonly kind: AppErrorKind
  readonly code: string | undefined
  readonly status: number | undefined
  readonly detail: string | undefined

  constructor(
    kind: AppErrorKind,
    options: { code?: string; status?: number; detail?: string } = {},
  ) {
    super(options.code ?? kind)
    this.name = 'AppError'
    this.kind = kind
    this.code = options.code
    this.status = options.status
    this.detail = options.detail
  }
}

export function isAppError(error: unknown, kind?: AppErrorKind): error is AppError {
  return error instanceof AppError && (kind === undefined || error.kind === kind)
}

export function toAppError(error: unknown): AppError {
  if (error instanceof AppError) return error
  const detail = error instanceof Error ? error.message : undefined
  return new AppError('network', detail === undefined ? {} : { detail })
}
