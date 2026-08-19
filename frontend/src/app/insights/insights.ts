import { DatePipe, DecimalPipe } from '@angular/common';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { ButtonModule } from 'primeng/button';
import { SelectButtonModule } from 'primeng/selectbutton';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { catchError, finalize, forkJoin, of } from 'rxjs';

import { AnaliseService } from '../nucleo/analise.service';
import { AnomaliaDetectada, PrevisaoProduto, Risco } from '../nucleo/modelos';

@Component({
  selector: 'app-insights',
  imports: [
    FormsModule,
    DatePipe,
    DecimalPipe,
    TableModule,
    TagModule,
    ButtonModule,
    SelectButtonModule,
  ],
  templateUrl: './insights.html',
})
export class Insights implements OnInit {
  private readonly api = inject(AnaliseService);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly previsoes = signal<PrevisaoProduto[]>([]);
  protected readonly anomalias = signal<AnomaliaDetectada[]>([]);
  protected readonly resumo = signal<Partial<Record<Risco, number>>>({});
  protected readonly carregando = signal(false);
  protected readonly geradoEm = signal<string | null>(null);

  /** Janela de histórico considerada. Trocar recalcula tudo no servidor. */
  protected janela = 90;
  protected readonly janelas = [
    { rotulo: '30 dias', valor: 30 },
    { rotulo: '90 dias', valor: 90 },
    { rotulo: '180 dias', valor: 180 },
  ];

  ngOnInit(): void {
    this.carregar();
  }

  protected carregar(): void {
    this.carregando.set(true);

    forkJoin({
      previsao: this.api.previsao(this.janela).pipe(catchError(() => of(null))),
      anomalias: this.api.anomalias(this.janela).pipe(catchError(() => of(null))),
    })
      .pipe(
        finalize(() => this.carregando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe(({ previsao, anomalias }) => {
        if (previsao) {
          this.previsoes.set(previsao.itens);
          this.resumo.set(previsao.resumo);
          this.geradoEm.set(previsao.geradoEm);
        }
        if (anomalias) {
          this.anomalias.set(anomalias.itens);
        }
      });
  }

  protected contar(risco: Risco): number {
    return this.resumo()[risco] ?? 0;
  }

  /**
   * Traduz a projeção numérica para a frase que o operador precisa ler.
   * "acaba em 4 dias" é acionável; "diasAteRuptura: 4.2" não é.
   */
  protected prazoEmPalavras(item: PrevisaoProduto): string {
    if (item.risco === 'SEM_DADOS') {
      return 'histórico insuficiente';
    }
    if (item.diasAteRuptura === null) {
      return 'sem consumo no período';
    }
    if (item.saldo === 0) {
      return 'estoque zerado';
    }
    if (item.diasAteRuptura < 1) {
      return 'acaba hoje';
    }
    if (item.diasAteRuptura < 2) {
      return 'acaba amanhã';
    }
    return `acaba em ~${Math.round(item.diasAteRuptura)} dias`;
  }

  protected severidade(risco: Risco): 'danger' | 'warn' | 'success' | 'secondary' {
    switch (risco) {
      case 'CRITICO':
        return 'danger';
      case 'ATENCAO':
        return 'warn';
      case 'OK':
        return 'success';
      default:
        return 'secondary';
    }
  }

  protected rotuloRisco(risco: Risco): string {
    switch (risco) {
      case 'CRITICO':
        return 'Crítico';
      case 'ATENCAO':
        return 'Atenção';
      case 'OK':
        return 'Ok';
      default:
        return 'Sem dados';
    }
  }

  /** Quantas vezes a baixa superou a mediana do produto. */
  protected vezesAcima(anomalia: AnomaliaDetectada): number {
    if (anomalia.medianaDoProduto <= 0) {
      return 0;
    }
    return anomalia.quantidade / anomalia.medianaDoProduto;
  }
}
