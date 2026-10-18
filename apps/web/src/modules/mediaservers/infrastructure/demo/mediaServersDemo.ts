/* eslint @typescript-eslint/require-await: off */
import type { MediaServersPorts } from '@/modules/mediaservers/application/ports'

export function createMediaServersDemo(): MediaServersPorts {
  return {
    settings: async () => ({
      jellyfin: {
        state: 'unconfigured',
        translationSource: 'none',
        translation: { libraryPath: '', movies: '', series: '' },
      },
    }),
  }
}
