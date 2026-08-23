import { of } from 'rxjs'
import { ICalibrationSummary, IModelIntelligenceOverview } from '../../models/model-intelligence.model.interface'
import { ModelIntelligenceService } from '../../services/model-intelligence.service'
import { ModelIntelligenceComponent } from './model-intelligence.component'

describe('ModelIntelligenceComponent', () => {
  let service: jasmine.SpyObj<ModelIntelligenceService>
  let notification: jasmine.SpyObj<any>
  let component: ModelIntelligenceComponent

  const calibration: ICalibrationSummary = {
    totalRuns: 8, evaluatedRuns: 5, acceptedOutputs: 4, rejectedOutputs: 1,
    needsReview: 0, unvalidatedRuns: 3, models: [],
    laneLeaders: [{
      lane: 'triage', providerId: 'local', modelId: 'capable', tokensPerSecond: 12,
      runs: 5, evaluatedRuns: 5, acceptedOutputs: 4, acceptanceRate: 0.8,
      confidence: 'emerging', averageTokens: 100, averageDurationMs: 800,
      averageCostEur: 0, reason: '4/5 evaluated outputs accepted.',
    }],
    generatedAt: '2026-08-04T10:00:00Z',
    explanation: 'Accepted output evidence ranks before efficiency.',
  }
  const overview: IModelIntelligenceOverview = {
    providers: [], lanes: ['triage'], totalProfiles: 2, activeModels: 1,
    telemetryRuns: 8, evaluatedRuns: 5, acceptedOutputs: 4, unvalidatedRuns: 3,
    cacheHits: 0, cacheMisses: 0, laneWinners: [], calibration,
  }

  beforeEach(() => {
    service = jasmine.createSpyObj<ModelIntelligenceService>('ModelIntelligenceService', [
      'overview', 'profiles', 'tokenBudgets', 'hardware', 'powerPolicy',
      'benchmark', 'detectHardware',
    ])
    notification = jasmine.createSpyObj('NzNotificationService', ['success', 'warning', 'error'])
    service.overview.and.returnValue(of(overview))
    service.profiles.and.returnValue(of({ profiles: [] }))
    service.tokenBudgets.and.returnValue(of({
      maximumInputTokens: 4096, maximumOutputTokens: 1024, maximumReasoningEffort: 'medium',
      maximumContextItems: 12, maximumSourceBytes: 1000000, contextStrategy: 'relevant_only',
      cacheStrategy: 'exact', batchEligible: true,
    }))
    service.hardware.and.returnValue(of({
      profile: { operatingSystem: 'windows', windowsVersion: '11', cpuCores: 8, gpuVendor: '',
        npuVendor: '', executionProviders: [], powerMode: 'balanced', batteryStatus: 'unknown' },
      selectedServingStack: 'ollama',
    }))
    service.powerPolicy.and.returnValue(of({
      mode: 'balanced', allowHeavyWorkNow: true, deferHeavyWorkOnBattery: true, nightBatchOnly: false,
    }))
    component = new ModelIntelligenceComponent(service, notification)
  })

  it('loads completion evidence without eagerly loading advanced diagnostics', () => {
    component.ngOnInit()

    expect(component.acceptancePercent()).toBe(80)
    expect(component.unresolvedOutcomes()).toBe(1)
    expect(component.calibration?.laneLeaders[0].reason).toContain('evaluated outputs accepted')
    expect(service.profiles).not.toHaveBeenCalled()
    expect(service.hardware).not.toHaveBeenCalled()
  })

  it('loads profiles and runtime details only when their sections open', () => {
    component.ngOnInit()
    component.onProfilesOpen(true)
    component.onRuntimeOpen(true)

    expect(service.profiles).toHaveBeenCalledTimes(1)
    expect(service.tokenBudgets).toHaveBeenCalledTimes(1)
    expect(service.hardware).toHaveBeenCalledTimes(1)
    expect(service.powerPolicy).toHaveBeenCalledTimes(1)
    expect(component.profilesLoaded).toBeTrue()
    expect(component.runtimeLoaded).toBeTrue()
  })
})
