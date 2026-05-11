import { useQuery } from '@tanstack/react-query';
import { mockTransactions } from '../lib/mock-data';

export function useTransactions() {
  return useQuery({
    queryKey: ['transactions'],
    queryFn: () => {
      // Имитация задержки сети
      return new Promise<typeof mockTransactions>((resolve) =>
        setTimeout(() => resolve(mockTransactions), 800)
      );
    },
    staleTime: 30_000,
  });
}