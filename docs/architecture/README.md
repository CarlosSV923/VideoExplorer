# Diagramas de arquitectura

Los candidatos JSON de este directorio son la fuente editable de los cuatro diagramas. Para regenerarlos se necesita Node.js y la skill archify v3.2.0 instalada en `C:\Users\carlo\.agents\skills\archify`. Desde la raíz del repositorio, ejecuta:

```powershell
node C:\Users\carlo\.agents\skills\archify\bin\archify.mjs finalize architecture docs/architecture/architecture.en.json docs/architecture/site/architecture.html --repo-root . --quality showcase --json
node C:\Users\carlo\.agents\skills\archify\bin\archify.mjs finalize architecture docs/architecture/architecture.es.json docs/architecture/site/es/architecture.html --repo-root . --quality showcase --json
node C:\Users\carlo\.agents\skills\archify\bin\archify.mjs finalize lifecycle docs/architecture/upload-lifecycle.en.json docs/architecture/site/upload-lifecycle.html --repo-root . --quality showcase --json
node C:\Users\carlo\.agents\skills\archify\bin\archify.mjs finalize lifecycle docs/architecture/upload-lifecycle.es.json docs/architecture/site/es/upload-lifecycle.html --repo-root . --quality showcase --json
```

Cada candidato fija la revisión del repositorio usada como evidencia. Al actualizar las fuentes o sus citas, actualiza también `meta.repository.revision` con el commit correspondiente y vuelve a finalizar los diagramas afectados. La página índice no depende de CDN y Pages publica directamente el contenido de `site/`.
