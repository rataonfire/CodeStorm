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
}