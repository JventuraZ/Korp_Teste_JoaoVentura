import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

import {
  CamposPorCst,
  ConfiguracaoFiscal,
  ContextoOperacao,
  ItemReferencia,
  Referencias,
  RegraTributaria,
  RespostaConfiguracao,
  RespostaRegras,
  ResultadoSimulacao,
  ResultadoValidacao,
} from './fiscal.modelos';

/**
 * Acesso ao microsserviço fiscal (porta 8083 via proxy).
 *
 * Protótipo: o serviço valida e devolve, mas não persiste. O estado
 * autoritativo vive na tela durante a sessão — por isso validação e simulação
 * enviam a configuração e as regras no corpo, em vez de o servidor consultá-las.
 */
@Injectable({ providedIn: 'root' })
export class FiscalService {
  private readonly http = inject(HttpClient);
  private readonly base = '/api/fiscal';

  referencias(): Observable<Referencias> {
    return this.http.get<Referencias>(`${this.base}/referencias`);
  }

  camposPorCst(): Observable<CamposPorCst> {
    return this.http.get<CamposPorCst>(`${this.base}/campos-por-cst`);
  }

  buscarNcm(busca: string): Observable<ItemReferencia[]> {
    return this.http.get<ItemReferencia[]>(`${this.base}/ncm`, {
      params: new HttpParams().set('busca', busca),
    });
  }

  buscarCfop(busca: string): Observable<ItemReferencia[]> {
    return this.http.get<ItemReferencia[]>(`${this.base}/cfop`, {
      params: new HttpParams().set('busca', busca),
    });
  }

  configuracao(codigo: string): Observable<RespostaConfiguracao> {
    return this.http.get<RespostaConfiguracao>(`${this.base}/produtos/${encodeURIComponent(codigo)}`);
  }

  salvar(codigo: string, configuracao: ConfiguracaoFiscal): Observable<RespostaConfiguracao> {
    return this.http.put<RespostaConfiguracao>(
      `${this.base}/produtos/${encodeURIComponent(codigo)}`, configuracao);
  }

  regras(codigo: string): Observable<RespostaRegras> {
    return this.http.get<RespostaRegras>(`${this.base}/produtos/${encodeURIComponent(codigo)}/regras`);
  }

  validar(codigo: string, configuracao: ConfiguracaoFiscal, regras: RegraTributaria[]): Observable<ResultadoValidacao> {
    return this.http.post<ResultadoValidacao>(
      `${this.base}/produtos/${encodeURIComponent(codigo)}/validacao`, { configuracao, regras });
  }

  simular(
    produto: string, contexto: ContextoOperacao, valor: number, quantidade: number,
    regras: RegraTributaria[],
  ): Observable<ResultadoSimulacao> {
    return this.http.post<ResultadoSimulacao>(`${this.base}/simulacao`, {
      pedido: { produto, contexto, valor, quantidade },
      regras,
    });
  }
}
