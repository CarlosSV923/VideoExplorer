import vectors from '../../../../../../contracts/filename-cases.json'

import { analyzeFile, type FileAnalysis, type MediaKind } from './filename'

interface FilenameCase {
  name: string
  input: { path: string; kind: MediaKind }
  expected: FileAnalysis
}

const sections: Record<string, { cases: FilenameCase[] }> = {
  videos: { cases: vectors.videos.cases as FilenameCase[] },
  folders: { cases: vectors.folders.cases as FilenameCase[] },
  subtitles: { cases: vectors.subtitles.cases as FilenameCase[] },
  unknown: { cases: vectors.unknown.cases as FilenameCase[] },
}

describe('filename vectors (contracts/filename-cases.json)', () => {
  for (const [section, { cases }] of Object.entries(sections)) {
    it.each(cases.map((c) => [c.name, c] as const))(`${section}: %s`, (_, c) => {
      expect(analyzeFile(c.input.path, c.input.kind)).toStrictEqual(c.expected)
    })
  }
})
