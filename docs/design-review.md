# Revisión de diseño (Fase 9)

Fuente: lienzo [`/design`](https://claude.ai/artifact/A2ENfFRCjVUMMcjMMiZCqb), 19 pantallas aprobadas por el usuario el 2026-10-16; tras esta revisión tiene 22. Pasadas: `design:design-critique` (usabilidad, jerarquía, consistencia) y `design:accessibility-review` (WCAG 2.1 AA, RNF-05 a RNF-07). Cada hallazgo indica su corrección; la columna «Estado» dice si ya está dibujada en el lienzo o si queda como regla para el traspaso ([`design-handoff.md`](./design-handoff.md)).

## 1. Crítica de diseño

**Impresión general.** La estética de cine (fondo casi negro, pósters grandes, hero a pantalla completa) cumple el objetivo de RF-40 y el acento turquesa marca bien las acciones principales sin competir con los pósters. La mayor oportunidad estaba en los flujos que el lienzo no dibujaba: editar desde la ficha, la cuadrícula de series y Asignar una película.

**Lo que funciona.**
- El hero cuenta en dos segundos qué hay y qué se puede hacer (Ver detalles / Editar); las filas tienen flechas siempre visibles, así que nada depende del hover (RNF-05).
- Los estados de los trabajos usan siempre texto más color, y la barra de progreso cambia de color entre subida y descompresión (WCAG 1.4.1).
- Asignar separa bien «qué es cada video» (izquierda) de «así quedará» (derecha) y bloquea Guardar con un motivo visible mientras haya duplicados sin decidir (RF-24, RF-25).
- La ficha de serie hace evidente lo que falta: chip «Falta», miniatura atenuada y aviso con el recuento (RF-43a).

### Usabilidad
| # | Hallazgo | Severidad | Corrección | Estado |
|---|---|---|---|---|
| C1 | No había pantalla de **Editar y reasignar** (RF-44), ni la vista previa de **Vincular** o de **quitar/poner la etiqueta de ID** (RF-45): el botón Editar llevaba a Asignar. | 🔴 Crítica | Pantalla propia «Editar y reasignar» para una película con versiones: coincidencia de TMDB, tipo, etiqueta de versión, idioma de subtítulos, etiqueta de ID y vista previa de renombres con Guardar. Vincular y la etiqueta de ID usan la misma pantalla. | Dibujado (`Editar`) |
| C2 | Faltaba la cuadrícula de **Series** (RF-41); «Series» y «2 series incompletas» llevaban a una ficha. | 🟡 Moderada | Pantalla Series igual que Películas, con el filtro extra «Incompletas» y el chip «Faltan N» en el póster; Pendientes abre Series con ese filtro. | Dibujado (`Series`) |
| C3 | Asignar solo mostraba el caso de capítulos. Faltaban la película en **partes** (número de parte), **No guardar**, el título por video con «Sin relación» y la etiqueta de versión **incompleta** (audio `und`, RF-24). | 🟡 Moderada | Segunda pantalla de Asignar para una película en dos partes, un video «No guardar» y un duplicado con etiqueta incompleta marcada en el campo. | Dibujado (`Asignar película`) |
| C4 | En No asignados cada fila tenía cuatro botones al mismo nivel, con **Borrar definitivamente** al lado de Asignar: ruido y riesgo de un clic equivocado. | 🟡 Moderada | Asignar queda como acción principal; Descargar, A la papelera y Borrar definitivamente pasan a un menú «Más acciones» por entrada (y por archivo). | Dibujado |
| C5 | El modo demo (RF-72) no tenía banner ni el botón «Probar con archivos de ejemplo». | 🟡 Moderada | Banner fijo de demo y estado vacío de Subir con «Probar con archivos de ejemplo». | Dibujado (`Estados`) |
| C6 | La barra de ayuda con glifos de gamepad (RNF-06) no estaba dibujada. | 🟡 Moderada | Barra inferior con glifos A/B/X/Y y LB/RB que solo aparece con un control conectado; en Asignar sustituye a la de atajos de teclado. | Dibujado (`Estados`) |
| C7 | En móvil, Asignar no mostraba los subtítulos ni la hoja «Así quedará». | 🟢 Menor | Mismo contenido que escritorio, apilado; «Así quedará» se abre como hoja inferior modal. | Regla de traspaso |

### Jerarquía visual
- **Lo primero que se ve:** el título del hero en Inicio y el título de la ficha en las fichas. Correcto.
- **Recorrido:** hero → pendientes → filas en Inicio; título → acciones → archivos en las fichas. Las tarjetas de Pendientes tienen el peso justo para no competir con el hero.
- **Énfasis:** una sola acción turquesa por zona (Subir en la cabecera, Ver detalles en el hero, Asignar en cada entrada, Guardar en Asignar). Las acciones destructivas usan el rojo solo en texto y borde.

### Consistencia
| Elemento | Problema | Corrección | Estado |
|---|---|---|---|
| Cabecera | El indicador de Subidas solo aparecía en Inicio y el icono de Ajustes era distinto entre pantallas. | El indicador aparece en toda pantalla mientras haya trabajos activos o esperando al usuario (RF-10). Un solo juego de iconos de trazo de 2 px. | Regla de traspaso |
| Panel de Subidas | En escritorio oscurecía la página pero se declaraba no modal. | Panel lateral no modal, sin oscurecer, para seguir navegando; en móvil, hoja inferior modal. | Dibujado |
| Barra inferior en móvil | Solo texto de 12 px. | Icono + texto de 13 px. | Regla de traspaso |

## 2. Auditoría de accesibilidad (WCAG 2.1 AA)

**Resumen:** 12 hallazgos · 1 crítico · 7 importantes · 4 menores. Todos corregidos en el lienzo o convertidos en regla del traspaso.

### Perceptible
| # | Hallazgo | Criterio | Severidad | Corrección | Estado |
|---|---|---|---|---|---|
| A1 | En la fila de colección de la ficha de película, las películas que faltan se atenuaban con `opacity: .55` en toda la tarjeta: «No la tienes» quedaba a 4,0:1 y el turquesa a 3,7:1. | 1.4.3 | 🔴 Crítico | Se atenúa solo el póster (borde punteado y fondo apagado); el texto conserva su color, como en la pantalla Colección. | Dibujado |
| A2 | En los pósters el título se leía dos veces (dentro del póster y debajo) y los chips («Faltan 4», «Sin vincular») no formaban parte del nombre del enlace. | 1.3.1 | 🟡 Importante | El título dentro del póster es decorativo (`aria-hidden`); el nombre del enlace es «título, año, chips». | Dibujado |
| A3 | En Inicio el `h1` era el título del hero, que cambia cada 8 s. | 1.3.1 | 🟡 Importante | `h1` «Inicio» (oculto visualmente) y el título del hero como `h2`; el carrusel sin región viva mientras rota. | Dibujado |
| A4 | La rejilla de Info técnica tenía 4 columnas fijas: no se reacomoda a 320 px ni con zoom al 400 %. | 1.4.10 | 🟡 Importante | Rejilla `auto-fit` de columnas de 220 px como mínimo. | Dibujado |
| A5 | Bordes decorativos (`line` a 1,45:1) y el borde punteado de las películas que faltan (2,1:1). | 1.4.11 | 🟢 Menor | Aceptado: son decorativos y el estado siempre va en texto. Los controles usan `control` (#7a7a86, 3,96:1 sobre `surface-2`, el peor caso). | Sin cambio |

### Operable
| # | Hallazgo | Criterio | Severidad | Corrección | Estado |
|---|---|---|---|---|---|
| A6 | Los atajos de una sola letra de Asignar (R, A, O) se disparaban en cualquier parte de la página, también al escribir. | 2.1.4 | 🟡 Importante | Solo actúan cuando el foco está en la tarjeta de un video y fuera de un campo de texto; Ctrl+Enter y Esc funcionan siempre. La barra de atajos lo dice. | Dibujado |
| A7 | Los controles segmentados (orden, género, duplicados) usan radios ocultos: el foco no se veía en la etiqueta. | 2.4.7 | 🟡 Importante | Anillo de foco en la etiqueta cuando su radio tiene foco (`label:has(:focus-visible)`). | Dibujado |
| A8 | Los indicadores del carrusel medían 36 × 44 px. | RNF-05 (2.5.5) | 🟢 Menor | 44 × 44 px. | Dibujado |
| A9 | Rotación del hero (> 5 s). | 2.2.2 | 🟡 Importante | Botón Pausar/Reanudar visible, pausa con hover o foco, sin rotación con `prefers-reduced-motion` (ya estaba dibujado). El botón cambia de nombre al pulsarlo. | Regla de traspaso |
| A10 | Panel de Subidas y diálogos: falta definir el foco. | 2.4.3 | 🟡 Importante | Diálogo y hoja móvil: foco al primer control, trampa de foco, Esc cierra y el foco vuelve al botón que lo abrió. Panel lateral no modal: sin trampa, Esc lo cierra. | Regla de traspaso |

### Comprensible
| # | Hallazgo | Criterio | Severidad | Corrección | Estado |
|---|---|---|---|---|---|
| A11 | Avisos por video en Asignar («TMDB no conoce este capítulo», etiqueta incompleta) no estaban ligados al campo. | 3.3.1 | 🟢 Menor | `aria-describedby` del campo al aviso y `aria-invalid` en la etiqueta incompleta, con el texto de qué falta. | Dibujado |

### Robusto
| # | Hallazgo | Criterio | Severidad | Corrección | Estado |
|---|---|---|---|---|---|
| A12 | La cuenta atrás del límite de intentos del login era `role="alert"`: se anunciaría cada segundo. | 4.1.3 | 🟢 Menor | `role="status"` (educado) que se anuncia una vez; el botón Entrar queda deshabilitado con el motivo. | Regla de traspaso |

### Contraste de la paleta
| Elemento | Texto | Fondo | Ratio | Mínimo | ¿Pasa? |
|---|---|---|---|---|---|
| Texto principal | `#f5f5f7` | `#0c0c0f` | 17,9:1 | 4,5:1 | ✅ |
| Texto secundario | `#c4c4cc` | `#0c0c0f` | 11,3:1 | 4,5:1 | ✅ |
| Texto terciario (peor fondo) | `#9696a1` | `#1d1d23` | 5,7:1 | 4,5:1 | ✅ |
| Acento como texto | `#35d0ba` | `#15151a` | 9,4:1 | 4,5:1 | ✅ |
| Texto sobre acento | `#04201c` | `#35d0ba` | 8,9:1 | 4,5:1 | ✅ |
| Aviso | `#ffe3a0` | `#2e2614` | 11,9:1 | 4,5:1 | ✅ |
| Peligro como texto | `#ff8577` | `#15151a` | 7,7:1 | 4,5:1 | ✅ |
| Texto sobre peligro | `#2a0b07` | `#ff8577` | 7,7:1 | 4,5:1 | ✅ |
| Borde de control (peor fondo) | `#7a7a86` | `#1d1d23` | 4,0:1 | 3:1 | ✅ |
| Texto atenuado al 55 % (antes de A1) | `#c4c4cc` | `#0c0c0f` | 4,0:1 | 4,5:1 | ❌ → corregido |

### Teclado
| Elemento | Tab | Enter/Espacio | Esc | Flechas |
|---|---|---|---|---|
| Fila de pósters | Un tab por póster, en orden | Abre la ficha | — | ←/→ entre pósters de la fila; ↑/↓ entre filas (navegación espacial, como GameExplorer) |
| Pestañas de temporada | Entra en la seleccionada | Selecciona | — | ←/→ entre temporadas |
| Menú «Más acciones» | Botón | Abre y enfoca el primer elemento | Cierra y vuelve al botón | ↑/↓ |
| Asignar | Tarjeta por tarjeta | Acepta la sugerencia | Cancelar (con confirmación si hay cambios) | ↑/↓ entre videos |
| Diálogos | Atrapado dentro | Acción enfocada | Cierra | — |

### Lector de pantalla
| Elemento | Se anuncia como | Nota |
|---|---|---|
| Póster | «Dark, 2017, serie, faltan 4, enlace» | Tras A2 |
| Hero | «Agregados recientemente destacados, carrusel» y el `h2` del título | Tras A3; sin región viva |
| Progreso | «Descomprimiendo Interstellar.2014.7z, barra de progreso, 63 %» | `aria-valuenow` |
| Estado de un trabajo | Texto del chip («Necesita contraseña») | Los cambios de estado se anuncian en una región `status` del panel |

## 3. Prioridades
1. **Pantallas que faltaban** (C1, C2, C3): sin ellas la Fase 10 no tiene diseño para RF-41, RF-44 y RF-45.
2. **Contraste y nombres accesibles** (A1, A2, A3): afectan a todo lector de pantalla y a quien tiene baja visión, en las pantallas más usadas.
3. **Atajos y foco** (A6, A7, A10): teclado y gamepad son requisito (RNF-06).
