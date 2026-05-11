import type { Transaction } from '../types';

export const mockTransactions: Transaction[] = [
  {
    id: 'tx-001',
    sourceA: { amount: 100.0, fee: 1.5, currency: 'USD', status: 'match' },
    sourceB: { amount: 100.0, fee: 1.5, currency: 'USD', status: 'match' },
    sourceC: { amount: 100.0, fee: 1.6, currency: 'USD', status: 'mismatch' },
  },
  {
    id: 'tx-002',
    sourceA: { amount: 250.0, fee: 3.0, currency: 'EUR', status: 'pending' },
    sourceB: { amount: 250.0, fee: 3.0, currency: 'EUR', status: 'pending' },
    sourceC: { amount: 0, fee: 0, currency: 'EUR', status: 'offline' },
  },
  {
    id: 'tx-003',
    sourceA: { amount: 50.0, fee: 0.5, currency: 'GBP', status: 'match' },
    sourceB: { amount: 50.0, fee: 0.5, currency: 'GBP', status: 'match' },
    sourceC: { amount: 50.0, fee: 0.5, currency: 'GBP', status: 'match' },
  },
];