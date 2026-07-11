export interface ISetupRequirement {
  step: string
  detail: string
}

export interface IBridgeContract {
  provider: string
  displayName: string
  connectorPreference: string[]
  readOnly: boolean
  requiredScopes: string[]
  credentialEnv?: string
  itemTypes: string[]
  setupRequirements: ISetupRequirement[]
  connectionStatus: string
}

export interface IAccountPermission {
  provider: string
  displayName: string
  readOnly: boolean
  declaredScopes: string[]
  credentialEnv?: string
  granted: boolean
  status: string
}

export interface IFeed {
  id: string
  name: string
  provider: string
  accountLabel: string
  sourceType: string
  path?: string
  url?: string
  operationType?: string
  enabled: boolean
}

export interface IFeedHealth {
  feed: IFeed
  connectionStatus: string
  lastSyncedAt?: string
  lastItemsRead: number
}

export interface ISyncReport {
  feedId: string
  itemsRead: number
  operationsCreated: number
  operationsRefreshed: number
  privacyFlagged: number
  cursor?: string
  errors?: string[]
}

export interface IFeedAudit {
  id: string
  feedId: string
  eventType: string
  message: string
  createdAt: string
}
