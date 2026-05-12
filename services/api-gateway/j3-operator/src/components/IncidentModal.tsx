import { X } from 'lucide-react';
import type { Transaction, SourceData, ReconciliationStatus } from '../types';

interface IncidentModalProps {
  transaction: Transaction;
  onClose: () => void;
}

function isEqual(a: number, b: number): boolean {
  return Math.abs(a - b) < 0.001;
}

function FieldValue({
  label,
  value,
  mismatch,
}: {
  label: string;
  value: string;
  mismatch: boolean;
}) {
  return (
    <div className={`flex justify-between py-1 ${mismatch ? 'bg-red-50 -mx-1 px-1 rounded' : ''}`}>
      <span className="text-xs text-gray-500">{label}</span>
      <span className={`text-sm font-medium ${mismatch ? 'text-red-700' : 'text-gray-900'}`}>
        {value}
      </span>
    </div>
  );
}

function SourceColumn({
  source,
  allSources,
}: {
  source: SourceData & { name: string };
  allSources: SourceData[];
}) {
  const amountMismatch = allSources.some((s) => !isEqual(s.amount, source.amount));
  const feeMismatch = allSources.some((s) => !isEqual(s.fee, source.fee));
  const statusColors: Record<ReconciliationStatus, string> = {
    match: 'text-green-600',
    mismatch: 'text-red-600',
    pending: 'text-gray-500',
    offline: 'text-orange-500',
  };

  return (
    <div className="flex flex-col gap-2 p-5 bg-white/80 backdrop-blur-sm rounded-2xl border border-primary-100 shadow-sm">
      <h3 className="font-semibold text-dark-800 mb-2">{source.name}</h3>
      <FieldValue
        label="Сумма"
        value={`${source.amount.toFixed(2)} ${source.currency}`}
        mismatch={amountMismatch}
      />
      <FieldValue
        label="Комиссия"
        value={`${source.fee.toFixed(2)} ${source.currency}`}
        mismatch={feeMismatch}
      />
      <div className="flex justify-between py-1">
        <span className="text-xs text-gray-500">Статус</span>
        <span className={`text-sm font-medium ${statusColors[source.status]}`}>
          {source.status === 'match' && '✓ Совпало'}
          {source.status === 'mismatch' && '✗ Расхождение'}
          {source.status === 'pending' && '○ Ожидаем'}
          {source.status === 'offline' && '− Офлайн'}
        </span>
      </div>
      {amountMismatch && (
        <div className="text-xs text-red-500 mt-1">
          Разница в сумме: {allSources.filter(s => !isEqual(s.amount, source.amount)).map(s => `${s.amount}`).join(', ')} ≠ {source.amount}
        </div>
      )}
      {feeMismatch && (
        <div className="text-xs text-red-500 mt-1">Разница в комиссии</div>
      )}
    </div>
  );
}

export function IncidentModal({ transaction, onClose }: IncidentModalProps) {
  const sources = [
    { ...transaction.sourceA, name: 'Источник A' },
    { ...transaction.sourceB, name: 'Источник B' },
    { ...transaction.sourceC, name: 'Источник C' },
  ];
  const allSources = [transaction.sourceA, transaction.sourceB, transaction.sourceC];

  const handleAck = () => {
    console.log('Ack инцидента', transaction.id);
    // TODO: POST /api/v1/incidents/{id}/ack
  };

  const handleResolve = () => {
    console.log('Resolve инцидента', transaction.id);
    // TODO: POST /api/v1/incidents/{id}/resolve
  };

  const handleAutoCorrect = () => {
    console.log('Автокорректировка', transaction.id);
    // TODO: POST /api/v1/incidents/{id}/auto-correct
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-dark-900/60 backdrop-blur-sm">
      <div className="bg-gradient-to-b from-white to-primary-50 rounded-3xl shadow-2xl w-full max-w-4xl max-h-[90vh] overflow-auto p-6 relative border border-primary-100">
        <button
          onClick={onClose}
          className="absolute top-3 right-3 p-1 rounded-full hover:bg-primary-100 text-dark-700"
        >
          <X size={20} />
        </button>

        <h2 className="text-2xl font-bold text-dark-800 mb-4">
          Инцидент по транзакции <span className="font-mono text-primary-600">{transaction.id}</span>
        </h2>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
          {sources.map((src) => (
            <SourceColumn key={src.name} source={src} allSources={allSources} />
          ))}
        </div>

        <div className="flex flex-wrap justify-end gap-3">
          <button
            onClick={handleAck}
            className="px-5 py-2.5 bg-primary-500 text-white rounded-xl hover:bg-primary-600 transition-colors shadow-lg shadow-primary-500/30"
          >
            Ack (Подтвердить)
          </button>
          <button
            onClick={handleResolve}
            className="px-5 py-2.5 bg-green-600 text-white rounded-xl hover:bg-green-700 transition-colors shadow-lg shadow-green-600/30"
          >
            Resolve (Закрыть)
          </button>
          <button
            onClick={handleAutoCorrect}
            className="px-5 py-2.5 bg-blue-600 text-white rounded-xl hover:bg-blue-700 transition-colors shadow-lg shadow-blue-600/30"
          >
            Auto‑Correct (Автокор.)
          </button>
        </div>
      </div>
    </div>
  );
}