import { HaiProgressiveSectionComponent } from './progressive-section.component'
import { ModuleViewPreferencesService } from './module-view-preferences.service'

describe('HaiProgressiveSectionComponent', () => {
  const moduleId = 'agent-teams-test'

  beforeEach(() => window.localStorage.removeItem(`hai.module-view.v1.${moduleId}`))
  afterEach(() => window.localStorage.removeItem(`hai.module-view.v1.${moduleId}`))

  it('can be opened by a primary action and persists the disclosure state', () => {
    const preferences = new ModuleViewPreferencesService()
    const section = new HaiProgressiveSectionComponent(preferences)
    section.moduleId = moduleId
    section.sectionId = 'members'
    const changes: boolean[] = []
    section.openChange.subscribe((open) => changes.push(open))

    section.setOpen(true)

    expect(section.open).toBeTrue()
    expect(changes).toEqual([true])
    expect(preferences.get(moduleId).openSections['members']).toBeTrue()
  })
})
