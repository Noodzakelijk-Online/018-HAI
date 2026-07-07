import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule, Routes } from '@angular/router';
import { NzStepsModule } from 'ng-zorro-antd/steps';
import { NzButtonModule } from 'ng-zorro-antd/button';
import { NzCardModule } from 'ng-zorro-antd/card';
import { OnboardingComponent } from './onboarding.component';

const routes: Routes = [{ path: '', component: OnboardingComponent }];

@NgModule({
  declarations: [OnboardingComponent],
  imports: [
    CommonModule,
    RouterModule.forChild(routes),
    NzStepsModule,
    NzButtonModule,
    NzCardModule,
  ],
})
export class OnboardingModule {}
