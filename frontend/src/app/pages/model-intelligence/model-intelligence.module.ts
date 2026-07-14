import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { FormsModule } from '@angular/forms'
import { RouterModule, Routes } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzCardModule } from 'ng-zorro-antd/card'
import { NzEmptyModule } from 'ng-zorro-antd/empty'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { NzNotificationModule } from 'ng-zorro-antd/notification'
import { NzTableModule } from 'ng-zorro-antd/table'
import { NzTagModule } from 'ng-zorro-antd/tag'
import { ModelIntelligenceComponent } from './model-intelligence.component'

const routes: Routes = [{ path: '', component: ModelIntelligenceComponent }]

@NgModule({
  declarations: [ModelIntelligenceComponent],
  imports: [
    CommonModule,
    FormsModule,
    RouterModule.forChild(routes),
    NzButtonModule,
    NzCardModule,
    NzEmptyModule,
    NzIconModule,
    NzNotificationModule,
    NzTableModule,
    NzTagModule,
  ],
})
export class ModelIntelligenceModule {}
