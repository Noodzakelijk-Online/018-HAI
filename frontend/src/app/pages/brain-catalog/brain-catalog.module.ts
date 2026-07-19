import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { RouterModule, Routes } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzCardModule } from 'ng-zorro-antd/card'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzNotificationModule } from 'ng-zorro-antd/notification'
import { NzSpinModule } from 'ng-zorro-antd/spin'
import { NzTagModule } from 'ng-zorro-antd/tag'
import { ControlRoomModule } from '../../control-room/control-room.module'
import { BrainCatalogComponent } from './brain-catalog.component'

const routes: Routes = [{ path: '', component: BrainCatalogComponent }]

@NgModule({
  declarations: [BrainCatalogComponent],
  imports: [
    CommonModule,
    RouterModule.forChild(routes),
    ControlRoomModule,
    NzButtonModule,
    NzCardModule,
    NzIconModule,
    NzNotificationModule,
    NzSpinModule,
    NzTagModule,
  ],
})
export class BrainCatalogModule {}
