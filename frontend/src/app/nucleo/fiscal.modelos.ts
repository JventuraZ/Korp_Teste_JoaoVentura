export interface ItemReferencia {
  codigo: string;
  descricao: string;
}

export interface Referencias {
  origensMercadoria: ItemReferencia[];
  cstIcms: ItemReferencia[];
  csosn: ItemReferencia[];
  cstPisCofins: ItemReferencia[];
  cstIpi: ItemReferencia[];
  tiposOperacao: ItemReferencia[];
  finalidades: ItemReferencia[];
  regimesTributarios: ItemReferencia[];
  tiposCliente: ItemReferencia[];
  ufs: string[];
}

/**
 * Mapa de quais campos fazem sentido para cada CST/CSOSN.
 *
 * Vem do servidor de propósito: mudar a regra de exibição não pode exigir
 * recompilar o frontend, pela mesma razão que as regras tributárias não ficam
 * no código.
 */
export interface CamposPorCst {
  icms: Record<string, string[]>;
  csosn: Record<string, string[]>;
  rotulos: Record<string, string>;
}

export interface PisCofins {
  cstPis: string;
  aliquotaPis: number | null;
  baseCalculoPis: string;
  cstCofins: string;
  aliquotaCofins: number | null;
  baseCalculoCofins: string;
}

export interface Ipi {
  cstIpi: string;
  enquadramentoLegal: string;
  codigoEnquadramento: string;
  aliquotaIpi: number | null;
  unidadeTributavel: string;
  quantidadeTributavel: number | null;
  valorPorUnidade: number | null;
}

/**
 * Repare no que NÃO está aqui: CFOP e CST de ICMS. Eles dependem da operação
 * e por isso vivem nas regras — fixá-los no produto seria o erro que o
 * requisito de CFOPs múltiplos pede para evitar.
 */
export interface ConfiguracaoFiscal {
  codigo: string;
  ncm: string;
  cest: string;
  codigoBeneficioFiscal: string;
  origemMercadoria: string;
  exTipi: string;
  unidadeTributavel: string;
  quantidadeTributavel: number | null;
  codigoAnp: string;
  producaoPropria: boolean;
  pisCofins: PisCofins;
  ipi: Ipi;
}

export type SituacaoFiscal = 'CONFIGURADO' | 'INCOMPLETO' | 'ERRO' | 'NAO_APLICAVEL';

export interface ResumoFiscal {
  situacao: SituacaoFiscal;
  areas: Record<string, SituacaoFiscal>;
  totalRegras: number;
  regrasAtivas: number;
}

export interface RespostaConfiguracao {
  configuracao: ConfiguracaoFiscal;
  resumo: ResumoFiscal;
  aviso: string;
}

/** Campo nulo = curinga: a condição vale para qualquer valor. */
export interface CondicoesRegra {
  operacao: string | null;
  ufOrigem: string | null;
  ufDestino: string | null;
  tipoCliente: string | null;
  consumidorFinal: boolean | null;
  contribuinteIcms: boolean | null;
  regimeEmpresa: string | null;
  finalidade: string | null;
}

export interface ResultadoRegra {
  cfop: string;
  cstIcms: string;
  csosn: string;
  aliquotaIcms: number | null;
  reducaoBaseIcms: number | null;
  aliquotaFcp: number | null;
  aliquotaIcmsSt: number | null;
  mva: number | null;
  aliquotaInterna: number | null;
  aliquotaInterestadual: number | null;
  cstPis: string;
  aliquotaPis: number | null;
  cstCofins: string;
  aliquotaCofins: number | null;
  cstIpi: string;
  aliquotaIpi: number | null;
  observacao: string;
}

export interface RegraTributaria {
  id: string;
  descricao: string;
  prioridade: number;
  ativa: boolean;
  condicoes: CondicoesRegra;
  resultado: ResultadoRegra;
}

export type SeveridadeConflito = 'AMBIGUA' | 'SOBREPOSTA';

export interface Conflito {
  severidade: SeveridadeConflito;
  regraA: string;
  regraB: string;
  explicacao: string;
}

export interface RespostaRegras {
  itens: RegraTributaria[];
  conflitos: Conflito[];
  aviso: string;
}

export interface ProblemaFiscal {
  gravidade: 'ERRO' | 'AVISO';
  area: string;
  mensagem: string;
  comoResolver: string;
}

export interface ResultadoValidacao {
  valida: boolean;
  problemas: ProblemaFiscal[];
}

export interface ContextoOperacao {
  operacao: string;
  ufOrigem: string;
  ufDestino: string;
  tipoCliente: string;
  consumidorFinal: boolean;
  contribuinteIcms: boolean;
  regimeEmpresa: string;
  finalidade: string;
}

export interface TributoSimulado {
  nome: string;
  situacao: string;
  baseCalculo: number | null;
  aliquota: number | null;
  valor: number | null;
  observacao: string;
}

export interface Candidata {
  regra: RegraTributaria;
  especificidade: number;
  condicoesCasadas: string[];
  escolhida: boolean;
}

export interface ResultadoSimulacao {
  encontrou: boolean;
  aviso: string;
  cfop: string;
  cstOuCsosn: string;
  valorOperacao: number;
  tributos: TributoSimulado[];
  totalTributos: number;
  /** O "Como chegamos a este cálculo?" — vem do motor, não é montado na tela. */
  trilha: string[];
  candidatas: Candidata[];
}

export function condicoesVazias(): CondicoesRegra {
  return {
    operacao: null, ufOrigem: null, ufDestino: null, tipoCliente: null,
    consumidorFinal: null, contribuinteIcms: null, regimeEmpresa: null, finalidade: null,
  };
}

export function resultadoVazio(): ResultadoRegra {
  return {
    cfop: '', cstIcms: '', csosn: '', aliquotaIcms: null, reducaoBaseIcms: null,
    aliquotaFcp: null, aliquotaIcmsSt: null, mva: null, aliquotaInterna: null,
    aliquotaInterestadual: null, cstPis: '', aliquotaPis: null, cstCofins: '',
    aliquotaCofins: null, cstIpi: '', aliquotaIpi: null, observacao: '',
  };
}
