import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  initialSubtitleRequest,
  initialVideoRequest,
  mergeTitleIntoVideos,
  saveState,
  subtitlesByVideo,
  titleChoiceFromHit,
} from '../application/assignment'
import { useCatalogPorts } from '../application/ports'
import type { AssignPlan, AssignRequest, AssignSuggestions, AssignVideo } from '../domain/types'
import { LibraryLayout } from '@/app/LibraryLayout'
import { isAppError } from '@/shared/kernel/errors'
import { useAction, useGamepadConnected } from '@/shared/input'
import { Banner, Button, ConfirmDialog, HelpBar, PageHeader, ShortcutBar, Sheet } from '@/shared/ui'
import { AssignVideoCard } from './AssignVideoCard'
import { PlanPreview } from './PlanPreview'
import { SubtitleList } from './SubtitleList'
import { TitlePicker } from './TitlePicker'

function conflictMessage(error: unknown, translate: (key: string) => string): string | undefined {
  if (!isAppError(error)) return undefined
  if (error.code === 'assigning') return translate('assign.errors.assigning')
  if (error.code === 'lock_lost') return translate('assign.errors.lockLost')
  if (error.code === 'plan_changed') return translate('assign.errors.planChanged')
  if (error.kind === 'validation') return translate('assign.errors.validation')
  return translate('assign.errors.generic')
}

export function AssignScreen({ id }: { id: number }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const ports = useCatalogPorts()
  const [token, setToken] = useState<string | null>(null)
  const [suggestions, setSuggestions] = useState<AssignSuggestions | null>(null)
  const [request, setRequest] = useState<AssignRequest | null>(null)
  const [plan, setPlan] = useState<AssignPlan | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [loading, setLoading] = useState(true)
  const [focused, setFocused] = useState(0)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [confirmCancel, setConfirmCancel] = useState(false)
  const [dirty, setDirty] = useState(false)
  const releaseLock = useRef<((options?: { keepalive?: boolean }) => void) | null>(null)
  const gamepad = useGamepadConnected()

  const goToUnassigned = useCallback(() => {
    try {
      void navigate({ to: '/no-asignados' as never })
    } catch {
      window.history.pushState({}, '', '/no-asignados')
    }
  }, [navigate])

  useEffect(() => {
    let active = true
    let acquired: string | null = null
    let released = false
    let pageHidden = false
    const release = (options?: { keepalive?: boolean }) => {
      if (!acquired || released) return
      released = true
      void (options
        ? ports.releaseAssignLock(id, acquired, options)
        : ports.releaseAssignLock(id, acquired))
    }
    const onPageHide = () => {
      pageHidden = true
      release({ keepalive: true })
    }
    window.addEventListener('pagehide', onPageHide)
    releaseLock.current = release
    void ports
      .takeAssignLock(id)
      .then((lock) => {
        if (!active || pageHidden) {
          void (pageHidden
            ? ports.releaseAssignLock(id, lock.token, { keepalive: true })
            : ports.releaseAssignLock(id, lock.token))
          return
        }
        acquired = lock.token
        setToken(lock.token)
        return ports.assignSuggestions(id, lock.token).then((result) => {
          if (!active) return
          setSuggestions(result)
          const videos = initialVideoRequest(result.videos)
          const subtitles = initialSubtitleRequest(result.subtitles)
          const withSeries =
            result.relation === 'episodes' && result.title
              ? mergeTitleIntoVideos(videos, titleChoiceFromHit(result.title))
              : videos
          setRequest({ videos: withSeries, subtitles })
          setLoading(false)
        })
      })
      .catch((cause: unknown) => {
        if (active) {
          setError(cause)
          setLoading(false)
        }
      })
    return () => {
      active = false
      window.removeEventListener('pagehide', onPageHide)
      if (releaseLock.current === release) releaseLock.current = null
      release()
    }
  }, [id, ports])

  const group = useMemo(() => subtitlesByVideo(suggestions?.subtitles ?? []), [suggestions])
  const currentState = saveState(plan)
  const translate = (key: string) => t(key as never)
  const issueText = (code: string) =>
    t(`assign.issue.${code}`, { defaultValue: t('assign.issue.generic') })
  const issueTextFor = (field: string, file?: string) => {
    const issue = plan?.errors.find(
      (item) => item.field === field && (file === undefined ? !item.file : item.file === file),
    )
    return issue ? issueText(issue.code) : undefined
  }
  const saveReason = currentState.enabled
    ? undefined
    : t(`assign.saveReason.${currentState.reason}`)

  const updateVideo = (index: number, value: AssignVideo) => {
    setDirty(true)
    setRequest(
      (current) =>
        current && {
          ...current,
          videos: current.videos.map((item, itemIndex) => (itemIndex === index ? value : item)),
        },
    )
    setPlan(null)
  }
  const confirmAll = () => {
    setDirty(true)
    setRequest(
      (current) =>
        current && {
          ...current,
          videos: current.videos.map((video) => ({ ...video, action: 'place' })),
        },
    )
    setPlan(null)
  }
  const preview = () => {
    if (!token || !request) return
    setError(null)
    void ports.planAssign(id, token, request).then(setPlan).catch(setError)
  }
  const save = useCallback(() => {
    if (!token || !request || !plan || !currentState.enabled) return
    setError(null)
    void ports
      .commitAssign(id, token, request)
      .then(() => {
        goToUnassigned()
      })
      .catch(setError)
  }, [currentState.enabled, goToUnassigned, id, plan, ports, request, token])
  const cancel = () => {
    if (dirty) {
      setConfirmCancel(true)
      return
    }
    cancelNow()
  }
  const cancelNow = () => {
    if (token) releaseLock.current?.()
    goToUnassigned()
  }
  useAction(
    'back',
    () => {
      cancel()
      return true
    },
    Boolean(token),
  )
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Enter' && event.ctrlKey) {
        event.preventDefault()
        save()
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [save])

  if (loading)
    return (
      <LibraryLayout hideMobileNav>
        <div className="py-12 text-body text-ink-2">{t('common.loading')}</div>
      </LibraryLayout>
    )
  if (error && !suggestions)
    return (
      <LibraryLayout hideMobileNav>
        <PageHeader
          back={<Button onClick={cancel}>{t('library.unassigned')}</Button>}
          title={t('screens.assign.title')}
        />
        <div className="py-8">
          <Banner tone="danger">{conflictMessage(error, translate) ?? t('errors.loadBody')}</Banner>
        </div>
      </LibraryLayout>
    )
  if (!suggestions || !request) return null

  return (
    <LibraryLayout hideMobileNav>
      <PageHeader
        back={<Button onClick={cancel}>{t('library.unassigned')}</Button>}
        title={
          <>
            {t('screens.assign.title')}{' '}
            <span className="font-mono-file text-title font-normal">{suggestions.name}</span>
          </>
        }
        actions={
          <Button variant="outline-accent" onClick={confirmAll}>
            {t('assign.confirmAll')}
          </Button>
        }
      />
      {!suggestions.tmdbConfigured && <Banner tone="warning">{t('assign.tmdbUnavailable')}</Banner>}
      {error ? (
        <div className="page-padding pt-5">
          <Banner tone="danger">
            {conflictMessage(error, translate) ?? t('assign.errors.generic')}
          </Banner>
        </div>
      ) : null}
      <div className="grid gap-8 py-8 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <section className="flex flex-col gap-5" aria-label={t('assign.videos.title')}>
          <div className="flex items-center justify-between gap-3">
            <h2 className="m-0 text-heading font-display">{t('assign.videos.title')}</h2>
            <Button variant="secondary" onClick={preview}>
              {t('assign.preview.button')}
            </Button>
          </div>
          {suggestions.relation === 'episodes' && (
            <TitlePicker
              {...(request.videos[0]?.title ? { value: request.videos[0].title } : {})}
              kind="series"
              {...(suggestions.title ? { suggested: suggestions.title } : {})}
              {...(suggestions.title?.title ? { initialTerm: suggestions.title.title } : {})}
              {...(issueTextFor('title') ? { error: issueTextFor('title') } : {})}
              onChange={(title) => {
                setDirty(true)
                setRequest(
                  (current) =>
                    current && { ...current, videos: mergeTitleIntoVideos(current.videos, title) },
                )
                setPlan(null)
              }}
            />
          )}
          {request.videos.map((video, index) => (
            <AssignVideoCard
              key={video.file}
              suggestion={
                suggestions.videos[index] ?? { file: video.file, reading: { type: 'unknown' } }
              }
              value={video}
              {...(plan?.videos[index] ? { plan: plan.videos[index] } : {})}
              issues={plan?.errors.filter((issue) => issue.file === video.file) ?? []}
              subtitles={group.get(video.file)?.length ?? 0}
              showPart={suggestions.relation === 'parts'}
              showTitle={suggestions.relation !== 'episodes'}
              focused={focused === index}
              onFocus={() => {
                setFocused(index)
              }}
              onMove={(delta) => {
                const next = Math.min(request.videos.length - 1, Math.max(0, index + delta))
                setFocused(next)
                document
                  .querySelector<HTMLElement>(
                    `[data-video-card="true"]:nth-of-type(${String(next + 1)})`,
                  )
                  ?.focus()
              }}
              onChange={(value) => {
                updateVideo(index, value)
              }}
            />
          ))}
          <SubtitleList
            suggestions={suggestions.subtitles}
            values={request.subtitles}
            issues={plan?.errors ?? []}
            onChange={(subtitles) => {
              setDirty(true)
              setRequest((current) => current && { ...current, subtitles })
              setPlan(null)
            }}
          />
        </section>
        <div className="hidden lg:block">
          <PlanPreview
            plan={plan}
            onSave={save}
            saveEnabled={currentState.enabled}
            {...(saveReason ? { saveReason } : {})}
            saving={false}
          />
        </div>
      </div>
      <div className="fixed inset-x-0 bottom-0 z-20 flex gap-3 border-t border-line bg-surface p-3 lg:hidden">
        <Button
          className="flex-1"
          onClick={() => {
            setPreviewOpen(true)
          }}
        >
          {t('assign.preview.open')}
        </Button>
        <Button
          variant="primary"
          className="flex-1"
          disabled={!currentState.enabled}
          {...(saveReason ? { disabledReason: saveReason } : {})}
          onClick={save}
        >
          {t('assign.save')}
        </Button>
      </div>
      {previewOpen && (
        <Sheet
          title={t('assign.preview.title')}
          onClose={() => {
            setPreviewOpen(false)
          }}
        >
          <PlanPreview
            plan={plan}
            onSave={() => {
              setPreviewOpen(false)
              save()
            }}
            saveEnabled={currentState.enabled}
            {...(saveReason ? { saveReason } : {})}
            saving={false}
          />
        </Sheet>
      )}
      {confirmCancel && (
        <ConfirmDialog
          title={t('assign.cancel.title')}
          confirmLabel={t('assign.cancel.confirm')}
          onCancel={() => {
            setConfirmCancel(false)
          }}
          onConfirm={cancelNow}
        >
          {t('assign.cancel.body')}
        </ConfirmDialog>
      )}
      {!gamepad && (
        <ShortcutBar
          groups={[
            {
              title: t('assign.shortcuts.videoFocus'),
              keys: [
                { key: '↑↓', label: t('assign.shortcuts.navigate') },
                { key: 'Enter', label: t('assign.shortcuts.accept') },
                { key: 'R', label: t('assign.shortcuts.replace') },
                { key: 'A', label: t('assign.shortcuts.keepBoth') },
                { key: 'O', label: t('assign.shortcuts.skip') },
              ],
            },
            {
              title: t('assign.shortcuts.always'),
              keys: [
                { key: 'Ctrl+Enter', label: t('assign.shortcuts.save') },
                { key: 'Esc', label: t('assign.shortcuts.cancel') },
              ],
            },
          ]}
        />
      )}
      <HelpBar
        actions={[
          { glyph: 'A', label: t('assign.shortcuts.accept') },
          { glyph: 'B', label: t('common.cancel') },
          { glyph: 'X', label: t('common.moreActions') },
        ]}
      />
    </LibraryLayout>
  )
}
