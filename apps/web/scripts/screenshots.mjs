import { execFileSync } from 'node:child_process'
import { mkdirSync, readdirSync, renameSync, rmSync, statSync } from 'node:fs'
import { join } from 'node:path'

import { chromium, devices } from '@playwright/test'

const base = process.env.BASE_URL ?? 'http://localhost:4173'
const out = join(import.meta.dirname, '..', '..', '..', 'docs', 'screenshots')
const langs = {
  en: {
    home: 'Home',
    movies: 'Movies',
    series: 'Series',
    unassigned: 'Unassigned',
    tryFiles: 'Try sample files',
    relationNone: 'No relation',
    genres: /Genres|Géneros/,
    animation: /Animation|Animación/,
    drama: 'Drama',
    select: 'Select',
    selected: /\d+ selected/,
    preview: 'Preview',
    previewTitle: 'Preview',
    save: 'Save',
    replace: 'Replace',
    changeMatch: /Change|Choose title/,
    searchTitle: 'Search title',
    chosen: 'Chosen',
    files: 'Files',
    collection: 'Collection',
    collectionTitle: /Collection|Colección/,
    seasons: 'Seasons',
    back: 'Unassigned',
    language: 'en',
  },
  es: {
    home: 'Inicio',
    movies: 'Películas',
    series: 'Series',
    unassigned: 'No asignados',
    tryFiles: 'Probar con archivos de ejemplo',
    relationNone: 'Sin relación',
    genres: /Genres|Géneros/,
    animation: /Animation|Animación/,
    drama: 'Drama',
    select: 'Seleccionar',
    selected: /\d+ seleccionados?/,
    preview: 'Previsualizar',
    previewTitle: 'Así quedará',
    save: 'Guardar',
    replace: 'Reemplazar',
    changeMatch: /Cambiar|Elegir título/,
    searchTitle: 'Buscar título',
    chosen: 'Elegida',
    files: 'Archivos',
    collection: 'Colección',
    collectionTitle: /Colección/,
    seasons: 'Temporadas',
    back: 'No asignados',
    language: 'es',
  },
}

const pause = (page, ms = 800) => page.waitForTimeout(ms)

async function settle(page) {
  await page.evaluate(() => document.fonts.ready)
  await page.waitForFunction(
    () => {
      const images = [...document.images].filter((image) =>
        image.currentSrc.includes('image.tmdb.org'),
      )
      return images.every((image) => image.complete && image.naturalWidth > 0)
    },
    undefined,
    { timeout: 30_000 },
  )
  await pause(page, 250)
}

async function goInApp(page, path) {
  await page.evaluate((to) => {
    history.pushState({}, '', to)
    dispatchEvent(new PopStateEvent('popstate'))
  }, path)
  await page.waitForURL(new URL(path, base).href)
  await pause(page, 450)
}

async function fixedPage(browser, lang, options = {}) {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    reducedMotion: 'reduce',
    ...options,
  })
  await context.addInitScript((language) => {
    localStorage.setItem('ve.lang', language)
    const style = document.createElement('style')
    style.textContent =
      '* { caret-color: transparent !important; animation: none !important; transition: none !important; scroll-behavior: auto !important; }'
    document.documentElement.append(style)
  }, lang)
  const page = await context.newPage()
  await page.goto(base + '/')
  await page.getByRole('heading', { level: 1, name: langs[lang].home }).waitFor()
  return { context, page }
}

async function capture(page, path) {
  await settle(page)
  await page.screenshot({ path, animations: 'disabled' })
}

async function waitForTmdbImage(locator) {
  const image = locator.locator('img[src*="image.tmdb.org"]').first()
  if (await image.count()) {
    await image.evaluate(async (element) => {
      const img = element
      if (!img.complete)
        await new Promise((resolve) => {
          img.addEventListener('load', resolve, { once: true })
          img.addEventListener('error', resolve, { once: true })
        })
      if (img.complete && img.naturalWidth) await img.decode().catch(() => undefined)
    })
  }
}

async function captureHome(page, langDir, filename = 'home.png') {
  const hero = page.getByRole('region', { name: /Featured|Destacado/ })
  await hero.waitFor()
  for (let index = 0; index < 5; index++) {
    const logo = hero.locator('h2 img[src*="image.tmdb.org"]')
    if (await logo.count()) {
      await logo.first().evaluate((image) => image.complete && image.naturalWidth > 0)
      if (await logo.first().evaluate((image) => image.complete && image.naturalWidth > 0)) break
    }
    await hero.getByRole('button', { name: /Next|Siguiente/ }).click()
    await pause(page, 250)
  }
  await capture(page, join(langDir, filename))
}

async function captureMovie(page, langDir, t) {
  await goInApp(page, '/movies')
  const movie = page.getByRole('link', { name: /Duna|Dune/ }).first()
  await movie.waitFor()
  await waitForTmdbImage(movie)
  await movie.click()
  await page.getByRole('heading', { level: 1, name: /Duna|Dune/ }).waitFor()
  await page.evaluate(() => {
    document.body.style.zoom = '0.75'
    window.scrollTo(0, 0)
  })
  const files = page.getByRole('heading', { name: t.files, exact: true })
  await page.getByRole('heading', { name: t.collectionTitle }).waitFor()
  await capture(page, join(langDir, 'movie.png'))
}

async function captureSeries(page, langDir, t) {
  await goInApp(page, '/series')
  const series = page.getByRole('link', { name: /Breaking Bad/ }).first()
  await series.waitFor()
  await waitForTmdbImage(series)
  await series.click()
  await page.getByRole('heading', { level: 1, name: 'Breaking Bad' }).waitFor()
  await page.evaluate(() => {
    document.body.style.zoom = '0.55'
    window.scrollTo(0, 0)
  })
  await page.getByRole('heading', { name: t.seasons, exact: true }).waitFor()
  await page.getByRole('tab', { name: /Season 3|Temporada 3/ }).click()
  await page.getByText(/Present|Presente/).first().waitFor()
  await page.getByText(/Missing|Falta/).first().waitFor()
  await capture(page, join(langDir, 'series.png'))
}

async function captureAssignment(page, langDir, t) {
  await goInApp(page, '/unassigned/1/assign')
  await page.getByRole('heading', { name: /Assign|Asignar/ }).waitFor()
  const picker = page.locator('article').first()
  const poster = picker.locator('button img[src*="image.tmdb.org"]').first()
  if (await poster.count()) await waitForTmdbImage(picker)
  await page.getByRole('button', { name: t.changeMatch }).first().click()
  const choices = page.getByRole('dialog')
  await choices.getByRole('textbox', { name: t.searchTitle }).fill('ted lasso')
  const match = choices.getByRole('radio').first()
  await match.waitFor()
  await waitForTmdbImage(choices)
  await match.click()
  const previewButton = page.getByRole('button', { name: t.preview, exact: true }).first()
  await previewButton.click()
  await page.getByRole('heading', { name: t.previewTitle, exact: true }).waitFor()
  await settle(page)
  await page.evaluate(() => window.scrollTo(0, 0))
  await capture(page, join(langDir, 'assign.png'))
}

async function captureUnassigned(page, langDir, t) {
  await goInApp(page, '/unassigned')
  await page.getByRole('heading', { name: t.unassigned, exact: true }).waitFor()
  await capture(page, join(langDir, 'unassigned.png'))
}

async function captureSelection(page, langDir, t) {
  await goInApp(page, '/movies')
  const menu = page.getByRole('button', { name: t.genres, exact: true })
  await menu.click()
  await page.getByRole('checkbox', { name: t.animation }).check()
  await page.getByRole('checkbox', { name: t.drama, exact: true }).check()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: t.select, exact: true }).click()
  const posters = page.locator('[data-selection-id]')
  await posters.nth(0).click()
  await posters.nth(1).click()
  await page.getByText(t.selected).waitFor()
  await page.getByRole('checkbox', { name: t.animation }).waitFor({ state: 'hidden' })
  await capture(page, join(langDir, 'select.png'))
}

async function capturePhone(browser, lang, langDir, t) {
  const context = await browser.newContext({
    ...devices['iPhone 13'],
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
    reducedMotion: 'reduce',
  })
  await context.addInitScript((language) => {
    localStorage.setItem('ve.lang', language)
    const style = document.createElement('style')
    style.textContent =
      '* { caret-color: transparent !important; animation: none !important; transition: none !important; scroll-behavior: auto !important; }'
    document.documentElement.append(style)
  }, lang)
  const page = await context.newPage()
  await page.goto(base + '/')
  await page.getByRole('heading', { level: 1, name: t.home }).waitFor()
  await captureHome(page, langDir, 'phone.png')
  await context.close()
}

async function uploadSample(page, t, wait = 500) {
  await page.getByRole('button', { name: t.tryFiles, exact: true }).click()
  const relation = page.getByRole('dialog')
  await relation.getByRole('button', { name: t.relationNone }).click()
  await pause(page, wait)
  const password = page.getByRole('textbox', { name: /Password|Contraseña/ }).first()
  if (await password.isVisible().catch(() => false)) {
    await password.fill('demo')
    await page
      .getByRole('button', { name: /Continue|Continuar/ })
      .first()
      .click()
  }
  await pause(page, wait)
}

async function captureGif(browser, lang, t, langDir) {
  const videoDir = join(langDir, 'video')
  rmSync(videoDir, { recursive: true, force: true })
  const context = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: 1,
    recordVideo: { dir: videoDir, size: { width: 1440, height: 900 } },
  })
  await context.addInitScript((language) => {
    localStorage.setItem('ve.lang', language)
    const style = document.createElement('style')
    style.textContent =
      '* { caret-color: transparent !important; animation: none !important; transition: none !important; scroll-behavior: auto !important; }'
    document.documentElement.append(style)
  }, lang)
  const page = await context.newPage()
  await page.goto(base + '/')
  await page.getByRole('heading', { level: 1, name: t.home }).waitFor()
  await pause(page, 1100)
  await uploadSample(page, t, 900)
  const uploadPanel = page.getByRole('complementary', { name: /Uploads|Subidas/ })
  await uploadPanel
    .getByRole('button', { name: /Close|Cerrar/ })
    .click()
    .catch(() => undefined)
  await pause(page, 1000)
  await page.getByRole('link', { name: /VideoExplorer/ }).click()
  await page.locator('a[href="/unassigned"]').first().click()
  await page.getByRole('heading', { name: t.unassigned, exact: true }).waitFor()
  await pause(page, 900)
  const entryLink = page
    .locator('article')
    .filter({ hasText: 'Dark.S02E05.1080p.mkv' })
    .locator('a[href^="/unassigned/"][href$="/assign"]')
    .first()
  await entryLink.waitFor()
  const assignHref = await entryLink.getAttribute('href')
  if (!assignHref) throw new Error('Unassigned entry did not expose its assign route')
  await entryLink.click()
  await page.getByRole('heading', { name: /Assign|Asignar/ }).waitFor()
  await pause(page, 1200)
  await page.getByRole('button', { name: t.changeMatch }).first().click()
  const picker = page.getByRole('dialog')
  await picker.getByRole('textbox', { name: t.searchTitle }).fill('dark')
  const match = picker.getByRole('radio').first()
  await match.waitFor()
  await waitForTmdbImage(picker)
  await match.click()
  await pause(page, 500)
  await page.getByRole('button', { name: t.changeMatch }).first().click()
  const chosenPicker = page.getByRole('dialog')
  const chosenMatch = chosenPicker.getByRole('radio', { checked: true }).first()
  await chosenMatch.waitFor()
  await waitForTmdbImage(chosenPicker)
  await chosenPicker.getByText(t.chosen, { exact: true }).waitFor()
  await pause(page, 1400)
  await chosenMatch.click()
  const previewButton = page.getByRole('button', { name: t.preview, exact: true }).first()
  await previewButton.click()
  const duplicate = page.getByRole('group', { name: /duplicate|duplicado/i })
  await duplicate.waitFor({ timeout: 10_000 }).catch(() => undefined)
  if (await duplicate.isVisible().catch(() => false)) {
    await duplicate.getByRole('button', { name: t.replace, exact: true }).click()
    await previewButton.click()
  }
  await page.getByRole('heading', { name: t.previewTitle, exact: true }).waitFor()
  await pause(page, 1300)
  await page.getByRole('button', { name: t.save, exact: true }).last().click()
  await page.waitForURL(/\/unassigned$/)
  await page.locator('a[href="/series"]').first().click()
  const savedSeries = page.getByRole('link', { name: /Dark/ }).first()
  await savedSeries.waitFor()
  await savedSeries.click()
  await page.waitForURL(/\/series\/\d+$/)
  await page.getByRole('heading', { name: 'Dark', exact: true }).waitFor()
  await pause(page, 1800)
  await context.close()

  const recorded = readdirSync(videoDir).find((file) => file.endsWith('.webm'))
  if (!recorded) throw new Error('Playwright did not record a video')
  const webm = join(videoDir, 'assign.webm')
  renameSync(join(videoDir, recorded), webm)
  const gif = join(langDir, 'assign.gif')
  const paletteFilter =
    'fps=12,scale=1200:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=96:stats_mode=diff[p];[b][p]paletteuse=dither=bayer'
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', webm, '-vf', paletteFilter, gif])
  if (statSync(gif).size > 6_000_000) {
    execFileSync('ffmpeg', [
      '-y',
      '-loglevel',
      'error',
      '-i',
      webm,
      '-vf',
      'fps=7,scale=900:-1:flags=lanczos,split[a][b];[a]palettegen=max_colors=64:stats_mode=diff[p];[b][p]paletteuse=dither=bayer',
      gif,
    ])
  }
  if (statSync(gif).size > 6_000_000)
    throw new Error(`GIF exceeds 6 MB: ${String(statSync(gif).size)} bytes`)
  rmSync(videoDir, { recursive: true, force: true })
}

async function shoot(browser, lang) {
  const t = langs[lang]
  const dir = join(out, lang)
  mkdirSync(dir, { recursive: true })
  const { context, page } = await fixedPage(browser, lang)
  await captureHome(page, dir)
  await captureMovie(page, dir, t)
  await captureSeries(page, dir, t)
  await captureAssignment(page, dir, t)
  await captureUnassigned(page, dir, t)
  await captureSelection(page, dir, t)
  await context.close()
  await capturePhone(browser, lang, dir, t)
  await captureGif(browser, lang, t, dir)
}

mkdirSync(out, { recursive: true })
const browser = await chromium.launch()
try {
  for (const lang of ['en', 'es']) await shoot(browser, lang)
} finally {
  await browser.close()
}
console.log('screenshots written to', out)
