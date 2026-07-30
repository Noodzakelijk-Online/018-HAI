import { Component, OnDestroy, OnInit } from '@angular/core'
import { NavigationEnd, Router } from '@angular/router'
import { Subscription } from 'rxjs'
import { filter } from 'rxjs/operators'
import { HAI_MODULE_GROUPS, HAI_MODULES, HaiModuleDefinition, moduleForUrl } from './module-registry'
import { HaiNavigationMode, HaiViewMode, ModuleViewPreferencesService } from './module-view-preferences.service'
import { ThemeMode, ThemeService } from '../services/theme.service'

@Component({
  selector: 'app-shell',
  templateUrl: './app-shell.component.html',
  styleUrls: ['./app-shell.component.scss'],
})
export class AppShellComponent implements OnInit, OnDestroy {
  readonly groups = HAI_MODULE_GROUPS
  readonly modules = HAI_MODULES
  current: HaiModuleDefinition = HAI_MODULES[0]
  themeMode: ThemeMode = 'dark'
  viewMode: HaiViewMode = 'basic'
  navigationMode: HaiNavigationMode = 'auto'
  mobileNavigationOpen = false
  private routerSubscription?: Subscription
  private readonly advancedDetailSelector = [
    'details.advanced-section',
    'details.advanced-block',
    'details.detail-block',
    'details.provider-detail',
    'details.legacy-actions',
    'details.pursuit-health',
    'details.route-intake-panel',
  ].join(', ')
  private readonly onDetailsToggle = (event: Event) => this.persistDetailState(event)

  constructor(
    private router: Router,
    private preferences: ModuleViewPreferencesService,
    private themeService: ThemeService,
  ) {}

  ngOnInit(): void {
    this.themeMode = this.themeService.mode()
    this.updateCurrent(this.router.url)
    this.routerSubscription = this.router.events.pipe(filter((event): event is NavigationEnd => event instanceof NavigationEnd))
      .subscribe((event) => this.updateCurrent(event.urlAfterRedirects))
    document.addEventListener('toggle', this.onDetailsToggle, true)
  }

  ngOnDestroy(): void {
    this.routerSubscription?.unsubscribe()
    document.removeEventListener('toggle', this.onDetailsToggle, true)
  }

  groupModules(groupId: string): HaiModuleDefinition[] {
    return this.modules.filter((module) => module.group === groupId)
  }

  navigate(module: HaiModuleDefinition): void {
    this.mobileNavigationOpen = false
    this.router.navigateByUrl(module.route)
  }

  openRoute(route: string): void {
    this.mobileNavigationOpen = false
    this.router.navigateByUrl(route)
  }

  isActive(module: HaiModuleDefinition): boolean { return module.id === this.current.id }

  toggleTheme(): void {
    this.themeMode = this.themeService.toggle()
  }

  toggleViewMode(): void {
    this.viewMode = this.viewMode === 'basic' ? 'advanced' : 'basic'
    this.preferences.setMode(this.current.id, this.viewMode)
    document.body.classList.toggle('hai-view-advanced', this.viewMode === 'advanced')
  }

  cycleNavigationMode(): void {
    const next: Record<HaiNavigationMode, HaiNavigationMode> = { auto: 'compact', compact: 'expanded', expanded: 'auto' }
    this.navigationMode = next[this.navigationMode]
    this.preferences.setNavigationMode(this.current.id, this.navigationMode)
  }

  navigationLabel(): string {
    return this.navigationMode === 'auto' ? 'Navigation: adaptive' : `Navigation: ${this.navigationMode}`
  }

  private updateCurrent(url: string): void {
    this.current = moduleForUrl(url)
    const preferences = this.preferences.get(this.current.id)
    this.viewMode = preferences.mode
    this.navigationMode = preferences.navigationMode
    document.body.classList.toggle('hai-view-advanced', this.viewMode === 'advanced')
    window.setTimeout(() => this.restoreDetailState())
  }

  private persistDetailState(event: Event): void {
    const detail = event.target as HTMLDetailsElement
    if (!(detail instanceof HTMLDetailsElement) || !detail.matches(this.advancedDetailSelector)) return
    const sectionId = this.sectionId(detail)
    this.preferences.setSection(this.current.id, sectionId, detail.open)
  }

  private restoreDetailState(): void {
    Array.from(document.querySelectorAll<HTMLDetailsElement>('.hai-module-outlet details'))
      .filter((detail) => detail.matches(this.advancedDetailSelector))
      .forEach((detail) => {
      detail.open = this.preferences.get(this.current.id).openSections[this.sectionId(detail)] === true
      })
  }

  private sectionId(detail: HTMLDetailsElement): string {
    if (detail.dataset['haiSection']) return detail.dataset['haiSection']
    const summary = detail.querySelector('summary')?.textContent?.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '') || 'detail'
    const siblings = Array.from(document.querySelectorAll<HTMLDetailsElement>('.hai-module-outlet details'))
      .filter((candidate) => candidate.matches(this.advancedDetailSelector))
    const generated = detail.id || `${summary}-${siblings.indexOf(detail)}`
    detail.dataset['haiSection'] = generated
    return generated
  }
}
