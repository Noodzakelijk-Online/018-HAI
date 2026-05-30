import { NgModule } from '@angular/core'
import { CommonModule } from '@angular/common'
import { RouterModule, Routes } from '@angular/router'
import { FormsModule } from '@angular/forms'
import { NzLayoutModule } from 'ng-zorro-antd/layout'
import { NzTableModule } from 'ng-zorro-antd/table'
import { NzTagModule } from 'ng-zorro-antd/tag'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzCardModule } from 'ng-zorro-antd/card'
import { NzModalModule } from 'ng-zorro-antd/modal'
import { NzGridModule } from 'ng-zorro-antd/grid'
import { NzToolTipModule } from 'ng-zorro-antd/tooltip'
import { NzSpinModule } from 'ng-zorro-antd/spin'
import { NzDividerModule } from 'ng-zorro-antd/divider'
import { ControlCenterComponent } from './control-center.component'
import { AUTOMATIONS_SERVICE_TOKEN } from '../../services/automations/automations.service.token'
import { AutomationsService } from '../../services/automations/automations.service'

const routes: Routes = [{ path: '', component: ControlCenterComponent }]

@NgModule({
  declarations: [ControlCenterComponent],
  imports: [
    CommonModule,
    RouterModule.forChild(routes),
    FormsModule,
    NzLayoutModule,
    NzTableModule,
    NzTagModule,
    NzButtonModule,
    NzIconModule,
    NzCardModule,
    NzModalModule,
    NzGridModule,
    NzToolTipModule,
    NzSpinModule,
    NzDividerModule,
  ],
  providers: [
    { provide: AUTOMATIONS_SERVICE_TOKEN, useClass: AutomationsService },
  ],
})
export class ControlCenterModule {}
