/*import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { Transaction } from '../types';

export function useRealtimeUpdates() {
  const queryClient = useQueryClient();
  const [history, setHistory] = useState<{ time: string; discrepancies: number }[]>([]);

  useEffect(() => {
    // Сразу добавим первую точку на основе текущих данных
    const initialTransactions = queryClient.getQueryData<Transaction[]>(['transactions']);
    if (initialTransactions && initialTransactions.length > 0) {
      const mismatchCount = initialTransactions.filter(
        (t) =>
          t.sourceA.status === 'mismatch' ||
          t.sourceB.status === 'mismatch' ||
          t.sourceC.status === 'mismatch'
      ).length;
      const now = new Date();
      const timeStr = now.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      setHistory([{ time: timeStr, discrepancies: mismatchCount }]);
    }

    const interval = setInterval(() => {
      const transactions = queryClient.getQueryData<Transaction[]>(['transactions']);
      if (!transactions || transactions.length === 0) return;

      const updated = [...transactions];
      const randomIndex = Math.floor(Math.random() * updated.length);
      const tx = { ...updated[randomIndex] };

      const sources = ['sourceA', 'sourceB', 'sourceC'] as const;
      const randomSource = sources[Math.floor(Math.random() * sources.length)];
      const statuses: Array<'match' | 'mismatch' | 'pending' | 'offline'> = [
        'match', 'mismatch', 'pending', 'offline'
      ];
      const newStatus = statuses[Math.floor(Math.random() * statuses.length)];

      tx[randomSource] = {
        ...tx[randomSource],
        status: newStatus,
        amount: Math.random() > 0.7 ? tx[randomSource].amount + (Math.random() - 0.5) * 10 : tx[randomSource].amount,
        fee: Math.random() > 0.7 ? tx[randomSource].fee + (Math.random() - 0.5) * 2 : tx[randomSource].fee,
      };
      updated[randomIndex] = tx;

      queryClient.setQueryData(['transactions'], updated);

      const mismatchCount = updated.filter(
        (t) =>
          t.sourceA.status === 'mismatch' ||
          t.sourceB.status === 'mismatch' ||
          t.sourceC.status === 'mismatch'
      ).length;

      const now = new Date();
      const timeStr = now.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', second: '2-digit' });

      setHistory((prev) => [
        ...prev,
        { time: timeStr, discrepancies: mismatchCount },
      ].slice(-60));
    }, 4000 + Math.random() * 2000);

    return () => clearInterval(interval);
  }, [queryClient]);

  return { history };
}

