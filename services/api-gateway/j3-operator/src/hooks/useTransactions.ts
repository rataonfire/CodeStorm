import { useQuery } from '@tanstack/react-query';
import type {
  ApiTransactionListResponse,
  ApiTransactionDetailsResponse,
  Transaction,
} from '../types';
import { buildTransactionFromDetails } from '../types';

async function fetchTransactions(): Promise<Transaction[]> {
  // 1. Получаем список транзакций
  const listRes = await fetch('/api/v1/transactions?limit=20');
  if (!listRes.ok) {
    throw new Error(`Ошибка HTTP: ${listRes.status}`);
  }
  const listData: ApiTransactionListResponse = await listRes.json();
  if (!listData.items || listData.items.length === 0) {
    return [];
  }

  // 2. Для каждой транзакции загружаем детали параллельно
  const detailsPromises = listData.items.map(async (item) => {
    const detailRes = await fetch(`/api/v1/transactions/${item.transaction_id}`);
    if (!detailRes.ok) {
      console.warn(`Не удалось загрузить детали для ${item.transaction_id}`);
      return null;
    }
    const detailData: ApiTransactionDetailsResponse = await detailRes.json();
    return buildTransactionFromDetails(detailData);
  });

  const transactions = (await Promise.all(detailsPromises)).filter(Boolean) as Transaction[];
  return transactions;
}

export function useTransactions() {
  return useQuery({
    queryKey: ['transactions'],
    queryFn: fetchTransactions,
    staleTime: 30_000,
    refetchInterval: 10_000, // можно опрашивать раз в 10 секунд для live-эффекта
  });
}