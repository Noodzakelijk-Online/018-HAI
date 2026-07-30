import { Injectable } from '@angular/core'

export type HaiViewMode = 'basic' | 'advanced'
export type HaiNavigationMode = 'auto' | 'expanded' | 'compact'

export interface HaiModuleViewPreferences {
  version: 1
  mode: HaiViewMode
  openSections: Record<string, boolean>
  navigationMode: HaiNavigationMode
}

const DEFAULT_PREFERENCES: HaiModuleViewPreferences = {
  version: 1,
  mode: 'basic',
  openSections: {},
  navigationMode: 'auto',
}

@Injectable({ providedIn: 'root' })
export class ModuleViewPreferencesService {
  private readonly prefix = 'hai.module-view.v1.'

  get(moduleId: string): HaiModuleViewPreferences {
    try {
      const raw = window.localStorage.getItem(this.key(moduleId))
      if (!raw) return this.copy(DEFAULT_PREFERENCES)
      const parsed = JSON.parse(raw) as Partial<HaiModuleViewPreferences>
      if (parsed.version !== 1) return this.copy(DEFAULT_PREFERENCES)
      return {
        version: 1,
        mode: parsed.mode === 'advanced' ? 'advanced' : 'basic',
        openSections: parsed.openSections && typeof parsed.openSections === 'object' ? parsed.openSections : {},
        navigationMode: parsed.navigationMode === 'compact' || parsed.navigationMode === 'expanded' ? parsed.navigationMode : 'auto',
      }
    } catch {
      return this.copy(DEFAULT_PREFERENCES)
    }
  }

  setMode(moduleId: string, mode: HaiViewMode): HaiModuleViewPreferences {
    return this.update(moduleId, { mode })
  }

  setSection(moduleId: string, sectionId: string, open: boolean): HaiModuleViewPreferences {
    const current = this.get(moduleId)
    return this.write(moduleId, { ...current, openSections: { ...current.openSections, [sectionId]: open } })
  }

  setNavigationMode(moduleId: string, navigationMode: HaiNavigationMode): HaiModuleViewPreferences {
    return this.update(moduleId, { navigationMode })
  }

  reset(moduleId: string): HaiModuleViewPreferences {
    try { window.localStorage.removeItem(this.key(moduleId)) } catch { /* local storage can be disabled */ }
    return this.copy(DEFAULT_PREFERENCES)
  }

  private update(moduleId: string, patch: Partial<HaiModuleViewPreferences>): HaiModuleViewPreferences {
    return this.write(moduleId, { ...this.get(moduleId), ...patch })
  }

  private write(moduleId: string, value: HaiModuleViewPreferences): HaiModuleViewPreferences {
    const normalized = this.copy(value)
    try { window.localStorage.setItem(this.key(moduleId), JSON.stringify(normalized)) } catch { /* best effort only */ }
    return normalized
  }

  private key(moduleId: string): string { return `${this.prefix}${moduleId}` }
  private copy(value: HaiModuleViewPreferences): HaiModuleViewPreferences {
    return { ...value, openSections: { ...value.openSections } }
  }
}
