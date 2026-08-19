import {
  AfterViewInit,
  Component,
  DestroyRef,
  ElementRef,
  OnDestroy,
  OnInit,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { MessageService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { DialogModule } from 'primeng/dialog';
import { InputNumberModule } from 'primeng/inputnumber';
import { IconFieldModule } from 'primeng/iconfield';
import { InputIconModule } from 'primeng/inputicon';
import { InputTextModule } from 'primeng/inputtext';
import { TableModule, TableLazyLoadEvent } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { TooltipModule } from 'primeng/tooltip';
import { Subject, catchError, debounceTime, distinctUntilChanged, finalize, of, switchMap, tap } from 'rxjs';

import { Produto } from '../nucleo/modelos';
import { ProdutosService } from '../nucleo/produtos.service';

@Component({
  selector: 'app-produtos',
  imports: [
    FormsModule,
    TableModule,
    ButtonModule,
    InputTextModule,
    InputNumberModule,
    IconFieldModule,
    InputIconModule,
    DialogModule,
    TagModule,
    TooltipModule,
    RouterLink,
  ],
  templateUrl: './produtos.html',
})
export class Produtos implements OnInit, AfterViewInit, OnDestroy {
  private readonly api = inject(ProdutosService);
  private readonly mensagens = inject(MessageService);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly produtos = signal<Produto[]>([]);
  protected readonly total = signal(0);
  protected readonly carregando = signal(false);

  protected readonly tamanhoPagina = 10;
  protected termoBusca = '';
  private paginaAtual = 1;

  /** Fonte do autocomplete: cada tecla empurra um termo para o pipeline. */
  private readonly termos = new Subject<string>();

  private readonly campoBusca = viewChild<ElementRef<HTMLInputElement>>('campoBusca');

  protected readonly dialogoAberto = signal(false);
  protected readonly salvando = signal(false);
  protected emEdicao: Produto | null = null;
  protected formulario = { codigo: '', descricao: '', saldo: 0 };

  /**
   * ngOnInit: monta o pipeline reativo da busca.
   *
   * debounceTime(300) espera o usuario parar de digitar -- sem ele, "parafuso"
   * dispararia oito requisicoes. distinctUntilChanged ignora teclas que nao
   * mudam o termo (setas, backspace que reverte). switchMap CANCELA a busca
   * anterior quando chega um termo novo, o que evita o bug classico de uma
   * resposta lenta antiga sobrescrever a resposta rapida atual.
   */
  ngOnInit(): void {
    this.termos
      .pipe(
        debounceTime(300),
        distinctUntilChanged(),
        tap(() => {
          this.paginaAtual = 1;
          this.carregando.set(true);
        }),
        switchMap((termo) =>
          this.api.listar(termo, 1, this.tamanhoPagina).pipe(catchError(() => of(null))),
        ),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((pagina) => {
        this.carregando.set(false);
        if (pagina) {
          this.produtos.set(pagina.itens);
          this.total.set(pagina.total);
        }
      });
  }

  /** ngAfterViewInit: o foco so pode ir para o campo depois de ele existir no DOM. */
  ngAfterViewInit(): void {
    this.campoBusca()?.nativeElement.focus();
  }

  /** ngOnDestroy: fecha o Subject para nao deixar produtor pendurado. */
  ngOnDestroy(): void {
    this.termos.complete();
  }

  protected aoDigitar(termo: string): void {
    this.termos.next(termo);
  }

  protected carregarPagina(evento: TableLazyLoadEvent): void {
    this.paginaAtual = Math.floor((evento.first ?? 0) / this.tamanhoPagina) + 1;
    this.recarregar();
  }

  protected recarregar(): void {
    this.carregando.set(true);
    this.api
      .listar(this.termoBusca, this.paginaAtual, this.tamanhoPagina)
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.carregando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((pagina) => {
        if (pagina) {
          this.produtos.set(pagina.itens);
          this.total.set(pagina.total);
        }
      });
  }

  protected abrirNovo(): void {
    this.emEdicao = null;
    this.formulario = { codigo: '', descricao: '', saldo: 0 };
    this.dialogoAberto.set(true);
  }

  protected abrirEdicao(produto: Produto): void {
    this.emEdicao = produto;
    this.formulario = {
      codigo: produto.codigo,
      descricao: produto.descricao,
      saldo: produto.saldo,
    };
    this.dialogoAberto.set(true);
  }

  protected salvar(): void {
    const { codigo, descricao, saldo } = this.formulario;
    if (!codigo.trim() || !descricao.trim()) {
      this.mensagens.add({
        severity: 'warn',
        summary: 'Campos obrigatórios',
        detail: 'Informe código e descrição.',
      });
      return;
    }

    this.salvando.set(true);

    const requisicao = this.emEdicao
      ? this.api.atualizar(this.emEdicao.codigo, descricao, saldo)
      : this.api.criar(codigo, descricao, saldo);

    requisicao
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.salvando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((produto) => {
        if (!produto) {
          return;
        }
        this.mensagens.add({
          severity: 'success',
          summary: this.emEdicao ? 'Produto atualizado' : 'Produto cadastrado',
          detail: `${produto.codigo} — saldo ${produto.saldo}`,
        });
        this.dialogoAberto.set(false);
        this.recarregar();
      });
  }

  protected severidadeDoSaldo(saldo: number): 'success' | 'warn' | 'danger' {
    if (saldo === 0) return 'danger';
    return saldo <= 5 ? 'warn' : 'success';
  }
}
