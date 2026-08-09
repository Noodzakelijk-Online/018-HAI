import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { FormsModule } from '@angular/forms'
import { RouterModule, Routes } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzDrawerModule } from 'ng-zorro-antd/drawer'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzInputModule } from 'ng-zorro-antd/input'
import { NzModalModule } from 'ng-zorro-antd/modal'
import { NzSpinModule } from 'ng-zorro-antd/spin'
import { ControlRoomModule } from '../../control-room/control-room.module'
import { GovernanceControlComponent } from './governance-control.component'

const routes: Routes = [{ path: '', component: GovernanceControlComponent }]

@NgModule({
  declarations: [GovernanceControlComponent],
  imports: [
    CommonModule,
    FormsModule,
    RouterModule.forChild(routes),
    ControlRoomModule,
    NzButtonModule,
    NzDrawerModule,
    NzIconModule,
    NzInputModule,
    NzModalModule,
    NzSpinModule,
  ],
})
export class GovernanceControlModule {}
