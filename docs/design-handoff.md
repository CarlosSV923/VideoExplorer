# Traspaso de diseño (Fase 9)

Fuente: lienzo [`/design`](https://claude.ai/artifact/A2ENfFRCjVUMMcjMMiZCqb) (22 pantallas, ya corregidas según [`design-review.md`](./design-review.md)). Destino: React 19 + Vite + Tailwind v4, primitivas en `apps/web/src/shared/ui`, pantallas en `modules/<contexto>/ui` (Fase 10). Como en GameExplorer, el código usa **tokens**, nunca valores sueltos; si algo no está aquí, se agrega aquí antes de programarlo.

## 1. Tokens

Se definen en `apps/web/src/shared/ui/tokens.css` como `@theme` de Tailwind v4 (variables CSS y utilidades a la vez: `bg-surface`, `text-ink-2`…). La paleta por defecto de Tailwind se borra para que solo existan estos tokens. Tema único oscuro (estética de cine, RF-40).

### Color
| Token | Valor | Uso |
|---|---|---|
| `bg` | `#0c0c0f` | Fondo de la app |
| `bg-sunken` | `#070709` | Barra de ayuda / atajos |
| `surface` | `#15151a` | Tarjetas, paneles, diálogos |
| `surface-2` | `#1d1d23` | Elementos dentro de un panel (trabajos de subida, menús) |
| `surface-3` | `#26262e` | Chips neutros, elemento de menú activo, píldoras de estado |
| `surface-missing` | `#121216` | Póster de lo que falta (colección) |
| `overlay` | `rgb(7 7 9 / .84)` | Fondo de diálogos y de la zona de soltar |
| `scrim` | `rgb(12 12 15 / .96 → 0)` | Degradados sobre backdrops (hero y fichas) |
| `line` | `#2e2e37` | Separadores y bordes de tarjetas (decorativos) |
| `control` | `#7a7a86` | Borde de botones secundarios, inputs y pestañas (≥ 3,96:1, WCAG 1.4.11) |
| `ink-1` | `#f5f5f7` | Texto principal (17,9:1) |
| `ink-2` | `#c4c4cc` | Texto secundario (11,3:1) |
| `ink-3` | `#9696a1` | Metadatos y placeholders (≥ 5,7:1 en cualquier `surface`) |
| `accent` | `#35d0ba` | Acción principal, foco, selección, enlaces |
| `accent-hover` | `#6fe3d1` | Hover de acción principal y enlaces |
| `on-accent` | `#04201c` | Texto sobre `accent` (8,9:1) |
| `success-surface` / `success-ink` | `#0f2e2a` / `#7fe8d7` | «Presente», «Lista para asignar», «Conectado» (borde `accent`) |
| `warning` | `#f5c451` | Bordes de aviso, contador de No asignados, chip «Faltan N» (texto `on-warning` `#1f1604`) |
| `warning-surface` / `warning-ink` | `#2e2614` / `#ffe3a0` | Avisos, duplicados, «Falta», «Copiando…», «Necesita contraseña» |
| `warning-muted` | `#bfa165` | Borde de las opciones no elegidas dentro de un aviso |
| `info` | `#8ab4ff` | Progreso de descompresión, escaneo en curso, «Próximamente» |
| `info-surface` / `info-ink` | `#1a2333` / `#cfe0ff` | Estados en curso, sesión vencida, banner de demo |
| `danger` | `#ff8577` | Acciones destructivas (texto y borde), errores de campo (7,7:1) |
| `on-danger` | `#2a0b07` | Texto sobre `danger` (botón de confirmación destructiva) |
| `danger-surface` / `danger-ink` | `#3a1b18` / `#ffd6d0` | Banner de salud, errores |
| `progress-upload` | `ink-1` | Barra de subida (siempre con texto del estado al lado, WCAG 1.4.1) |
| `progress-extract` | `info` | Barra de descompresión |

**Pósters y backdrops sin imagen** (títulos libres o imagen que no carga; la demo usa las imágenes de TMDB, RF-71): degradado `linear-gradient(165deg, <tono> 0%, <tono oscuro> 100%)` generado de forma estable a partir del id del título, con el título en mayúsculas encima. El título dentro del póster es decorativo (`aria-hidden`).

### Tipografía
Familias: **Outfit** 400/500/600/700/800 para la interfaz y **JetBrains Mono** 400/500 para nombres de archivo, rutas, etiquetas de ID y atajos. Se sirven desde la app (`@fontsource`), nunca desde Google, para que el navegador no contacte a terceros.

| Token | Tamaño / interlínea | Peso | Uso |
|---|---|---|---|
| `display-hero` | `clamp(44px, 6vw, 76px)` / .95, mayúsculas | 800 | Título del hero (cuando no hay logo) |
| `display` | 56 px / 1 (30 px en móvil) | 800 | Título de ficha y de colección |
| `page-title` | 44 px / 1 | 800 | `h1` de Películas, Series, No asignados, Ajustes |
| `title` | 26–28 px / 1.2 | 800 | Título de diálogo, cabecera de Asignar / Editar |
| `heading` | 22–24 px | 700 | Secciones (filas, «Archivos», «Así quedará») |
| `subheading` | 18–20 px | 700 | Título de tarjeta, capítulo, entrada |
| `label` | 13–14 px, mayúsculas, `tracking .08em` | 700 | Rótulos de sección y de campo |
| `body-lg` | 18 px / 1.5 | 400 | Sinopsis |
| `body` | 16 px / 1.5 | 400–600 | Texto general, botones |
| `body-sm` | 15 px | 400–600 | Metadatos, avisos |
| `caption` | 13–14 px | 600–700 | Chips, metadatos de póster |
| `mono` | 14 px (13 en listas, 12 en móvil) | 400–500 | Archivos y rutas, `overflow-wrap: anywhere` |

### Espaciado, radios y tamaños
| Token | Valor | Uso |
|---|---|---|
| `space-1…8` | 4, 8, 12, 16, 20, 24, 32, 40 px | Gaps y paddings |
| `page-x` | 48 px (≥ 1024) · 32 px (≥ 640) · 16 px | Margen lateral |
| `row-gap` | 36 px (26 en móvil) | Entre filas de Inicio |
| `radius-sm` | 6–8 px | Miniaturas, atajos (`kbd`), chips cuadrados |
| `radius-md` | 10 px | Inputs, pósters, avisos |
| `radius-lg` | 12–14 px | Tarjetas y paneles |
| `radius-xl` | 18–20 px | Diálogos y hoja inferior móvil |
| `radius-pill` | `9999px` | Botones, chips, pestañas |
| `control-lg` | 52 px | Botón principal de diálogo, Guardar, login |
| `control-md` | 48 px | Botones de ficha y de hero |
| `control-sm` | 44 px | Botones de icono, chips, pestañas, filas de menú. **Mínimo absoluto** (RNF-05) |
| `poster-w` | 176 px (escritorio) · 120 px (móvil) · 160 px (colección en ficha) | Ancho de póster en filas; 2:3 |
| `thumb-w` | 224 px (escritorio) · 128 px (móvil) | Miniatura de capítulo; 16:9 |

### Foco
`:focus-visible` → `outline: 3px solid accent; outline-offset: 3px`. Los inputs ocultos dentro de un `label` (orden, géneros, decisiones de duplicado, interruptor de etiqueta de ID) muestran el anillo en el `label` con `label:has(input:focus-visible)`. El póster enfocado también se eleva (`translateY(-4px)`). **Nunca** se quita el foco sin reemplazarlo.

## 2. Componentes (`shared/ui`)

| Componente | Variantes / props | Notas |
|---|---|---|
| `Button` | `variant: primary \| secondary \| outline-accent \| danger \| danger-solid`, `size: lg \| md \| sm`, `icon?`, `loading?`, `disabled?` + `disabledReason?` | Píldora. `secondary` = contorno `control`; `outline-accent` = «Confirmar todo»; `danger` = contorno y texto `danger`; `danger-solid` solo en el diálogo de confirmación |
| `IconButton` | `label` (obligatorio, `aria-label`), `tone: default \| danger`, `variant: outline \| ghost \| overlay` | 44×44. `overlay` = fondo translúcido sobre imágenes |
| `LinkButton` | Igual que `Button`, renderiza `<a>` | Navegar (Ver detalles, Asignar, Editar y reasignar) |
| `FileButton` | `multiple`, `onFiles` | `label` + `input[type=file]` invisible; foco en el `label` |
| `TextField` / `NumberField` / `Select` | `label` (visible u oculto), `hint?`, `error?`, `invalid?` | El error va en `aria-describedby` y pone `aria-invalid`; borde `danger` de 2 px |
| `SearchField` | `value`, `onClear` | Píldora con lupa; borde `accent` de 2 px cuando tiene texto; atajo `/` |
| `PasswordField` | `reveal` | Botón «Mostrar la contraseña» con `aria-pressed` |
| `Checkbox` / `Switch` | `label` | `Switch` = `input[type=checkbox][role=switch]` (etiqueta de ID) |
| `SegmentedControl` | Radios o botones con `aria-pressed` | Orden, idioma, tipo (Película / Serie) |
| `ChipGroup` | Radios con estilo de chip | (ya no se usa en las cuadrículas; el filtro de género es `MultiSelectMenu`) |
| `MultiSelectMenu` | `label`, `options`, `value: string[]`, `onChange`, `clearLabel` | Botón desplegable con lista de casillas: filtro de géneros (RF-41) |
| `Chip` / `StatusChip` | `tone: neutral \| success \| warning \| info \| danger`, `solid?` | Texto + color siempre; `solid` = chip `warning` sobre póster («Faltan 4», «Sin vincular») |
| `Tabs` | `role=tablist` | Temporadas; ←/→ entre pestañas, seleccionada en `ink-1` sobre fondo claro |
| `SectionTabs` | Enlaces con `aria-current` | General / Papelera en Ajustes |
| `Menu` | Botón con `aria-haspopup="menu"` + `role=menu` | «Más acciones» (ficha, No asignados, archivo); elementos destructivos en `danger` y con «…» si abren confirmación |
| `Banner` | `tone: warning \| danger \| info`, `live: polite \| assertive`, `action?` | `role=status` o `role=alert` |
| `HealthBanner` | `checks` | Banner fijo `danger` bajo la cabecera (RF-63) con «Detalles» desplegable y «Copiar detalle técnico» |
| `Dialog` / `ConfirmDialog` | `title`, `onClose`, acciones; `tone: danger` | Modal sobre `overlay`; atrapa el foco y lo devuelve; Esc o B cierran; acción principal a la derecha. Con `tone: danger` el foco inicial va a **Cancelar**, nunca a la acción destructiva (Enter no borra por accidente) |
| `Sheet` | Hoja inferior modal (móvil) | Subidas y «Así quedará» en < 768 px; asa visual decorativa |
| `SidePanel` | Panel lateral no modal (escritorio) | Subidas; no oscurece la página |
| `ProgressBar` | `value`, `label`, `tone: upload \| extract` | `role=progressbar`, 8 px de alto |
| `EmptyState` | `icon`, `title`, `body`, `action?` | Plantillas de `Estados` |
| `Brand` | — | Marca «Video» en `ink-1` + «Explorer» en `accent`, 22 px (20 en móvil), peso 800, sin mayúsculas; enlace a Inicio de 44 px de alto |
| `PageHeader` | `back?`, `title`, `subtitle?`, `actions?` | Asignar, Editar |
| `Poster` | `title`, `meta`, `image?`, `chips`, `href`, `dimmed?` | Nombre accesible «título, año, chips»; `dimmed` atenúa solo la imagen (A1) |
| `PosterRow` | `title`, `count?`, `seeAll?` | Lista horizontal con `overflow-x: auto`, `scroll-snap`, botones ‹ › siempre visibles (escritorio), swipe (táctil) |
| `AutoPosterRow` | `title`, `items` (máx. 10), `seeAll` | Fila de Inicio (RF-40, 2026-10-20): sin barra de desplazamiento visible (`scrollbar-width: none`), avance automático de un póster cada 4 s con vuelta al inicio, pausa con hover/foco/toque y botón Pausar/Reanudar (`aria-pressed`) junto a ‹ ›; sin avance con `prefers-reduced-motion`; región sin `aria-live` mientras avanza. Sustituye en Inicio a las filas por género y de colecciones del lienzo |
| `HelpBar` / `Glyph` | `actions: [{button, label}]` | Solo con gamepad conectado; glifos A/B/X/Y/LB/RB propios en SVG |
| `ShortcutBar` | `groups: [{title, keys}]` | Barra de atajos de Asignar (sin gamepad); `kbd` en `mono` |

Componentes de módulo (Fase 10): `HeroCarousel`, `PendingStrip`, `TitleHeader` (backdrop + póster + acciones), `VersionCard` (con Info técnica desplegable), `EpisodeList`, `CollectionGrid`, `UnassignedEntry`, `AssignVideoCard`, `DuplicateChoice`, `PlanPreview`, `TitlePicker`, `RelationDialog`, `UploadJob`, `TrashEntry`, `SettingsCard`.

## 3. Estados e interacción

| Elemento | Estado | Comportamiento |
|---|---|---|
| Botón primario | hover / active | Fondo `accent-hover` / `scale(.98)` |
| Botón secundario | hover | Fondo `rgb(255 255 255 / .08)` |
| Cualquier botón | disabled con motivo | `aria-disabled="true"` (sigue enfocable), fondo `surface-3`, texto `ink-2`, motivo visible encima (p. ej. «Decide 1 duplicado para poder guardar») y en `aria-describedby` |
| Cualquier botón | loading | Indicador giratorio a la izquierda y «Guardando…»; no se puede pulsar dos veces |
| Póster | hover / foco | Elevación de 4 px y anillo de foco; nada aparece solo con hover |
| Capítulo | presente / falta / próximamente | Chip `success` / `warning` / `info`; en «falta» y «próximamente» la miniatura baja a .45 y la fila usa `surface` más oscuro; las acciones de archivo solo existen en «presente» |
| Entrada de No asignados | Copiando… / Asignándose / trabajo en curso | Sin acciones, con una línea que explica por qué y cuándo se libera |
| Duplicado | sin decidir | Tarjeta con borde `warning`; su ruta en «Así quedará» dice «Ruta pendiente hasta que elijas» |
| Etiqueta de versión | incompleta | Borde `danger` de 2 px, `aria-invalid` y texto de qué falta (idioma del audio) |
| Campo | error | Borde `danger` y mensaje debajo |
| Hero | rotando | Cada 8 s; se pausa con hover, con foco dentro y con el botón Pausar (que pasa a «Reanudar»); sin rotación con `prefers-reduced-motion` |
| Hero | entre títulos | Mismo tamaño para todos los títulos (sin saltos de diseño al rotar): alto fijo por tamaño de pantalla (600 px en escritorio, 440 px en móvil), backdrop con `object-fit: cover`, logo dentro de una caja fija (máx. 520 × 160 px en escritorio, 300 × 110 px en móvil) con `object-fit: contain` y alineado abajo a la izquierda, título de texto con el mismo alto de caja cuando no hay logo, y sinopsis cortada a 3 líneas |

### Movimiento
| Elemento | Disparador | Animación | Duración | Curva |
|---|---|---|---|---|
| Hero | Cambio de título | Fundido cruzado del backdrop y del texto | 400 ms | `ease-in-out` |
| Fila de pósters | ‹ › | Desplazamiento suave de una pantalla menos un póster | 300 ms | `cubic-bezier(.2,.8,.2,1)` |
| Póster | Hover / foco | `translateY(-4px)` | 120 ms | `ease-out` |
| Diálogo / hoja | Abrir / cerrar | Fundido y `scale(.98 → 1)` / subida desde abajo | 160 / 220 ms | `ease-out` |
| Panel de Subidas | Abrir / cerrar | Desliza desde la derecha | 200 ms | `ease-out` |
| Zona de soltar | Archivo sobre la ventana | Fundido de la superposición | 120 ms | `ease-out` |
| Barra de progreso | Avance | Ancho | 200 ms | `linear` |

Con `prefers-reduced-motion: reduce` todas las duraciones bajan a 0 ms y el hero no rota.

## 4. Entrada: táctil, mouse, teclado y gamepad

Capa única en `shared/input` (la de GameExplorer, adaptada): las pantallas se suscriben a **acciones**, nunca a teclas; pila de manejadores (el diálogo atiende primero).

| Acción | Teclado | Gamepad | Táctil / mouse |
|---|---|---|---|
| `confirm` | Enter / Espacio | A | Tocar / clic |
| `back` | Esc | B | Botón «Volver» / cerrar |
| `menu` | — (botón visible) | X | «Más acciones» |
| `search` | `/` | Y | Lupa |
| `navigate` | Flechas (fuera de campos de texto) | Cruceta y stick izquierdo | Swipe en filas y hero |
| `prev` / `next` | — | LB / RB | Pestañas visibles |
| `upload` | — (botón visible) | RT | «Subir» |

- **Navegación espacial** propia (como GameExplorer): las flechas y la cruceta mueven el foco al elemento más cercano en esa dirección; dentro de una fila, ←/→ recorren pósters y ↑/↓ cambian de fila. LB/RB cambian de temporada en la ficha de serie y de sección en Ajustes.
- **Glifos** solo con un control conectado (`HelpBar` en lugar de `ShortcutBar`); detección con `gamepadconnected` y lectura por `requestAnimationFrame` (zona muerta 0,5; repetición tras 400 ms y luego cada 120 ms).
- **Atajos de Asignar** (RNF-06, decisión de diseño de esta fase):
  - Con el foco en la tarjeta de un video y fuera de un campo de texto: ↑/↓ cambian de video, Enter acepta la sugerencia, **R** Reemplazar, **A** Conservar ambos, **O** Omitir (WCAG 2.1.4: las teclas de una letra solo actúan con ese foco).
  - Siempre: Ctrl+Enter Guardar (si está habilitado) y Esc Cancelar (pide confirmar si hay cambios).
  - En Editar y reasignar valen los mismos atajos.
- **Hero:** ←/→ con el foco en el hero cambian de título; swipe horizontal con `touch-action: pan-y` y umbral de 48 px.
- Nada depende solo de hover ni de arrastrar: las filas tienen ‹ ›, la zona de soltar tiene «Subir» y el selector nativo.

## 5. Pantallas y tamaños

| Ancho | Cambios |
|---|---|
| ≥ 1280 | Mockups de escritorio (1440) |
| 1024–1279 | Igual, con menos pósters visibles por fila; Asignar y Editar mantienen dos columnas |
| 768–1023 (tablet) | Cabecera en dos líneas (navegación debajo). Asignar y Editar: «Así quedará» pasa debajo del formulario con Guardar fijo abajo. Panel de Subidas a ancho completo |
| < 768 (teléfono) | Mockups de móvil (390): barra de navegación inferior con 5 destinos (Inicio, Películas, Series, No asignados, Ajustes), icono + texto de 13 px; cabecera con lupa, indicador de subidas y botón Subir de icono; hero de 440 px con botones a lo ancho; pósters de 120 px; acciones de archivo en un menú «⋯»; Subidas y «Así quedará» como hoja inferior; Guardar fijo abajo |

### Rutas (TanStack Router, en inglés; RF-64, decisión 2026-10-21)
| Ruta | Pantalla |
|---|---|
| `/login` | Login (`?redirect=` vuelve a donde se estaba) |
| `/` | Inicio |
| `/movies`, `/series` | Cuadrículas (`?sort=added\|title\|year`, `?genre=a,b` (varios), en series `?incomplete=1`) |
| `/movies/:id`, `/series/:id` | Fichas (`?season=` en series) |
| `/movies/:id/edit`, `/series/:id/edit` | Editar y reasignar (también Vincular y etiqueta de ID) |
| `/collections/:id` | Colección |
| `/search?q=` | Búsqueda global |
| `/unassigned` y `/unassigned/:id/assign` | No asignados y Asignar |
| `/settings/general`, `/settings/trash` | Ajustes |

Las rutas antiguas en español no redirigen: muestran «no encontrado».

### Pregunta de relación de una subida (`RelationDialog`, RF-03, ajuste 2026-10-21)
- Las tres respuestas son **tarjetas de opción apiladas** a lo ancho del diálogo (nunca botones de ancho fijo en fila: con textos largos se solapaban). Cada tarjeta es un `button` de mínimo 64 px de alto, borde `control`, radio `md`, fondo `surface-2`, hover `surface-3` y borde `accent`, con icono a la izquierda (película partida, serie, archivos sueltos), el nombre en `body` semibold y debajo una línea de ayuda en `body-sm` `ink-2` que hace wrap: «Partes de una misma película o serie» — «Un título dividido en varios archivos (CD1, CD2…)»; «Capítulos de una misma serie» — «Cada archivo es uno o varios capítulos»; «Sin relación» — «Cada archivo o comprimido es independiente». Un clic sigue empezando la subida.
- El título dice cuántos archivos son: «¿Cómo se relacionan estos N archivos?». Ancho máximo del diálogo 560 px; en móvil, hoja inferior con las tarjetas a lo ancho. Foco inicial en la primera tarjeta; flechas arriba/abajo y la cruceta recorren; Escape o B cancelan como hasta ahora.

### Elegir coincidencia en Editar, Vincular y Asignar (`TitlePicker`, RF-21, ajuste 2026-10-21)
- **Es una elección, no información:** cada sección («En tu biblioteca · Asignar a existente», «Resultados de TMDB») va encabezada por una instrucción «Elige una coincidencia» sobre la lista, y la lista es un `radiogroup` (`role="radiogroup"` con `aria-labelledby` del encabezado; una sola elección entre las dos secciones). Cada resultado es una **tarjeta de opción** `role="radio"` con `aria-checked`: a la izquierda un círculo de radio de 20 px (borde `control`; elegido: relleno `accent` con punto `on-accent`), luego la miniatura del póster, y a la derecha título (semibold, hasta 2 líneas), «Año · Tipo» en `ink-2` y, si ya está en la biblioteca, el chip «En tu biblioteca». Hover: fondo `surface-3` y cursor de mano. **Elegida:** borde `accent` de 2 px, fondo `success-surface` y, al final de la fila, «Elegida» con check en `accent`; nunca solo por color (el radio y el texto lo dicen también).
- **Póster:** miniatura de 48×72 px (2:3, radio `sm`) desde `posterFile` (`/api/images/poster/{file}`), con carga diferida (`loading="lazy"`) y el póster generado de siempre (degradado con la inicial) si no hay imagen o falla. En la biblioteca se usa el póster guardado del título.
- **Cuántos se ven:** cada sección muestra **5 resultados** y debajo un botón de texto «Mostrar 5 más» (con el número restante si son menos: «Mostrar 3 más») hasta agotar la lista; cambiar el texto de búsqueda vuelve a 5. La coincidencia elegida siempre se ve aunque esté más abajo (se muestra hasta ella).
- **Teclado y mando:** Tab entra al grupo en la opción elegida (o la primera); flechas y cruceta recorren las opciones de las dos secciones; Espacio, Enter o A eligen. «Mostrar 5 más» es un botón normal en el orden de tabulación tras cada sección.
- Igual en la lista en línea de Editar/Vincular y en el diálogo de Asignar (mismo componente de opción). En móvil la tarjeta ocupa el ancho y la miniatura baja a 40×60.

### Barra de las cuadrículas y filtro de géneros (RF-41, decisión 2026-10-21)
- **Barra:** a la izquierda «Ordenar» (`SegmentedControl`) y el botón «Géneros»; a la derecha «Seleccionar». En series, «Solo incompletas» sigue debajo. En móvil la barra hace `flex-wrap` y «Seleccionar» queda al final de la última línea.
- **Botón «Géneros»:** `Button` `secondary` `sm` con chevron hacia abajo (gira al abrir); texto «Géneros», el nombre del género si hay uno, o «Géneros · N» si hay varios. Con filtro activo el borde es `accent` (como `SearchField` con texto). `aria-haspopup="true"`, `aria-expanded` y `aria-controls`.
- **Lista:** panel `surface-2` con borde `line`, radio `md`, sombra, ancho mínimo 240 px y máximo 320 px, alto máximo 60vh con scroll; se abre debajo del botón alineado a su izquierda (en móvil, a lo ancho del contenido). Cada opción es una casilla real (`input type=checkbox` dentro de `label`) de 44 px de alto como mínimo, casilla de 20 px con borde `control` y, marcada, fondo `accent` con check `on-accent`; hover `surface-3`. Al final, separada por una línea, «Quitar filtro» (botón de texto, desactivado sin filtro). El filtro se aplica al marcar y la lista sigue abierta.
- **Interacción:** se cierra con Escape, B del mando, clic o toque fuera, o al volver a pulsar el botón; al cerrar, el foco vuelve al botón. Al abrir, el foco va a la primera casilla; flechas arriba/abajo (y la cruceta del mando) recorren las opciones, Espacio o A marcan. El recuento de títulos del encabezado se actualiza al filtrar.

### Selección múltiple en Películas y Series (RF-41, decisión 2026-10-21)
- **Entrada:** botón secundario «Seleccionar» (icono de casilla) al final de la barra de orden y género; en modo selección pasa a «Cancelar».
- **Póster en modo selección:** casilla de 24 px arriba a la izquierda sobre un círculo `surface/80`; marcado = fondo `accent` con check, y el póster lleva un anillo `accent` de 3 px y se oscurece un 20 %. El póster es un botón con `role="checkbox"` y `aria-checked` y el nombre del título (no un enlace); el foco visible es el de siempre. Sin hover especial.
- **Barra de selección:** fija abajo, ancho de contenido, `surface-2` con borde `line` y sombra; en móvil va encima de la navegación inferior (nunca la tapa). Izquierda: «N seleccionados» (`aria-live="polite"`) y «Seleccionar todo» / «Deseleccionar todo» como botón de texto; derecha: «Mandar a la papelera» (botón peligro, desactivado con 0) y «Cancelar». En móvil los botones van a lo ancho en dos filas. Con mando se muestran los glifos A (marcar), B (cancelar) y X (papelera) solo si hay mando conectado.
- **Confirmación:** `ConfirmDialog` de peligro (foco inicial en Cancelar): título «¿Mandar N títulos a la papelera?», cuerpo con hasta 5 nombres y «y N más», y el aviso de que se pueden restaurar desde Ajustes → Papelera durante la retención. Mientras se mandan, el botón muestra progreso «Mandando 3 de 8…» y la barra no admite cambios.
- **Resultado:** `Banner` sobre la cuadrícula, descartable: `info` con `role=status` «N títulos en la papelera» si todo va bien; si alguno falla, `danger` con `role=alert` y los nombres que fallaron con su motivo, que siguen seleccionados. No hay componente de toast.

Subidas es un panel (escritorio) o una hoja (móvil) sobre la pantalla actual, sin ruta; la pregunta de relación es un diálogo. La cabecera de **todas** las pantallas tiene siempre el botón «Subidas» (abre el panel o la hoja), separado de «Subir» (abre el selector de archivos); mientras haya trabajos activos o esperando al usuario muestra su número (RF-10). En móvil es un botón de icono con el número como insignia, junto al de subir.

## 6. Contenido y casos límite

- **Títulos largos:** en pósters, hasta 3 líneas y luego `…`; completos en la ficha. Archivos y rutas en `mono` con `overflow-wrap: anywhere`, nunca recortados en Asignar, Editar y «Así quedará».
- **Inglés:** los textos crecen ~30 %; los botones no tienen ancho fijo y las barras hacen `flex-wrap`.
- **Tamaños y fechas** con `Intl` según el idioma (`16,9 GB` / `16.9 GB`; «1 dic 2017»). Duraciones como «1 h 59 min».
- **Imágenes:** de `/api/images`; si faltan o fallan, póster o backdrop generado (§1). Logo del hero según RF-61; sin logo, el título con `display-hero`. Sin títulos con backdrop no hay hero (RF-40).
- **TMDB caído:** en la ficha, aviso `warning` y datos en caché; en Asignar, aviso y nombre libre; «Actualizar datos» fallido, aviso `danger` y datos anteriores (RF-44).
- **Títulos libres y «Sin vincular»:** chip «Sin vincular» en el póster; la ficha muestra solo lo que hay en disco y «Vincular» en lugar de «Editar y reasignar».
- **Vacíos y errores:** plantillas de `Estados` (biblioteca vacía, No asignados vacío, papelera vacía, búsqueda sin resultados, no se pudo cargar, Jellyfin sin configurar / con error / sin rutas detectadas, login con error, sesión vencida, conflictos al guardar `plan_changed` / `lock_lost`, guardado fallido sin cambios).
- **Entrada de No asignados sin archivos visibles** (solo carpetas ocultas o del sistema): se lista con «Sin archivos visibles» y la explicación, sin acciones (pendiente de decidir en la Fase 10).
- **Carga:** esqueletos con la forma del contenido (pósters, filas de capítulos) después de 300 ms.
- **Ficha sin datos de TMDB guardados** (decisión 2026-10-21): la ficha se muestra de inmediato con lo que hay en disco y lo ya guardado (título, archivos, temporadas y capítulos presentes), y se completa sola cuando termina la carga de TMDB en segundo plano (póster, sinopsis, capítulos faltantes y próximos), sin volver al esqueleto. Las recargas en segundo plano (eventos SSE de subidas o del catálogo) nunca vacían una pantalla que ya tiene datos.
- **Errores del backend:** el `detail` viene en inglés y no se muestra; la UI traduce por código (`error` de los trabajos, `code` de salud y de Jellyfin) o por estado HTTP.
- **Modo demo (RF-72):** banner fijo `info` arriba («Modo demo: los cambios se pierden al recargar la página.»; el estado vive en memoria, como en GameExplorer), sin login, y «Probar con archivos de ejemplo» junto a «Subir» en escritorio. En < 768 px no va en la cabecera: está arriba del panel/hoja de Subidas y en el estado vacío de Subir.

## 7. Idiomas (i18n)

- `i18next` + `react-i18next`, recursos en `apps/web/src/shared/i18n/{es,en}.json`, empaquetados.
- Español por defecto; la elección se guarda en el navegador (`localStorage` `ve.lang`) y actualiza `<html lang>`. Los textos de TMDB vienen en `TMDB_LANGUAGE` y no se traducen en el cliente.
- Claves por pantalla y componente (`assign.duplicate.keepBoth`); plurales con `_one` / `_other`. ESLint `i18next/no-literal-string` impide texto escrito en JSX.

## 8. Accesibilidad (resumen operativo)

- Un único `<h1>` por pantalla. En Inicio es «Inicio» (oculto visualmente); el título del hero es `h2` y el carrusel no es región viva mientras rota.
- Landmarks: `header`, `nav` (principal y la barra inferior móvil), `main`, `footer` (barras de ayuda o atajos). Diálogos con `role=dialog`, `aria-modal` y `aria-labelledby`.
- Orden de foco: cabecera → contenido → acciones → barra de ayuda. Al abrir un diálogo o hoja, foco en el primer control; al cerrar, vuelve al botón que lo abrió. El panel lateral de Subidas no atrapa el foco.
- Cambios de estado de los trabajos en una región `role=status` del panel; errores graves con `role=alert`. La cuenta atrás del login es `role=status` y se anuncia una vez.
- Botones de icono siempre con `aria-label` traducido; íconos decorativos con `aria-hidden`.
- Contraste verificado en [`design-review.md`](./design-review.md) §2; ningún texto se atenúa con `opacity`.
