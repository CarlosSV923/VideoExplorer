import type { TFunction } from 'i18next'

import type { Kind } from '../application/search'

export function kindLabel(kind: Kind, t: TFunction) {
  return t(kind === 'movie' ? 'library.movie' : 'library.series')
}
