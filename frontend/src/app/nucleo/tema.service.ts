import { DOCUMENT } from '@angular/common';
import { Injectable, effect, inject, signal } from '@angular/core';

export type PreferenciaTema = 'claro' | 'escuro' | 'sistema';

const CHAVE = 'korp.tema';
const CLASSE_ESCURA = 'tema-escuro';

/**
 * Controla o tema claro/escuro da aplicação.
 *
 * Três estados, e o terceiro importa: "sistema" acompanha a preferência do
 * sistema operacional em tempo real. Quem trabalha com troca automática ao
 * anoitecer espera que o ERP acompanhe, em vez de ficar preso no que escolheu
 * de manhã.
 */
@Injectable({ providedIn: 'root' })
export class TemaService {
  private readonly documento = inject(DOCUMENT);
  private readonly consultaSistema = this.documento.defaultView?.matchMedia?.(
    '(prefers-color-scheme: dark)',
  );

  readonly preferencia = signal<PreferenciaTema>(this.lerPreferencia());

  /** O que está de fato na tela — resolve "sistema" para claro ou escuro. */
  readonly escuroAtivo = signal(false);

  constructor() {
    // Mudança na preferência do SO só reflete quando o usuário está em "sistema".
    this.consultaSistema?.addEventListener('change', () => {
      if (this.preferencia() === 'sistema') {
        this.aplicar();
      }
    });

    effect(() => {
      const preferencia = this.preferencia();
      this.documento.defaultView?.localStorage?.setItem(CHAVE, preferencia);
      this.aplicar();
    });
  }

  /** Alterna entre claro → escuro → sistema, em ciclo. */
  alternar(): void {
    const proxima: Record<PreferenciaTema, PreferenciaTema> = {
      claro: 'escuro',
      escuro: 'sistema',
      sistema: 'claro',
    };
    this.preferencia.set(proxima[this.preferencia()]);
  }

  rotulo(): string {
    switch (this.preferencia()) {
      case 'claro': return 'Tema claro';
      case 'escuro': return 'Tema escuro';
      default: return 'Acompanhando o sistema';
    }
  }

  icone(): string {
    switch (this.preferencia()) {
      case 'claro': return 'pi pi-sun';
      case 'escuro': return 'pi pi-moon';
      default: return 'pi pi-desktop';
    }
  }

  private aplicar(): void {
    const escuro =
      this.preferencia() === 'escuro' ||
      (this.preferencia() === 'sistema' && (this.consultaSistema?.matches ?? false));

    this.documento.documentElement.classList.toggle(CLASSE_ESCURA, escuro);
    this.escuroAtivo.set(escuro);
  }

  private lerPreferencia(): PreferenciaTema {
    const gravada = this.documento.defaultView?.localStorage?.getItem(CHAVE);
    return gravada === 'claro' || gravada === 'escuro' || gravada === 'sistema'
      ? gravada
      : 'sistema';
  }
}
