import vectors from '../../../../../../contracts/version-labels.json'

import { versionLabels, type VersionMedia } from './versionLabel'

interface VersionCase {
  name: string
  input: {
    versions: {
      width: number
      height: number
      codec: string
      audio: { language: string; title: string }[]
    }[]
  }
  expected: { labels: string[]; incomplete: boolean[] }
}

describe('version label vectors (contracts/version-labels.json)', () => {
  it('has cases to check', () => {
    expect(vectors.versionLabel.cases.length).toBeGreaterThan(0)
  })

  it.each((vectors.versionLabel.cases as VersionCase[]).map((c) => [c.name, c] as const))(
    'version label: %s',
    (_, c) => {
      const versions: VersionMedia[] = c.input.versions.map((v) => ({
        width: v.width,
        height: v.height,
        codec: v.codec,
        audio: v.audio,
      }))
      const got = versionLabels(versions)
      expect(got.map((l) => l.text)).toEqual(c.expected.labels)
      expect(got.map((l) => l.incomplete)).toEqual(c.expected.incomplete)
    },
  )
})
