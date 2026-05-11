import { useState } from 'react';
import { useTransactions } from '../hooks/useTransactions';
import { StatusBadge } from './StatusBadge';
import { IncidentModal } from './IncidentModal';
import type { SourceData, Transaction } from '../types';

function SourceCell({ source }: { source: SourceData }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-sm font-medium text-dark-800">
        {source.amount.toFixed(2)} {source.currency}
      </span>
      <span className="text-xs text-primary-600">
        Комиссия: {source.fee.toFixed(2)}
      </span>
      <StatusBadge status={source.status} />
    </div>
  );
}

export function TransactionTable() {
  const { data: transactions, isLoading, isError, error } = useTransactions();
  const [selectedTransaction, setSelectedTransaction] = useState<Transaction | null>(null);

  if (isLoading) {
    return <div className="p-8 text-center text-dark-700">Загрузка транзакций...</div>;
  }

  if (isError) {
    return (
      <div className="p-8 text-center text-red-400">
        Ошибка загрузки: {error instanceof Error ? error.message : 'Неизвестная ошибка'}
      </div>
    );
  }

  if (!transactions || transactions.length === 0) {
    return <div className="p-8 text-center text-dark-700">Нет транзакций</div>;
  }

  return (
    <>
      <div className="overflow-x-auto shadow-2xl rounded-2xl border border-dark-700/10">
        <table className="min-w-full divide-y divide-primary-100">
          <thead className="bg-dark-800/80 backdrop-blur-sm">
            <tr>
              <th className="px-4 py-3 text-left text-xs font-semibold text-primary-100 uppercase tracking-wider">
                ID транзакции
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold text-primary-100 uppercase tracking-wider">
                Источник A
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold text-primary-100 uppercase tracking-wider">
                Источник B
              </th>
              <th className="px-4 py-3 text-left text-xs font-semibold text-primary-100 uppercase tracking-wider">
                Источник C
              </th>
            </tr>
          </thead>
          <tbody className="bg-white/80 backdrop-blur-sm divide-y divide-primary-100">
            {transactions.map((tx) => {
              const hasMismatch =
                tx.sourceA.status === 'mismatch' ||
                tx.sourceB.status === 'mismatch' ||
                tx.sourceC.status === 'mismatch';
              const hasPending =
                tx.sourceA.status === 'pending' ||
                tx.sourceB.status === 'pending' ||
                tx.sourceC.status === 'pending';
              const rowBg = hasMismatch
                ? 'bg-red-50/90 hover:bg-red-100/80'
                : hasPending
                ? 'bg-amber-50/90 hover:bg-amber-100/80'
                : 'bg-white/70 hover:bg-primary-50/50';

              return (
                <tr
                  key={tx.id}
                  className={`${rowBg} transition-all duration-200 cursor-pointer`}
                  onClick={() => setSelectedTransaction(tx)}
                >
                  <td className="px-4 py-3 text-sm font-mono text-dark-900">
                    {tx.id}
                  </td>
                  <td className="px-4 py-3">
                    <SourceCell source={tx.sourceA} />
                  </td>
                  <td className="px-4 py-3">
                    <SourceCell source={tx.sourceB} />
                  </td>
                  <td className="px-4 py-3">
                    <SourceCell source={tx.sourceC} />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {selectedTransaction && (
        <IncidentModal
          transaction={selectedTransaction}
          onClose={() => setSelectedTransaction(null)}
        />
      )}
    </>
  );
}