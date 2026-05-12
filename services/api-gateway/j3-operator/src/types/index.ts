export type ReconciliationStatus = 'match' | 'mismatch' | 'pending' | 'offline';

export interface SourceData {
  amount: number;
  fee: number;
  currency: string;
  status: ReconciliationStatus;
}

export interface Transaction {
  id: string;
  sourceA: SourceData;
  sourceB: SourceData;
  sourceC: SourceData;
  overallStatus: string; // 'matched' | 'mismatch' | ...
  merchantId?: string;
  txType?: string;
  createdAt?: string;
}

// Типы ответов от API
export interface ApiTransactionSummary {
  transaction_id: string;
  overall_status: string;
  created_at: string;
  updated_at: string;
  merchant_id: string;
  tx_type: string;
}

export interface ApiTransactionListResponse {
  items: ApiTransactionSummary[];
  next_cursor: string | null;
  total_estimate: number;
}

export interface ApiTransactionDetail {
  source: 'merchant' | 'gateway' | 'bank';
  amount_expected: number;
  amount_actual: number;
  fee_expected: number;
  fee_actual: number;
  is_matched: boolean;
}

export interface ApiTransactionDetailsResponse {
  summary: ApiTransactionSummary;
  details: ApiTransactionDetail[];
}

// Функция преобразования деталей в источники
function mapDetailsToSources(
  details: ApiTransactionDetail[],
  currency: string
): Pick<Transaction, 'sourceA' | 'sourceB' | 'sourceC'> {
  const getSource = (sourceName: 'merchant' | 'gateway' | 'bank'): SourceData => {
    const detail = details.find((d) => d.source === sourceName);
    if (!detail) {
      return {
        amount: 0,
        fee: 0,
        currency,
        status: 'pending', // источник не прислал данные
      };
    }
    // Статус определяем по is_matched и наличию данных
    let status: ReconciliationStatus = 'match';
    if (!detail.is_matched) {
      status = 'mismatch';
    }
    // можно добавить доп. логику, если нужно отличать offline (сейчас нет)
    return {
      amount: detail.amount_actual,
      fee: detail.fee_actual,
      currency,
      status,
    };
  };

  return {
    sourceA: getSource('merchant'),
    sourceB: getSource('gateway'),
    sourceC: getSource('bank'),
  };
}

// Экспортная функция сборки Transaction из ответа деталей
export function buildTransactionFromDetails(
  detailsResponse: ApiTransactionDetailsResponse
): Transaction {
  const { summary, details } = detailsResponse;
  const sources = mapDetailsToSources(details, 'UZS'); // валюта пока захардкожена, позже можно брать из summary
  return {
    id: summary.transaction_id,
    ...sources,
    overallStatus: summary.overall_status,
    merchantId: summary.merchant_id,
    txType: summary.tx_type,
    createdAt: summary.created_at,
  };
}