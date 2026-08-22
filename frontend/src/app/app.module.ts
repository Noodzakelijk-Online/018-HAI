import {ErrorHandler, NgModule} from '@angular/core';
import {BrowserModule} from '@angular/platform-browser';
import {AppRoutingModule} from './app-routing.module';
import {AppComponent} from './app.component';
import {NZ_I18N} from 'ng-zorro-antd/i18n';
import {en_US} from 'ng-zorro-antd/i18n';
import {registerLocaleData} from '@angular/common';
import en from '@angular/common/locales/en';
import {HTTP_INTERCEPTORS, provideHttpClient, withInterceptorsFromDi} from '@angular/common/http';
import {BrowserAnimationsModule} from '@angular/platform-browser/animations';
import {BulbOutline, CalendarOutline, ContactsOutline, HeartOutline, NodeIndexOutline, StarOutline} from '@ant-design/icons-angular/icons';
import {NZ_ICONS} from 'ng-zorro-antd/icon';
import {AUTH_SERVICE_TOKEN} from './services/auth/auth.service.token';
import {AuthService} from './services/auth/auth.service';
import {ChunkLoadRecoveryHandler} from './services/chunk-load-recovery.handler';
import {ControlRoomModule} from './control-room/control-room.module';
import {RequestTimeoutInterceptor} from './interceptors/request-timeout.interceptor';

registerLocaleData(en);

export const HAI_ICONS = [BulbOutline, CalendarOutline, ContactsOutline, HeartOutline, NodeIndexOutline, StarOutline];

@NgModule({
    declarations: [
        AppComponent,
    ],
    imports: [
        BrowserModule,
        AppRoutingModule,
        BrowserAnimationsModule,
        ControlRoomModule,
    ],
    providers: [
        {provide: NZ_I18N, useValue: en_US},
        {provide: NZ_ICONS, useValue: HAI_ICONS},
        {provide: AUTH_SERVICE_TOKEN, useClass: AuthService},
        {provide: ErrorHandler, useClass: ChunkLoadRecoveryHandler},
        {provide: HTTP_INTERCEPTORS, useClass: RequestTimeoutInterceptor, multi: true},
        provideHttpClient(withInterceptorsFromDi()),
    ],
    bootstrap: [AppComponent],
})
export class AppModule {
}
