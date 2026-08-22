import { FormBuilder } from '@angular/forms'
import { Router } from '@angular/router'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { of } from 'rxjs'
import { ILLMPolicyService } from '../../services/llm-policy.service.interface'
import { ThemeService } from '../../services/theme.service'
import { LLMPolicyComponent } from './llm-policy.component'

describe('LLMPolicyComponent', () => {
  function createComponent(): LLMPolicyComponent {
    const themeService = jasmine.createSpyObj<ThemeService>(
      'ThemeService',
      ['mode', 'toggle', 'label', 'icon']
    )
    themeService.mode.and.returnValue('dark')
    themeService.icon.and.returnValue('star')

    return new LLMPolicyComponent(
      new FormBuilder(),
      {} as ILLMPolicyService,
      {} as NzNotificationService,
      {} as Router,
      themeService
    )
  }

  it('uses the centrally registered theme icon', () => {
    const component = createComponent()

    expect(component.themeIcon()).toBe('star')
  })

  it('explains conservative validator evidence for a routed model', () => {
    const component = createComponent()

    expect(component.calibrationSummary({
      calibration: {
        lane: 'recursive_deep_review',
        evaluatedRuns: 10,
        acceptedOutputs: 8,
        acceptanceRate: 0.8,
        wilsonLowerBound: 0.49,
        confidence: 'medium',
      },
    } as any)).toBe(
      '8/10 validator-accepted outputs, 49.0% conservative lower bound, medium confidence.'
    )
  })

  it('keeps trusted, review, failed, and unevaluated outcomes visually distinct', () => {
    const component = createComponent()

    expect(component.validationLabel('source_supported')).toBe('source supported')
    expect(component.validationColor('test_passed')).toBe('green')
    expect(component.validationColor('needs_review')).toBe('gold')
    expect(component.validationColor('failed')).toBe('red')
    expect(component.validationColor()).toBe('default')
  })

  it('defers routing, probe, and maintenance history until an operator opens it', () => {
    const themeService = jasmine.createSpyObj<ThemeService>('ThemeService', ['mode', 'toggle', 'label', 'icon'])
    themeService.mode.and.returnValue('dark')
    themeService.icon.and.returnValue('star')
    const service = jasmine.createSpyObj<ILLMPolicyService>('ILLMPolicyService', [
      'getPolicy', 'getLogs', 'getGenerationHistory', 'getProbeHistory', 'getModelMaintenanceHistory',
    ])
    service.getPolicy.and.returnValue(of({ providers: [] } as any))
    service.getLogs.and.returnValue(of([]))
    service.getGenerationHistory.and.returnValue(of([]))
    service.getProbeHistory.and.returnValue(of([]))
    service.getModelMaintenanceHistory.and.returnValue(of([]))
    const component = new LLMPolicyComponent(
      new FormBuilder(), service, {} as NzNotificationService, {} as Router, themeService
    )

    component.refresh()

    expect(service.getPolicy).toHaveBeenCalled()
    expect(service.getLogs).not.toHaveBeenCalled()
    expect(service.getGenerationHistory).not.toHaveBeenCalled()
    expect(service.getProbeHistory).not.toHaveBeenCalled()
    expect(service.getModelMaintenanceHistory).not.toHaveBeenCalled()
  })
})
