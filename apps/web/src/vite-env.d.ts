/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** "http" talks to the Go API; "demo" runs fully in the browser (Vercel). */
  readonly VITE_DATA_SOURCE?: 'http' | 'demo'
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
