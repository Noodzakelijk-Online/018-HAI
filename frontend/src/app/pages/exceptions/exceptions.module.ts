import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule, Routes } from '@angular/router';
import { NzTableModule } from 'ng-zorro-antd/table';
import { NzCardModule } from 'ng-zorro-antd/card';
import { NzTagModule } from 'ng-zorro-antd/tag';
import { NzEmptyModule } from 'ng-zorro-antd/empty';
import { NzButtonModule } from 'ng-zorro-antd/button';
import { NzSpinModule } from 'ng-zorro-antd/spin';
import { ExceptionsComponent } from './exceptions.component';

const routes: Routes = [{ path: '', component: ExceptionsComponent }];

@NgModule({
  declarations: [ExceptionsComponent],
  imports: [
    CommonModule,
    RouterModule.forChild(routes),
    NzTableModule,
    NzCardModule,
    NzTagModule,
    NzEmptyModule,
    NzButtonModule,
    NzSpinModule,
  ],
})
export class ExceptionsModule {}
