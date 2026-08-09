import { provideZoneChangeDetection } from '@angular/core';
import { platformBrowser } from '@angular/platform-browser';

import { AppModule } from './app/app.module';


// HAI's existing modules update mutable view state from HttpClient and RxJS
// subscriptions. Angular 22 defaults to zoneless scheduling and OnPush
// components, so retain the official compatibility path while modules migrate
// incrementally to signals. Coalescing avoids redundant application ticks.
platformBrowser().bootstrapModule(AppModule, {
  applicationProviders: [
    provideZoneChangeDetection({
      eventCoalescing: true,
      runCoalescing: true,
    }),
  ],
})
  .catch(err => console.error(err));
