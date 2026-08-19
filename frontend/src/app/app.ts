import { HttpClient } from '@angular/common/http';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { ButtonModule } from 'primeng/button';
import { TagModule } from 'primeng/tag';
import { TooltipModule } from 'primeng/tooltip';
import { ToastModule } from 'primeng/toast';
import { catchError, interval, of, startWith, switchMap } from 'rxjs';

import { AnaliseService } from './nucleo/analise.service';
import { TemaService } from './nucleo/tema.service';

interface Saude {
  status: string;
  banco: string;
  estoque?: string;
}

@Component({
  selector: 'app-root',
  imports: [
    RouterOutlet, RouterLink, RouterLinkActive,
    ToastModule, TagModule, ButtonModule, TooltipModule,
  ],
  templateUrl: './app.html',
  styleUrl: './app.css',
})
export class App implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly analise = inject(AnaliseService);

  /** Público: o template chama alternar() e lê o ícone. */
  protected readonly tema = inject(TemaService);
  private readonly destroyRef = inject(DestroyRef);

  /** Estado do Estoque, visto pelo Faturamento e exibido no cabecalho. */
  protected readonly estoqueOnline = signal(true);

  /** Produtos com ruptura prevista para menos de 7 dias. */
  protected readonly criticos = signal(0);

  ngOnInit(): void {
    interval(60_000)
      .pipe(
        startWith(0),
        switchMap(() => this.analise.previsao().pipe(catchError(() => of(null)))),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((painel) => this.criticos.set(painel?.resumo?.CRITICO ?? 0));

    interval(5000)
      .pipe(
        startWith(0),
        switchMap(() =>
          this.http.get<Saude>('/api/saude-faturamento').pipe(catchError(() => of(null))),
        ),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((saude) => this.estoqueOnline.set(saude?.estoque === 'ok'));
  }
}
