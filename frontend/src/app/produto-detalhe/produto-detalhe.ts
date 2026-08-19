import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, RouterLink } from '@angular/router';
import { MessageService } from 'primeng/api';
import { ButtonModule } from 'primeng/button';
import { InputNumberModule } from 'primeng/inputnumber';
import { InputTextModule } from 'primeng/inputtext';
import { TabsModule } from 'primeng/tabs';
import { TagModule } from 'primeng/tag';
import { catchError, finalize, of, switchMap } from 'rxjs';

import { AbaFiscal } from '../fiscal/aba-fiscal';
import { Produto } from '../nucleo/modelos';
import { ProdutosService } from '../nucleo/produtos.service';

/**
 * Página de detalhe do produto.
 *
 * O cadastro era um diálogo, adequado para três campos. A dimensão fiscal tem
 * dezenas, distribuídos em seções — cabe numa página com abas, que é também o
 * padrão que o usuário de ERP espera.
 */
@Component({
  selector: 'app-produto-detalhe',
  imports: [
    FormsModule, RouterLink, ButtonModule, InputNumberModule, InputTextModule,
    TabsModule, TagModule, AbaFiscal,
  ],
  templateUrl: './produto-detalhe.html',
})
export class ProdutoDetalhe implements OnInit {
  private readonly rota = inject(ActivatedRoute);
  private readonly api = inject(ProdutosService);
  private readonly mensagens = inject(MessageService);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly produto = signal<Produto | null>(null);
  protected readonly codigo = signal('');
  protected readonly carregando = signal(true);
  protected readonly salvando = signal(false);

  protected formulario = { descricao: '', saldo: 0 };

  ngOnInit(): void {
    this.rota.paramMap
      .pipe(
        switchMap((parametros) => {
          const codigo = parametros.get('codigo') ?? '';
          this.codigo.set(codigo);
          this.carregando.set(true);
          return this.api.buscarPorCodigo(codigo).pipe(catchError(() => of(null)));
        }),
        finalize(() => this.carregando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((produto) => {
        this.carregando.set(false);
        this.produto.set(produto);
        if (produto) {
          this.formulario = { descricao: produto.descricao, saldo: produto.saldo };
        }
      });
  }

  protected salvarDadosGerais(): void {
    const atual = this.produto();
    if (!atual) return;

    this.salvando.set(true);
    this.api
      .atualizar(atual.codigo, this.formulario.descricao, this.formulario.saldo)
      .pipe(
        catchError(() => of(null)),
        finalize(() => this.salvando.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe((produto) => {
        if (!produto) return;
        this.produto.set(produto);
        this.mensagens.add({
          severity: 'success',
          summary: 'Produto atualizado',
          detail: `${produto.codigo} — saldo ${produto.saldo}`,
        });
      });
  }
}
