import { NgModule } from "@angular/core";
import { RouterModule, Routes } from "@angular/router";
import { authGuard } from "./services/auth/guards/auth.guard";
import { RedirectIfLoggedGuard } from "./services/auth/guards/login.guard";

const routes: Routes = [
  {
    path: "login",
    loadChildren: () =>
      import("./pages/login/login.module").then((m) => m.LoginModule),
    canActivate: [RedirectIfLoggedGuard],
  },
  {
    path: "home",
    loadChildren: () =>
      import("./pages/home/home.module").then((m) => m.HomeModule),
    canActivate: [authGuard],
  },
  {
    path: "command-dashboard",
    loadChildren: () =>
      import("./pages/command-dashboard/command-dashboard.module").then(
        (m) => m.CommandDashboardModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "ambient-brain",
    loadChildren: () =>
      import("./pages/ambient-brain/ambient-brain.module").then(
        (m) => m.AmbientBrainModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "hai-os",
    loadChildren: () =>
      import("./pages/hai-os/hai-os.module").then((m) => m.HAIOSModule),
    canActivate: [authGuard],
  },
  {
    path: "control-center",
    loadChildren: () =>
      import("./pages/control-center/control-center.module").then(
        (m) => m.ControlCenterModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "llm-policy",
    loadChildren: () =>
      import("./pages/llm-policy/llm-policy.module").then(
        (m) => m.LLMPolicyModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "memory",
    loadChildren: () =>
      import("./pages/memory/memory.module").then((m) => m.MemoryModule),
    canActivate: [authGuard],
  },
  {
    path: "task-blueprint",
    loadChildren: () =>
      import("./pages/task-blueprint/task-blueprint.module").then(
        (m) => m.TaskBlueprintModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "connected-sources",
    loadChildren: () =>
      import("./pages/connected-sources/connected-sources.module").then(
        (m) => m.ConnectedSourcesModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "grounded-answers",
    loadChildren: () =>
      import("./pages/grounded-answers/grounded-answers.module").then(
        (m) => m.GroundedAnswersModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "workflow-engine",
    loadChildren: () =>
      import("./pages/workflow-engine/workflow-engine.module").then(
        (m) => m.WorkflowEngineModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "pursuits",
    loadChildren: () =>
      import("./pages/pursuits/pursuits.module").then((m) => m.PursuitsModule),
    canActivate: [authGuard],
  },
  {
    path: "background-operations",
    loadChildren: () =>
      import("./pages/background-operations/background-operations.module").then(
        (m) => m.BackgroundOperationsModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "onboarding",
    loadChildren: () =>
      import("./pages/onboarding/onboarding.module").then(
        (m) => m.OnboardingModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "exceptions",
    loadChildren: () =>
      import("./pages/exceptions/exceptions.module").then(
        (m) => m.ExceptionsModule
      ),
    canActivate: [authGuard],
  },
  {
    path: "quick-capture",
    loadChildren: () =>
      import("./pages/quick-capture/quick-capture.module").then(
        (m) => m.QuickCaptureModule
      ),
    canActivate: [authGuard],
  },
  // {
  //   path: 'home',
  //   loadChildren: () =>
  //     import('./pages/home/home.module').then((m) => m.HomeModule),
  // },
  { path: "", redirectTo: "/control-center", pathMatch: "full" },
  { path: "**", redirectTo: "/control-center" },
];

@NgModule({
  imports: [RouterModule.forRoot(routes)],
  exports: [RouterModule],
})
export class AppRoutingModule {}
