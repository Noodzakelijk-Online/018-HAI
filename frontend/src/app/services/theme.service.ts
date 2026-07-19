import { Injectable } from '@angular/core'

export type ThemeMode = 'light' | 'dark'

@Injectable({
  providedIn: 'root',
})
export class ThemeService {
  private readonly storageKey = 'hai-theme-mode'
  private readonly legacyStorageKey = 'hai-control-center-theme'
  private currentMode: ThemeMode = 'dark'

  constructor() {
    this.currentMode = this.load()
    this.apply(this.currentMode)
  }

  mode(): ThemeMode {
    return this.currentMode
  }

  setMode(mode: ThemeMode): ThemeMode {
    this.currentMode = mode
    this.persist(mode)
    this.apply(mode)
    return mode
  }

  toggle(): ThemeMode {
    return this.setMode(this.currentMode === 'dark' ? 'light' : 'dark')
  }

  label(): string {
    return this.currentMode === 'dark' ? 'Dark mode' : 'Light mode'
  }

  icon(): string {
    return this.currentMode === 'dark' ? 'eye-invisible' : 'bulb'
  }

  private load(): ThemeMode {
    try {
      const saved =
        window.localStorage.getItem(this.storageKey) ||
        window.localStorage.getItem(this.legacyStorageKey)
      return saved === 'light' ? 'light' : 'dark'
    } catch {
      return 'dark'
    }
  }

  private persist(mode: ThemeMode): void {
    try {
      window.localStorage.setItem(this.storageKey, mode)
      window.localStorage.setItem(this.legacyStorageKey, mode)
    } catch {
      // Some hardened browser contexts disable localStorage.
    }
  }

  private apply(mode: ThemeMode): void {
    const root = document.documentElement
    const body = document.body
    root.setAttribute('data-hai-theme', mode)
    root.classList.toggle('hai-theme-dark', mode === 'dark')
    root.classList.toggle('hai-theme-light', mode === 'light')
    body.classList.toggle('hai-theme-dark', mode === 'dark')
    body.classList.toggle('hai-theme-light', mode === 'light')
    body.style.colorScheme = mode
  }
}
