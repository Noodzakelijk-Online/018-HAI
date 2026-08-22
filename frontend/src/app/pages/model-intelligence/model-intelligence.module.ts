import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { RouterModule, Routes } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzSpinModule } from 'ng-zorro-antd/spin'
import { ModelIntelligenceComponent } from './model-intelligence.component'
import { ControlRoomModule } from '../../control-room/control-room.module'

const routes: Routes = [{ path: '', component: ModelIntelligenceComponent }]

@NgModule({
  declarations: [ModelIntelligenceComponent],
  imports: [
    CommonModule,
    RouterModule.forChild(routes),
    ControlRoomModule,
    NzButtonModule,
    NzIconModule,
    NzSpinModule,
  ],
})
export class ModelIntelligenceModule {}
