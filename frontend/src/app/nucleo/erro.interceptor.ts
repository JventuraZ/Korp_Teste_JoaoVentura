import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { MessageService } from 'primeng/api';
import { catchError, throwError } from 'rxjs';

import { Problema, tipoDoErro } from './modelos';

/**
 * Interceptor UNICO de erro para os dois microsservicos.
 *
 * Isto so e possivel porque Go e C# respondem no mesmo formato RFC 7807. O
 * interceptor le o campo `type` -- nao o status -- porque e o tipo que carrega
 * o significado: dois erros podem compartilhar o 409 e pedir acoes opostas do
 * usuario.
 */
export const interceptorDeErro: HttpInterceptorFn = (requisicao, proximo) => {
  const mensagens = inject(MessageService);

  return proximo(requisicao).pipe(
    catchError((resposta: HttpErrorResponse) => {
      const problema = extrairProblema(resposta);
      const { severidade, titulo, detalhe } = traduzir(problema);

      mensagens.add({
        severity: severidade,
        summary: titulo,
        detail: detalhe,
        life: severidade === 'error' ? 8000 : 6000,
      });

      return throwError(() => problema);
    }),
  );
};

function extrairProblema(resposta: HttpErrorResponse): Problema {
  if (resposta.status === 0) {
    return {
      type: 'https://korp.local/erros/sem-conexao',
      title: 'Sem conexão',
      status: 0,
      detail: 'Não foi possível falar com o servidor.',
    };
  }

  const corpo = resposta.error;
  if (corpo && typeof corpo === 'object' && 'type' in corpo) {
    return corpo as Problema;
  }

  return {
    type: 'https://korp.local/erros/erro-desconhecido',
    title: 'Erro inesperado',
    status: resposta.status,
    detail: resposta.message,
  };
}

type Severidade = 'error' | 'warn' | 'info';

function traduzir(problema: Problema): {
  severidade: Severidade;
  titulo: string;
  detalhe: string;
} {
  switch (tipoDoErro(problema)) {
    case 'saldo-insuficiente': {
      const faltantes = (problema.itensInsuficientes ?? [])
        .map((item) => `${item.codigo}: pedido ${item.quantidadeSolicitada}, disponível ${item.saldoDisponivel}`)
        .join(' · ');

      return {
        severidade: 'warn',
        titulo: 'Estoque insuficiente',
        detalhe: faltantes
          ? `Ajuste as quantidades — ${faltantes}`
          : problema.detail,
      };
    }

    case 'nota-nao-aberta':
    case 'nota-em-alteracao':
      return {
        severidade: 'warn',
        titulo: 'Nota já processada',
        detalhe: 'Esta nota mudou de estado. Recarregue a tela para ver a versão atual.',
      };

    case 'estoque-indisponivel':
    case 'banco-indisponivel':
    case 'sem-conexao':
      return {
        severidade: 'error',
        titulo: 'Serviço indisponível',
        detalhe: 'O serviço não respondeu. Nada foi perdido — tente novamente em instantes.',
      };

    case 'falha-ao-fechar-nota':
      return {
        severidade: 'error',
        titulo: 'Impressão desfeita',
        detalhe: 'A baixa foi estornada e a nota continua aberta. Tente imprimir novamente.',
      };

    case 'codigo-duplicado':
      return {
        severidade: 'warn',
        titulo: 'Código já cadastrado',
        detalhe: 'Já existe um produto com este código.',
      };

    default:
      return {
        severidade: problema.status >= 500 ? 'error' : 'warn',
        titulo: problema.title || 'Erro',
        detalhe: problema.detail,
      };
  }
}
