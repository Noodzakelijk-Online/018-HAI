import { FormBuilder } from '@angular/forms'
import { Router } from '@angular/router'
import { NzNotificationService } from 'ng-zorro-antd/notification'
import { ILLMPolicyService } from '../../services/llm-policy.service.interface'
import { ThemeService } from '../../services/theme.service'
import { LLMPolicyComponent } from './llm-policy.component'

describe('LLMPolicyComponent theme icon', () => {
  it('uses the centrally registered theme icon', () => {
    const themeService = jasmine.createSpyObj<ThemeService>(
      'ThemeService',
      ['mode', 'toggle', 'label', 'icon']
    )
    themeService.mode.and.returnValue('dark')
    themeService.icon.and.returnValue('star')

    const component = new LLMPolicyComponent(
      new FormBuilder(),
      {} as ILLMPolicyService,
      {} as NzNotificationService,
      {} as Router,
      themeService
    )

    expect(component.themeIcon()).toBe('star')
    expect(themeService.icon).toHaveBeenCalled()
  })
})
