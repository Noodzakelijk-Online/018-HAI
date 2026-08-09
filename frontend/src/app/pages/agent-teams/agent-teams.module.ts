import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { FormsModule } from '@angular/forms'
import { RouterModule, Routes } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzDrawerModule } from 'ng-zorro-antd/drawer'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzNotificationModule } from 'ng-zorro-antd/notification'
import { NzSpinModule } from 'ng-zorro-antd/spin'
import { ControlRoomModule } from '../../control-room/control-room.module'
import { AgentTeamsComponent } from './agent-teams.component'

const routes: Routes = [{ path: '', component: AgentTeamsComponent }]

@NgModule({
  declarations: [AgentTeamsComponent],
  imports: [
    CommonModule,
    FormsModule,
    RouterModule.forChild(routes),
    ControlRoomModule,
    NzButtonModule,
    NzDrawerModule,
    NzIconModule,
    NzNotificationModule,
    NzSpinModule,
  ],
})
export class AgentTeamsModule {}
