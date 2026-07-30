import { ModuleViewPreferencesService } from './module-view-preferences.service'

describe('ModuleViewPreferencesService', () => {
  let service: ModuleViewPreferencesService

  beforeEach(() => {
    localStorage.clear()
    service = new ModuleViewPreferencesService()
  })

  it('starts every module in basic automatic mode', () => {
    expect(service.get('memory')).toEqual({ version: 1, mode: 'basic', openSections: {}, navigationMode: 'auto' })
  })

  it('persists only the selected module view state', () => {
    service.setMode('memory', 'advanced')
    service.setSection('memory', 'records', true)
    service.setNavigationMode('memory', 'compact')

    expect(service.get('memory')).toEqual({
      version: 1,
      mode: 'advanced',
      openSections: { records: true },
      navigationMode: 'compact',
    })
    expect(service.get('workflow-engine')).toEqual({ version: 1, mode: 'basic', openSections: {}, navigationMode: 'auto' })
  })

  it('falls back safely when stored preferences are invalid', () => {
    localStorage.setItem('hai.module-view.v1.memory', '{not-json')
    expect(service.get('memory').mode).toBe('basic')

    localStorage.setItem('hai.module-view.v1.memory', JSON.stringify({ version: 0, mode: 'advanced' }))
    expect(service.get('memory').mode).toBe('basic')
  })

  it('resets one module without touching another', () => {
    service.setMode('memory', 'advanced')
    service.setMode('workflow-engine', 'advanced')
    service.reset('memory')

    expect(service.get('memory').mode).toBe('basic')
    expect(service.get('workflow-engine').mode).toBe('advanced')
  })
})
