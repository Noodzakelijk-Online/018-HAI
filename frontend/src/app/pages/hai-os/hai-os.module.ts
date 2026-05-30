import { NgModule } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterModule, Routes } from '@angular/router';
import { NzButtonModule } from 'ng-zorro-antd/button';
import { NzCardModule } from 'ng-zorro-antd/card';
import { NzIconModule } from 'ng-zorro-antd/icon';
import { NzLayoutModule } from 'ng-zorro-antd/layout';
import { NzNotificationModule } from 'ng-zorro-antd/notification';
import { NzTableModule } from 'ng-zorro-antd/table';
import { HAIOSComponent } from './hai-os.component';
import { HAI_OS_SERVICE_TOKEN } from '../../services/hai-os/hai-os.service.token';
import { HAIOSService } from '../../services/hai-os/hai-os.service';

const routes: Routes = [{ path: '', component: HAIOSComponent }];

@NgModule({
  declarations: [HAIOSComponent],
  imports: [
    CommonModule,
    RouterModule.forChild(routes),
    NzButtonModule,
    NzCardModule,
    NzIconModule,
    NzLayoutModule,
    NzNotificationModule,
    NzTableModule,
  ],
  providers: [{ provide: HAI_OS_SERVICE_TOKEN, useClass: HAIOSService }],
})
export class HAIOSModule {}
