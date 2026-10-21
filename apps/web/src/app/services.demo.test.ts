import { createDemoServices } from './services'
import type { EditRequest } from '@/modules/catalog/domain/types'

describe('VideoExplorer demo adapters', () => {
  it('wires every module to in-memory ports and seeds RF-71a titles, entries, and trash', async () => {
    const services = await createDemoServices()
    await expect(services.identity.session()).resolves.toHaveProperty('expiresAt')
    await expect(services.mediaServers.settings()).resolves.toMatchObject({
      jellyfin: { state: 'unconfigured' },
    })
    const { titles } = await services.catalog.libraryTitles()
    expect(titles.map((item) => item.tmdbId)).toEqual(
      expect.arrayContaining([
        8587, 11430, 272, 155, 1726, 10138, 68721, 438631, 27205, 129, 194, 496243, 597, 1396,
        70523, 87108, 97546, 71446, 93405,
      ]),
    )
    const titanic = titles.find((item) => item.tmdbId === 597)
    const dune = titles.find((item) => item.tmdbId === 438631)
    expect(titanic).toBeDefined()
    expect(dune).toBeDefined()
    if (!titanic || !dune) throw new Error('Expected seeded movie titles')
    expect((await services.catalog.libraryTitle(titanic.id)).videos).toHaveLength(2)
    expect((await services.catalog.libraryTitle(dune.id)).videos).toHaveLength(2)
    expect((await services.catalog.unassigned()).entries.map((item) => item.name)).toEqual(
      expect.arrayContaining([
        'Ted.Lasso.S02.1080p.WEB',
        'Mufasa.The.Lion.King.2024.2160p.mkv',
        'Get.Out.2017.1080p',
        'The.Dark.Knight.2008.1080p.mkv',
        'Entrada vacía',
      ]),
    )
    expect((await services.catalog.trash()).entries).toHaveLength(2)
    expect((await services.catalog.scans()).scans).toHaveLength(2)
  })

  it('simulates own archives without a password and emits job events before creating an unassigned entry', async () => {
    vi.useFakeTimers()
    try {
      const services = await createDemoServices()
      const batch = await services.ingestion.createBatch({ relation: 'none' })
      const events: string[] = []
      const stop = services.ingestion.watchJobs({ onJob: (job) => events.push(job.status) })
      let jobId = ''
      services.ingestion.upload?.(
        new File([new Uint8Array(20)], 'My.Movie.rar'),
        { batchId: batch.id, relation: 'none', fileName: 'My.Movie.rar' },
        {
          onJobId: (id) => {
            jobId = id
          },
          onProgress: () => undefined,
          onSuccess: () => undefined,
          onError: (error) => {
            throw error
          },
        },
      )
      await vi.runAllTimersAsync()
      expect(events).toEqual(
        expect.arrayContaining(['uploading', 'uploaded', 'extracting', 'filtering', 'done']),
      )
      expect(await services.ingestion.jobs()).toEqual(
        expect.arrayContaining([expect.objectContaining({ id: jobId, status: 'done' })]),
      )
      expect((await services.catalog.unassigned()).entries).toEqual(
        expect.arrayContaining([expect.objectContaining({ name: 'My.Movie', origin: 'upload' })]),
      )
      stop()
    } finally {
      vi.useRealTimers()
    }
  })

  it('returns fresh Unassigned snapshots after restoring an upload from trash', async () => {
    const services = await createDemoServices()
    const before = await services.catalog.unassigned()

    await services.catalog.restoreTrashEntry(2, { decisions: [] })

    const after = await services.catalog.unassigned()
    expect(after).not.toBe(before)
    expect(after.entries).not.toBe(before.entries)
    expect(after.entries).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          name: 'Inception.2010.part1.rar',
          origin: 'upload',
        }),
      ]),
    )
  })

  it('joins sample parts, asks for the documented sample password, and filters the Sample folder', async () => {
    vi.useFakeTimers()
    try {
      const services = await createDemoServices()
      const batch = await services.ingestion.createBatch({
        relation: 'parts',
        name: 'sample:Inception',
      })
      const uploadPart = (fileName: string) => {
        let id = ''
        services.ingestion.upload?.(
          new File([new Uint8Array([1])], fileName),
          { batchId: batch.id, relation: 'parts', fileName },
          {
            onJobId: (value) => {
              id = value
            },
            onProgress: () => undefined,
            onSuccess: () => undefined,
            onError: (error) => {
              throw error
            },
          },
        )
        return () => id
      }
      const first = uploadPart('Inception.2010.part1.rar')
      uploadPart('Inception.2010.part2.rar')
      await vi.runAllTimersAsync()
      const needsPassword = (await services.ingestion.jobs()).find((job) => job.id === first())
      expect(needsPassword?.status).toBe('needs_password')
      expect(
        (await services.ingestion.jobs())
          .filter((job) => job.batch.id === batch.id)
          .map((job) => job.status),
      ).toEqual(['needs_password', 'needs_password'])
      if (!needsPassword) throw new Error('Expected sample archive job')
      await services.ingestion.submitPassword(needsPassword.id, 'demo')
      await vi.runAllTimersAsync()
      expect(
        (await services.ingestion.jobs()).find((job) => job.id === needsPassword.id)?.status,
      ).toBe('done')

      const sampleBatch = await services.ingestion.createBatch({ relation: 'none' })
      let sampleJobId = ''
      const sample = new File([new Uint8Array([1])], 'video.mkv')
      services.ingestion.upload?.(
        sample,
        {
          batchId: sampleBatch.id,
          relation: 'none',
          fileName: sample.name,
          relativePath: 'Sample/video.mkv',
        },
        {
          onJobId: (value) => {
            sampleJobId = value
          },
          onProgress: () => undefined,
          onSuccess: () => undefined,
          onError: (error) => {
            throw error
          },
        },
      )
      await vi.runAllTimersAsync()
      expect((await services.ingestion.jobs()).find((job) => job.id === sampleJobId)?.status).toBe(
        'discarded',
      )
      expect((await services.catalog.trash()).entries).toEqual(
        expect.arrayContaining([
          expect.objectContaining({ reason: 'filtered', relativePath: 'Sample/video.mkv' }),
        ]),
      )
      const samba = (await services.ingestion.jobs()).find(
        (job) => job.id === 'demo-samba-job-x7k2',
      )
      expect(samba?.status).toBe('needs_password')
      await services.ingestion.submitPassword('demo-samba-job-x7k2', 'demo')
      await vi.runAllTimersAsync()
      expect((await services.ingestion.jobs()).find((job) => job.id === samba?.id)?.status).toBe(
        'done',
      )
      expect((await services.catalog.unassigned()).entries).toEqual(
        expect.arrayContaining([expect.objectContaining({ name: 'x7k2', origin: 'upload' })]),
      )
      expect((await services.catalog.unassigned()).jobs).toHaveLength(0)
    } finally {
      vi.useRealTimers()
    }
  })

  it('suggests the seeded library duplicate, previews its choice, and saves in memory', async () => {
    const services = await createDemoServices()
    const lock = await services.catalog.takeAssignLock(4)
    const suggestions = await services.catalog.assignSuggestions(4, lock.token)
    expect(suggestions.videos[0]?.match?.source).toBe('library')
    const titleId = suggestions.videos[0]?.match?.libraryTitleId
    if (!titleId) throw new Error('Expected a library title suggestion')
    const file = suggestions.videos[0]?.file
    if (!file) throw new Error('Expected a video suggestion')
    const request = {
      videos: [
        {
          file,
          action: 'place' as const,
          title: { source: 'library' as const, kind: 'movie' as const, libraryTitleId: titleId },
          part: 0,
          duplicate: 'keep_both' as const,
        },
      ],
      subtitles: [],
    }
    const assignment = request.videos[0]
    if (!assignment) throw new Error('Expected an assignment choice')
    const plan = await services.catalog.planAssign(4, lock.token, request)
    expect(plan.valid).toBe(true)
    expect(plan.videos[0]?.duplicates).toHaveLength(1)
    expect(plan.videos[0]?.duplicates[0]?.version).toBe('2160p')
    expect(plan.videos[0]?.versionIncomplete).toBe(false)
    expect(plan.videos[0]?.duplicates[0]?.rel).toContain('The Dark Knight (2008) [tmdbid-155]')
    const unknownAudioPlan = await services.catalog.planAssign(4, lock.token, {
      ...request,
      videos: [{ ...assignment, file: `${file}.und`, duplicate: 'keep_both' }],
    })
    expect(unknownAudioPlan.videos[0]?.versionIncomplete).toBe(true)
    const saved = await services.catalog.commitAssign(4, lock.token, {
      ...request,
      videos: [{ ...assignment, duplicate: 'replace' }],
    })
    expect(saved.entryRemoved).toBe(true)
    expect((await services.catalog.unassigned()).entries.some((entry) => entry.id === 4)).toBe(
      false,
    )
  })

  it('loads demo artwork only from the TMDB image CDN and makes no API requests', async () => {
    const fetchSpy = vi.spyOn(globalThis, 'fetch')
    try {
      const services = await createDemoServices()
      const image = services.metadata.imageUrl('poster', '8587.svg')
      expect(image).toMatch(/^https:\/\/image\.tmdb\.org\/t\/p\/w500\//)
      await services.catalog.libraryTitles()
      await services.system.health()
      await services.mediaServers.settings()
      await services.ingestion.jobs()
      expect(fetchSpy).not.toHaveBeenCalled()
    } finally {
      fetchSpy.mockRestore()
    }
  })

  it('previews a free title linked to TMDB with the canonical tagged folder and video name', async () => {
    const services = await createDemoServices()
    const title = await services.catalog.libraryTitle(999)
    const video = title.videos[0]
    if (!video) throw new Error('Expected the seeded free title video')
    const request: EditRequest = {
      revision: '',
      title: { source: 'tmdb', tmdbId: 155, kind: 'movie' },
      videos: [{ fileId: video.id, version: '', part: 0 }],
      subtitles: [],
    }
    const plan = await services.catalog.planTitleEdit(999, request)
    expect(plan.folder).toContain('The Dark Knight (2008) [tmdbid-155]')
    expect(plan.videos[0]?.file).toBe('Vacaciones en la playa (2015).mkv')
    expect(plan.videos[0]?.finalPath).toContain(
      'The Dark Knight (2008) [tmdbid-155]/The Dark Knight (2008) [tmdbid-155].mkv',
    )
    expect(plan.videos[0]?.changed).toBe(true)
    await services.catalog.commitTitleEdit(999, request)
    const saved = await services.catalog.libraryTitle(999)
    expect(saved.tmdbId).toBe(155)
    expect(saved.videos[0]?.path).toBe(plan.videos[0]?.finalPath)
  })
})
