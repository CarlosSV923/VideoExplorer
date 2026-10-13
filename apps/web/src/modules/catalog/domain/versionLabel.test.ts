import vectors from '../../../../../../contracts/version-labels.json'

import { NameError } from './errors'
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
  expected: string[] | { error: string }
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
      if (Array.isArray(c.expected)) {
        expect(versionLabels(versions)).toEqual(c.expected)
        return
      }
      let caught: unknown
      try {
        versionLabels(versions)
      } catch (err) {
        caught = err
      }
      expect(caught).toBeInstanceOf(NameError)
      expect((caught as NameError).field).toBe(c.expected.error)
    },
  )
})
