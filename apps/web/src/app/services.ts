import type { CatalogPorts } from '@/modules/catalog/application/ports'
import type { IdentityPorts } from '@/modules/identity/application/ports'
import type { IngestionPorts } from '@/modules/ingestion/application/ports'
import type { MediaServersPorts } from '@/modules/mediaservers/application/ports'
import type { MetadataPorts } from '@/modules/metadata/application/ports'
import type { SystemPorts } from '@/modules/system/application/ports'
import { createApiClient } from '@/shared/api/client'

import type { DataSource } from './config'

export interface Services {
  catalog: CatalogPorts
  identity: IdentityPorts
  ingestion: IngestionPorts
  mediaServers: MediaServersPorts
  metadata: MetadataPorts
  system: SystemPorts
}

export async function createHttpServices(): Promise<Services> {
  const [catalog, identity, ingestion, mediaServers, metadata, system] = await Promise.all([
    import('@/modules/catalog/infrastructure/http/catalogHttp'),
    import('@/modules/identity/infrastructure/http/identityHttp'),
    import('@/modules/ingestion/infrastructure/http/ingestionHttp'),
    import('@/modules/mediaservers/infrastructure/http/mediaServersHttp'),
    import('@/modules/metadata/infrastructure/http/metadataHttp'),
    import('@/modules/system/infrastructure/http/systemHttp'),
  ])
  const client = createApiClient()
  return {
    catalog: catalog.createCatalogHttp(client),
    identity: identity.createIdentityHttp(client),
    ingestion: ingestion.createIngestionHttp(client),
    mediaServers: mediaServers.createMediaServersHttp(client),
    metadata: metadata.createMetadataHttp(client),
    system: system.createSystemHttp(client),
  }
}

export async function createDemoServices(): Promise<Services> {
  const [catalogModule, identity, ingestion, mediaServers, metadata, system] = await Promise.all([
    import('@/modules/catalog/infrastructure/demo/catalogDemo'),
    import('@/modules/identity/infrastructure/demo/identityDemo'),
    import('@/modules/ingestion/infrastructure/demo/ingestionDemo'),
    import('@/modules/mediaservers/infrastructure/demo/mediaServersDemo'),
    import('@/modules/metadata/infrastructure/demo/metadataDemo'),
    import('@/modules/system/infrastructure/demo/systemDemo'),
  ])
  const catalog = catalogModule.createCatalogDemo()
  return {
    catalog,
    identity: identity.createIdentityDemo(),
    ingestion: ingestion.createIngestionDemo(catalog),
    mediaServers: mediaServers.createMediaServersDemo(),
    metadata: metadata.createMetadataDemo(catalog),
    system: system.createSystemDemo(),
  }
}

/** Composes the selected adapter set; the UI only sees the module ports. */
export function createServices(source: DataSource): Promise<Services> {
  switch (source) {
    case 'http':
      return createHttpServices()
    case 'demo':
      return createDemoServices()
  }
}
