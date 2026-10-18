import type { CatalogPorts } from '@/modules/catalog/application/ports'
import { createCatalogHttp } from '@/modules/catalog/infrastructure/http/catalogHttp'
import { createCatalogDemo } from '@/modules/catalog/infrastructure/demo/catalogDemo'
import type { IdentityPorts } from '@/modules/identity/application/ports'
import { createIdentityHttp } from '@/modules/identity/infrastructure/http/identityHttp'
import { createIdentityDemo } from '@/modules/identity/infrastructure/demo/identityDemo'
import type { IngestionPorts } from '@/modules/ingestion/application/ports'
import { createIngestionHttp } from '@/modules/ingestion/infrastructure/http/ingestionHttp'
import { createIngestionDemo } from '@/modules/ingestion/infrastructure/demo/ingestionDemo'
import type { MediaServersPorts } from '@/modules/mediaservers/application/ports'
import { createMediaServersHttp } from '@/modules/mediaservers/infrastructure/http/mediaServersHttp'
import { createMediaServersDemo } from '@/modules/mediaservers/infrastructure/demo/mediaServersDemo'
import type { MetadataPorts } from '@/modules/metadata/application/ports'
import { createMetadataHttp } from '@/modules/metadata/infrastructure/http/metadataHttp'
import { createMetadataDemo } from '@/modules/metadata/infrastructure/demo/metadataDemo'
import type { SystemPorts } from '@/modules/system/application/ports'
import { createSystemHttp } from '@/modules/system/infrastructure/http/systemHttp'
import { createSystemDemo } from '@/modules/system/infrastructure/demo/systemDemo'
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

export function createHttpServices(): Services {
  const client = createApiClient()
  return {
    catalog: createCatalogHttp(client),
    identity: createIdentityHttp(client),
    ingestion: createIngestionHttp(client),
    mediaServers: createMediaServersHttp(client),
    metadata: createMetadataHttp(client),
    system: createSystemHttp(client),
  }
}

export function createDemoServices(): Services {
  const catalog = createCatalogDemo()
  return {
    catalog,
    identity: createIdentityDemo(),
    ingestion: createIngestionDemo(catalog),
    mediaServers: createMediaServersDemo(),
    metadata: createMetadataDemo(catalog),
    system: createSystemDemo(),
  }
}

/** Composes the selected adapter set; the UI only sees the module ports. */
export function createServices(source: DataSource): Services {
  switch (source) {
    case 'http':
    case 'demo':
      return createDemoServices()
  }
}
