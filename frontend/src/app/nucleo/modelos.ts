/**
 * Tipos das duas APIs.
 *
 * Escritos a partir dos contratos HTTP (docs/contrato-estoque.md e o README),
 * nao gerados do codigo dos servicos: o frontend depende do contrato, nao da
 * implementacao de quem responde.
 */

export interface Produto {
  id: string;
  codigo: string;
  descricao: string;
  saldo: number;
  versao: number;
  criadoEm: string;
  atualizadoEm: string;
}

export interface PaginaProdutos {
  itens: Produto[];
  pagina: number;
  tamanho: number;
  total: number;
}

export type StatusNota = 'Aberta' | 'Fechada';

export interface ItemNota {
  id: string;
  codigo: string;
  /** Retrato da descricao no momento da inclusao, gravado pelo Faturamento. */
  descricao: string;
  quantidade: number;
}

export interface NotaResumo {
  id: string;
  numero: number;
  numeroFormatado: string;
  status: StatusNota;
  criadaEm: string;
  fechadaEm: string | null;
  totalItens: number;
  totalUnidades: number;
}

export interface NotaDetalhe extends Omit<NotaResumo, 'totalItens' | 'totalUnidades'> {
  podeImprimir: boolean;
  itens: ItemNota[];
}

export interface PaginaNotas {
  itens: NotaResumo[];
  pagina: number;
  tamanho: number;
  total: number;
}

export interface ItemInsuficiente {
  codigo: string;
  quantidadeSolicitada: number;
  saldoDisponivel: number;
}

export type Risco = 'CRITICO' | 'ATENCAO' | 'OK' | 'SEM_DADOS';

export interface PrevisaoProduto {
  codigo: string;
  descricao: string;
  saldo: number;
  /** Unidades por dia, estimadas por média móvel exponencial. */
  consumoDiario: number;
  /** Nulo quando o produto não teve consumo na janela: o saldo dura indefinidamente. */
  diasAteRuptura: number | null;
  dataRupturaEstimada: string | null;
  risco: Risco;
  /** Dias de histórico considerados — indica o quanto confiar na projeção. */
  amostras: number;
}

export interface PainelPrevisao {
  itens: PrevisaoProduto[];
  janelaDias: number;
  geradoEm: string;
  resumo: Partial<Record<Risco, number>>;
}

export interface AnomaliaDetectada {
  codigo: string;
  descricao: string;
  quantidade: number;
  medianaDoProduto: number;
  escore: number;
  referencia: string;
  momento: string;
}

export interface PainelAnomalias {
  itens: AnomaliaDetectada[];
  janelaDias: number;
  geradoEm: string;
}

/**
 * RFC 7807. Os dois microsservicos respondem erro exatamente nesta forma, o que
 * permite um unico interceptor tratar Go e C# sem distincao.
 */
export interface Problema {
  type: string;
  title: string;
  status: number;
  detail: string;
  instance?: string;
  itensInsuficientes?: ItemInsuficiente[];
}

/** Ultimo segmento do campo `type`, que e o identificador estavel do erro. */
export function tipoDoErro(problema: Problema): string {
  return problema.type?.split('/').pop() ?? '';
}
