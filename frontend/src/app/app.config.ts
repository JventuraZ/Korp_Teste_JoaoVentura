import { provideHttpClient, withInterceptors } from '@angular/common/http';
import { ApplicationConfig, provideBrowserGlobalErrorListeners } from '@angular/core';
import { provideRouter } from '@angular/router';
import Aura from '@primeuix/themes/aura';
import { MessageService } from 'primeng/api';
import { providePrimeNG } from 'primeng/config';

import { routes } from './app.routes';
import { interceptorDeErro } from './nucleo/erro.interceptor';

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(routes),

    provideHttpClient(withInterceptors([interceptorDeErro])),

    providePrimeNG({
      theme: {
        preset: Aura,
        options: {
          // Classe controlada pelo TemaService, em vez do padrao 'system':
          // permite ao usuario escolher explicitamente, sem perder a opcao de
          // acompanhar o sistema operacional.
          darkModeSelector: '.tema-escuro',
        },
      },
    }),

    MessageService,
  ],
};
