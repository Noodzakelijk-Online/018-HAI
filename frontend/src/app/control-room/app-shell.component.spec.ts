import { ViewEncapsulation } from '@angular/core'
import { AppShellComponent } from './app-shell.component'

describe('AppShellComponent', () => {
  it('applies shared authenticated workspace styles without component scoping', () => {
    expect((AppShellComponent as any).ɵcmp.encapsulation).toBe(ViewEncapsulation.None)
  })
})
