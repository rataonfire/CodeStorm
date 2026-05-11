// Типы событий, приходящих от API Gateway через WebSocket
// Основаны на разделе 3.4 документа STANDARDS.md

export type WsEventType =
  | 'transaction_received'
  | 'transaction_progress'
  | 'transaction_matched'
  | 'incident_created'
  | 'incident_updated'
  | 'source_status_changed';

export interface WsEvent {
  type: WsEventType;
  ts_ms: number;
  transaction_id?: string;
  incident_id?: string;
  source?: string;
  sources_seen?: string[];
  incident_type?: string;
  severity?: number;
  description?: string;
  new_status?: string;
  new_severity?: number;
  is_online?: boolean;
}