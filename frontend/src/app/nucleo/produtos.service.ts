import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

import { PaginaProdutos, Produto } from './modelos';

/** Acesso ao servico de Estoque (Go, porta 8081 via proxy). */
@Injectable({ providedIn: 'root' })
export class ProdutosService {
  private readonly http = inject(HttpClient);
  private readonly base = '/api/produtos';

  listar(busca = '', pagina = 1, tamanho = 10): Observable<PaginaProdutos> {
    let parametros = new HttpParams()
      .set('pagina', pagina)
      .set('tamanho', tamanho);

    if (busca.trim()) {
      parametros = parametros.set('busca', busca.trim());
    }

    return this.http.get<PaginaProdutos>(this.base, { params: parametros });
  }

  buscarPorCodigo(codigo: string): Observable<Produto> {
    return this.http.get<Produto>(`${this.base}/${encodeURIComponent(codigo)}`);
  }

  criar(codigo: string, descricao: string, saldo: number): Observable<Produto> {
    return this.http.post<Produto>(this.base, { codigo, descricao, saldo });
  }

  /**
   * Ajuste manual de cadastro. Grava um movimento do tipo AJUSTE no Estoque --
   * caminho deliberadamente separado da baixa por nota fiscal.
   */
  atualizar(codigo: string, descricao: string, saldo?: number): Observable<Produto> {
    const corpo: { descricao: string; saldo?: number } = { descricao };
    if (saldo !== undefined) {
      corpo.saldo = saldo;
    }
    return this.http.put<Produto>(`${this.base}/${encodeURIComponent(codigo)}`, corpo);
  }
}
