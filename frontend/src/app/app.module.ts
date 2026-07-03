import {NgModule} from '@angular/core';
import {BrowserModule} from '@angular/platform-browser';
import {AppRoutingModule} from './app-routing.module';
import {AppComponent} from './app.component';
import {NZ_I18N} from 'ng-zorro-antd/i18n';
import {en_US} from 'ng-zorro-antd/i18n';
import {registerLocaleData} from '@angular/common';
import en from '@angular/common/locales/en';
import {HttpClientModule} from "@angular/common/http";
import {BrowserAnimationsModule} from '@angular/platform-browser/animations';
import {AUTH_SERVICE_TOKEN} from './services/auth/auth.service.token';
import {AuthService} from './services/auth/auth.service';

registerLocaleData(en);

@NgModule({
    declarations: [
        AppComponent,
    ],
    imports: [
        BrowserModule,
        AppRoutingModule,
        HttpClientModule,
        BrowserAnimationsModule
    ],
    providers: [
        {provide: NZ_I18N, useValue: en_US},
        {provide: AUTH_SERVICE_TOKEN, useClass: AuthService},
    ],
    bootstrap: [AppComponent]
})
export class AppModule {
}
