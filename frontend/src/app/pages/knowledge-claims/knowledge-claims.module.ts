import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { FormsModule } from '@angular/forms'
import { RouterModule, Routes } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzCheckboxModule } from 'ng-zorro-antd/checkbox'
import { NzDrawerModule } from 'ng-zorro-antd/drawer'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzInputModule } from 'ng-zorro-antd/input'
import { NzSpinModule } from 'ng-zorro-antd/spin'
import { NzTagModule } from 'ng-zorro-antd/tag'
import { ControlRoomModule } from '../../control-room/control-room.module'
import { KnowledgeClaimsComponent } from './knowledge-claims.component'

const routes: Routes = [{ path: '', component: KnowledgeClaimsComponent }]

@NgModule({
  declarations: [KnowledgeClaimsComponent],
  imports: [
    CommonModule,
    FormsModule,
    RouterModule.forChild(routes),
    ControlRoomModule,
    NzButtonModule,
    NzCheckboxModule,
    NzDrawerModule,
    NzIconModule,
    NzInputModule,
    NzSpinModule,
    NzTagModule,
  ],
})
export class KnowledgeClaimsModule {}
