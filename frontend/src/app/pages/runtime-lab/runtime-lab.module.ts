import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { RouterModule, Routes } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzCardModule } from 'ng-zorro-antd/card'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzNotificationModule } from 'ng-zorro-antd/notification'
import { NzTagModule } from 'ng-zorro-antd/tag'
import { RuntimeLabComponent } from './runtime-lab.component'

const routes: Routes = [{ path: '', component: RuntimeLabComponent }]

@NgModule({
  declarations: [RuntimeLabComponent],
  imports: [
    CommonModule,
    RouterModule.forChild(routes),
    NzButtonModule,
    NzCardModule,
    NzIconModule,
    NzNotificationModule,
    NzTagModule,
  ],
})
export class RuntimeLabModule {}
