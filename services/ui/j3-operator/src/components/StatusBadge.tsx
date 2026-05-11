import { CheckCircle, XCircle, Circle, MinusCircle } from 'lucide-react';
import type { ReconciliationStatus } from '../types';

const statusConfig: Record<ReconciliationStatus, {
  icon: React.ElementType;
  color: string;
  label: string;
}> = {
  match:    { icon: CheckCircle, color: 'text-green-600', label: '✓ Совпало' },
  mismatch: { icon: XCircle,     color: 'text-red-600',   label: '✗ Расхождение' },
  pending:  { icon: Circle,      color: 'text-gray-500',  label: '○ Ожидаем' },
  offline:  { icon: MinusCircle, color: 'text-orange-500',label: '− Таймаут' },
};

export function StatusBadge({ status }: { status: ReconciliationStatus }) {
  const { icon: Icon, color, label } = statusConfig[status];
  return (
    <span className={`inline-flex items-center gap-1 text-sm font-medium ${color}`}>
      <Icon size={16} />
      {label}
    </span>
  );
}