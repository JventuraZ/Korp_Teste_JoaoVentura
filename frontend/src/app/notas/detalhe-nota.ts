import {
  Component,
  DestroyRef,
  EventEmitter,
  Input,
  OnChanges,
  OnDestroy,
  OnInit,
  Output,
  SimpleChanges,
  inject,
  signal,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { MessageService } from 'primeng/api';
import { AutoCompleteModule, AutoCompleteCompleteEvent } from 'primeng/autocomplete';
import { ButtonModule } from 'primeng/button';
import { InputNumberModule } from 'primeng/inputnumber';
import { ProgressBarModule } from 'primeng/progressbar';
import { TableModule } from 'primeng/table';
import { TagModule } from 'primeng/tag';
import { Subject, catchError, debounceTime, distinctUntilChanged, finalize, of, switchMap } from 'rxjs';

import { NotaDetalhe, Produto } from '../nucleo/modelos';
import { NotasService } from '../nucleo/notas.service';
import { ProdutosService } from '../nucleo/produtos.service';

@Component({
  selector: 'app-detalhe-nota',
  imports: [
    FormsModule,
    DatePipe,
    TableModule,
    ButtonModule,
    AutoCompleteModule,
    InputNumberModule,
    ProgressBarModule,
    TagModule,
  ],
  templateUrl: './detalhe-nota.html',
})
export class DetalheNota implements OnInit, OnChanges, OnDestroy {
  private readonly notasApi = inject(NotasService);
  private readonly produtosApi = inject(ProdutosService);
  private readonly mensagens = inject(MessageService);
  private readonly destroyRef = inject(DestroyRef);

  /** Id da nota selecionada na lista ao lado. */
  @Input() notaId: string | null = null;

  /** Avisa o pai para atualizar a listagem (status e totais mudaram). */
  @Output() readonly notaAlterada = new EventEmitter<void>();

  protected readonly nota = signal<NotaDetalhe | null>(null);
  protected readonly carregando = signal(false);
  protected readonly imprimindo = signal(false);
  protected readonly adicionando = signal(false);

  protected produtoSelecionado: Produto | string | null = null;
  protected quantidade = 1;
  protected readonly sugestoes = signal<Produto[]>([]);

  private readonly termos = new Subject<string>();

  /**
   * ngOnInit: pipeline do autocomplete de produtos.
   *
   * Mesmo desenho da busca da tela de Produtos -- debounce para nao consultar a
   * cada tecla, e switchMap para descartar respostas de termos ja abandonados.
   */
  ngOnInit(): void {
    this.termos
      .pipe(
        debounceTime(300),
        distinctUntilChanged(),
        switchMap((termo) =>
          this.produtosApi.listar(termo, 1, 8).pipe(catchError(() => of(null))),
        ),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((pagina) => this.sugestoes.set(pagina?.itens ?? []));
  }

  /**
   * ngOnChanges: o pai troca a nota selecionada sem destruir este componente,
   * entao a mudanca do @Input e o unico sinal de que ha outra nota para
   * carregar. Sem este hook, a tela ficaria presa na primeira nota aberta.
   */
  ngOnChanges(mudancas: SimpleChanges): void {
    if (mudancas['notaId']) {
      this.produtoSelecionado = null;
      this.quantidade = 1;
      this.carregar();
    }
  }

  ngOnDestroy(): void {
    this.termos.complete();
  }

  protected buscarProdutos(evento: AutoCompleteCompleteEvent): void {
    this.termos.next(evento.query);
  }

  protected carregar(): void {
    if (!this.notaId) {
      this.nota.set(null);
      return;
    }

    this.carregando.set(true);
    this.notasApi
      .obter(this.notaId)
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.carregando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((nota) => this.nota.set(nota));
  }

  protected adicionarItem(): void {
    const atual = this.nota();
    if (!atual) {
      return;
    }

    const codigo =
      typeof this.produtoSelecionado === 'string'
        ? this.produtoSelecionado
        : this.produtoSelecionado?.codigo;

    if (!codigo) {
      this.mensagens.add({
        severity: 'warn',
        summary: 'Selecione um produto',
        detail: 'Busque pelo código ou pela descrição.',
      });
      return;
    }

    this.adicionando.set(true);
    this.notasApi
      .adicionarItem(atual.id, codigo, this.quantidade)
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.adicionando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((nota) => {
        if (!nota) {
          return;
        }
        this.nota.set(nota);
        this.produtoSelecionado = null;
        this.quantidade = 1;
        this.notaAlterada.emit();
      });
  }

  protected removerItem(itemId: string): void {
    const atual = this.nota();
    if (!atual) {
      return;
    }

    this.notasApi
      .removerItem(atual.id, itemId)
      .pipe(catchError(() => of(null)), takeUntilDestroyed(this.destroyRef))
      .subscribe((nota) => {
        if (nota) {
          this.nota.set(nota);
          this.notaAlterada.emit();
        }
      });
  }

  /**
   * Dispara a saga de impressao.
   *
   * O indicador de processamento nao e enfeite: esta chamada atravessa dois
   * microsservicos e pode repetir ate tres vezes antes de desistir.
   */
  protected imprimir(): void {
    const atual = this.nota();
    if (!atual) {
      return;
    }

    this.imprimindo.set(true);
    this.notasApi
      .imprimir(atual.id)
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.imprimindo.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((nota) => {
        if (!nota) {
          this.carregar();
          return;
        }

        this.nota.set(nota);
        this.mensagens.add({
          severity: 'success',
          summary: `${nota.numeroFormatado} impressa`,
          detail: 'Estoque baixado e nota fechada.',
        });
        this.notaAlterada.emit();
      });
  }

  protected totalUnidades(): number {
    return this.nota()?.itens.reduce((soma, item) => soma + item.quantidade, 0) ?? 0;
  }
}
