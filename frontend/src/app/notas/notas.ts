import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { DatePipe } from '@angular/common';
import { MessageService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { TableModule, TableLazyLoadEvent } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { catchError, finalize, of } from 'rxjs';

import { NotaResumo } from '../nucleo/modelos';
import { NotasService } from '../nucleo/notas.service';
import { DetalheNota } from './detalhe-nota';

@Component({
  selector: 'app-notas',
  imports: [TableModule, ButtonModule, TagModule, DatePipe, DetalheNota],
  templateUrl: './notas.html',
})
export class Notas implements OnInit {
  private readonly api = inject(NotasService);
  private readonly mensagens = inject(MessageService);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly notas = signal<NotaResumo[]>([]);
  protected readonly total = signal(0);
  protected readonly carregando = signal(false);
  protected readonly criando = signal(false);

  /** Id repassado ao componente de detalhe; a troca dispara o ngOnChanges dele. */
  protected readonly selecionada = signal<string | null>(null);

  protected readonly tamanhoPagina = 12;
  private paginaAtual = 1;

  ngOnInit(): void {
    this.recarregar();
  }

  protected carregarPagina(evento: TableLazyLoadEvent): void {
    this.paginaAtual = Math.floor((evento.first ?? 0) / this.tamanhoPagina) + 1;
    this.recarregar();
  }

  protected recarregar(): void {
    this.carregando.set(true);
    this.api
      .listar(this.paginaAtual, this.tamanhoPagina)
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.carregando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((pagina) => {
        if (!pagina) {
          return;
        }
        this.notas.set(pagina.itens);
        this.total.set(pagina.total);

        if (!this.selecionada() && pagina.itens.length > 0) {
          this.selecionada.set(pagina.itens[0].id);
        }
      });
  }

  protected criar(): void {
    this.criando.set(true);
    this.api
      .criar()
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.criando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((nota) => {
        if (!nota) {
          return;
        }
        this.mensagens.add({
          severity: 'success',
          summary: `${nota.numeroFormatado} criada`,
          detail: 'Numeração sequencial gerada pelo banco.',
        });
        this.recarregar();
        this.selecionada.set(nota.id);
      });
  }
}
