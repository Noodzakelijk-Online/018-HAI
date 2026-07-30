import { CommonModule } from '@angular/common'
import { NgModule } from '@angular/core'
import { RouterModule } from '@angular/router'
import { NzButtonModule } from 'ng-zorro-antd/button'
import { NzIconModule } from 'ng-zorro-antd/icon'
import { AppShellComponent } from './app-shell.component'
import { HaiProgressiveSectionComponent } from './progressive-section.component'

@NgModule({
  declarations: [AppShellComponent, HaiProgressiveSectionComponent],
  imports: [CommonModule, RouterModule, NzButtonModule, NzIconModule],
  exports: [HaiProgressiveSectionComponent],
})
export class ControlRoomModule {}
