export interface IBrowserVerificationProfile {
  id: string
  name: string
  url: string
  expectedPath?: string
}

export interface IBrowserVerificationStatus {
  enabled: boolean
  configured: boolean
  runnerUrl?: string
  profiles: IBrowserVerificationProfile[]
  configError?: string
  scope: string
}
