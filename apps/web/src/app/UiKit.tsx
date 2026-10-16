import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useLetterShortcuts } from '@/shared/input'
import { LanguageToggle } from '@/shared/i18n/LanguageToggle'
import {
  Banner,
  Button,
  Checkbox,
  Chip,
  ChipGroup,
  EmptyState,
  FileButton,
  HelpBar,
  ImageIcon,
  Menu,
  NumberField,
  PageHeader,
  PasswordField,
  Poster,
  PosterRow,
  ProgressBar,
  SearchField,
  SectionTabs,
  SegmentedControl,
  Select,
  ShortcutBar,
  SidePanel,
  StatusChip,
  Switch,
  Tabs,
  TextField,
  ConfirmDialog,
  Sheet,
  UploadIcon,
} from '@/shared/ui'

export function UiKit() {
  const { t } = useTranslation()
  const [sort, setSort] = useState('recent')
  const [genre, setGenre] = useState('all')
  const [season, setSeason] = useState('one')
  const [dialog, setDialog] = useState(false)
  const [sheet, setSheet] = useState(false)
  const [panel, setPanel] = useState(false)
  const shortcutRoot = useRef<HTMLElement>(null)
  const [shortcutMessage, setShortcutMessage] = useState('')
  useLetterShortcuts(shortcutRoot, {
    r: () => {
      setShortcutMessage(t('uiKit.primary'))
    },
    a: () => {
      setShortcutMessage(t('uiKit.confirmAction'))
    },
    o: () => {
      setShortcutMessage(t('uiKit.pending'))
    },
  })

  return (
    <div className="min-h-screen bg-bg text-ink-1">
      <PageHeader
        title={t('app.name')}
        subtitle={<p className="m-0 text-body text-ink-2">{t('app.tagline')}</p>}
        actions={
          <>
            <LanguageToggle />
            <FileButton onFiles={() => undefined} shortcut icon={<UploadIcon />}>
              {t('uiKit.upload')}
            </FileButton>
          </>
        }
      />
      <main
        ref={shortcutRoot}
        className="page-padding mx-auto flex max-w-[1440px] flex-col gap-8 py-8"
      >
        <header className="flex flex-col gap-2">
          <h1 className="sr-only">{t('uiKit.title')}</h1>
          <h2 className="m-0 text-heading font-display">{t('uiKit.title')}</h2>
          <p className="m-0 text-body text-ink-2">{t('uiKit.subtitle')}</p>
        </header>

        <section aria-labelledby="ui-actions" className="flex flex-col gap-4">
          <h2 id="ui-actions" className="m-0 text-heading font-display">
            {t('uiKit.actions')}
          </h2>
          <div className="flex flex-wrap items-center gap-3">
            <Button variant="primary">{t('uiKit.primary')}</Button>
            <Button>{t('uiKit.secondary')}</Button>
            <Button variant="outline-accent">{t('uiKit.confirmAction')}</Button>
            <Button variant="danger">{t('uiKit.delete')}</Button>
            <Button disabled disabledReason={t('uiKit.disabled')}>
              {t('uiKit.save')}
            </Button>
            <Menu
              label={t('uiKit.menu')}
              items={[
                { label: t('uiKit.download'), onSelect: () => undefined },
                { label: t('uiKit.delete'), tone: 'danger', onSelect: () => undefined },
              ]}
            >
              {t('uiKit.menu')}
            </Menu>
          </div>
        </section>

        <section
          aria-labelledby="ui-fields"
          className="grid gap-6 rounded-lg border border-line bg-surface p-6 lg:grid-cols-2"
        >
          <h2 id="ui-fields" className="m-0 text-heading font-display lg:col-span-2">
            {t('uiKit.fields')}
          </h2>
          <TextField
            label={t('uiKit.name')}
            hint={t('uiKit.nameHint')}
            defaultValue="Dark.S01E01.mkv"
          />
          <TextField label={t('uiKit.name')} error={t('uiKit.invalid')} defaultValue="" />
          <NumberField label={t('uiKit.sort')} defaultValue={1} />
          <Select label={t('uiKit.sort')} defaultValue="recent">
            <option value="recent">{t('uiKit.recent')}</option>
            <option value="title">{t('uiKit.titleOrder')}</option>
          </Select>
          <SearchField label={t('uiKit.search')} shortcut placeholder={t('uiKit.search')} />
          <PasswordField label={t('uiKit.password')} />
          <Checkbox label={t('uiKit.checkbox')} checked onChange={() => undefined} />
          <Switch label={t('uiKit.switch')} checked onChange={() => undefined} />
        </section>

        <section aria-labelledby="ui-choices" className="flex flex-col gap-4">
          <h2 id="ui-choices" className="m-0 text-heading font-display">
            {t('uiKit.choices')}
          </h2>
          <SegmentedControl
            label={t('uiKit.sort')}
            value={sort}
            onChange={setSort}
            options={[
              { value: 'recent', label: t('uiKit.recent') },
              { value: 'title', label: t('uiKit.titleOrder') },
            ]}
          />
          <ChipGroup
            legend={t('uiKit.chips')}
            name="genre"
            value={genre}
            onChange={setGenre}
            options={[
              { value: 'all', label: t('uiKit.all') },
              { value: 'action', label: t('uiKit.action') },
            ]}
          />
          <div className="flex flex-wrap gap-2">
            <Chip>{t('uiKit.all')}</Chip>
            <StatusChip tone="success">{t('uiKit.present')}</StatusChip>
            <StatusChip tone="warning" solid>
              {t('uiKit.pending')}
            </StatusChip>
            <StatusChip tone="info">{t('uiKit.progress')}</StatusChip>
          </div>
          <Tabs
            label={t('uiKit.tabs')}
            value={season}
            onChange={setSeason}
            options={[
              { value: 'one', label: t('uiKit.seasonOne') },
              { value: 'two', label: t('uiKit.seasonTwo') },
            ]}
          />
          <SectionTabs
            label={t('uiKit.tabs')}
            current="one"
            tabs={[
              { value: 'one', href: '#one', label: t('uiKit.seasonOne') },
              { value: 'two', href: '#two', label: t('uiKit.seasonTwo') },
            ]}
          />
        </section>

        <section aria-labelledby="ui-feedback" className="flex flex-col gap-4">
          <h2 id="ui-feedback" className="m-0 text-heading font-display">
            {t('uiKit.feedback')}
          </h2>
          <Banner tone="warning">{t('uiKit.warning')}</Banner>
          <ProgressBar value={63} label={t('uiKit.progress')} tone="extract" />
        </section>

        <section className="flex flex-col gap-4">
          <PosterRow title={t('uiKit.posterRow')} count={2}>
            <Poster
              href="#dark"
              title={t('uiKit.posterTitle')}
              meta={t('uiKit.posterMeta')}
              accessibleChips={[t('uiKit.posterChip')]}
              chips={[
                <StatusChip key="missing" tone="warning" solid>
                  {t('uiKit.posterChip')}
                </StatusChip>,
              ]}
            />
            <Poster
              href="#dark-two"
              title={t('uiKit.posterSecondTitle')}
              meta={t('uiKit.posterSecondMeta')}
              seed={t('uiKit.posterSecondTitle')}
            />
          </PosterRow>
        </section>

        <section className="grid gap-4 lg:grid-cols-2">
          <EmptyState
            icon={<ImageIcon size={36} />}
            title={t('uiKit.empty')}
            body={t('uiKit.emptyBody')}
            action={<Button variant="primary">{t('uiKit.upload')}</Button>}
          />
          <div className="flex flex-wrap items-start gap-3 rounded-lg border border-line bg-surface p-6">
            <Button
              onClick={() => {
                setDialog(true)
              }}
            >
              {t('uiKit.dialogOpen')}
            </Button>
            <Button
              onClick={() => {
                setSheet(true)
              }}
            >
              {t('uiKit.sheetOpen')}
            </Button>
            <Button
              onClick={() => {
                setPanel(true)
              }}
            >
              {t('uiKit.panelOpen')}
            </Button>
          </div>
        </section>

        <section className="flex flex-col gap-3">
          <p aria-live="polite" className="m-0 min-h-6 text-body-sm text-accent">
            {shortcutMessage}
          </p>
          <ShortcutBar
            groups={[
              {
                title: t('uiKit.shortcuts'),
                keys: [
                  { key: 'R', label: t('uiKit.primary') },
                  { key: 'A', label: t('uiKit.confirmAction') },
                  { key: 'O', label: t('uiKit.pending') },
                ],
              },
            ]}
          />
          <HelpBar
            actions={[
              { glyph: 'A', label: t('uiKit.confirmAction') },
              { glyph: 'B', label: t('uiKit.backAction') },
            ]}
            aside={<span className="text-ink-2">{t('uiKit.help')}</span>}
          >
            <Button variant="secondary">{t('uiKit.help')}</Button>
          </HelpBar>
        </section>
      </main>
      {dialog && (
        <ConfirmDialog
          title={t('uiKit.dialogTitle')}
          confirmLabel={t('uiKit.confirm')}
          tone="danger"
          onConfirm={() => {
            setDialog(false)
          }}
          onCancel={() => {
            setDialog(false)
          }}
        >
          {t('uiKit.dialogBody')}
        </ConfirmDialog>
      )}
      {sheet && (
        <Sheet
          title={t('uiKit.sheetTitle')}
          onClose={() => {
            setSheet(false)
          }}
        >
          <p className="m-0 text-body text-ink-2">{t('uiKit.sheetBody')}</p>
        </Sheet>
      )}
      {panel && (
        <SidePanel
          title={t('uiKit.panelTitle')}
          onClose={() => {
            setPanel(false)
          }}
        >
          <p className="m-0 text-body text-ink-2">{t('uiKit.panelBody')}</p>
        </SidePanel>
      )}
    </div>
  )
}
