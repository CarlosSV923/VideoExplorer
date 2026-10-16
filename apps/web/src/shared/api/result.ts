import { AppError, type AppErrorKind } from '@/shared/kernel/errors'

const statusKinds: Record<number, AppErrorKind> = {
  400: 'badRequest',
  401: 'unauthorized',
  404: 'notFound',
  409: 'conflict',
  422: 'validation',
  429: 'rateLimited',
  507: 'insufficientStorage',
}

function problemFields(problem: unknown): { code?: string; detail?: string } {
  if (typeof problem !== 'object' || problem === null) return {}
  const record = problem as Record<string, unknown>
  return {
    ...(typeof record.code === 'string' ? { code: record.code } : {}),
    ...(typeof record.detail === 'string' ? { detail: record.detail } : {}),
  }
}

/** Maps RFC 9457 responses without ever using `detail` as the user message. */
export function problemError(status: number, problem: unknown): AppError {
  const fields = problemFields(problem)
  return new AppError(statusKinds[status] ?? (status >= 500 ? 'server' : 'badRequest'), {
    status,
    ...fields,
  })
}

interface Result<T> {
  data?: T
  error?: unknown
  response: Response
}

export async function unwrap<T>(request: Promise<Result<T>>): Promise<T> {
  let result: Result<T>
  try {
    result = await request
  } catch (error) {
    const detail = error instanceof Error ? error.message : undefined
    throw new AppError('network', detail === undefined ? {} : { detail })
  }
  if (result.error !== undefined || !result.response.ok) {
    throw problemError(result.response.status, result.error)
  }
  return result.data as T
}

export async function unwrapVoid(request: Promise<Result<unknown>>): Promise<void> {
  await unwrap(request)
}
