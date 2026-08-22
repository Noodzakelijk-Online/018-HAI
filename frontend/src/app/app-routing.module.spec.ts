import { APP_ROUTES } from './app-routing.module'
import { AppShellComponent } from './control-room/app-shell.component'
import { authGuard } from './services/auth/guards/auth.guard'

describe('application routing', () => {
  it('protects every route rendered inside the operational shell', () => {
    const shellRoute = APP_ROUTES.find((route) => route.component === AppShellComponent)

    expect(shellRoute).toBeDefined()
    expect(shellRoute?.canActivateChild).toContain(authGuard)
  })
})
