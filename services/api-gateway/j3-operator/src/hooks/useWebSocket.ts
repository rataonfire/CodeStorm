import { useEffect, useState, useRef, useCallback } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { Transaction } from '../types';
import type { WsEvent } from '../types/ws';

const WS_URL = `ws://${window.location.host}/api/v1/ws`;
const HEARTBEAT_INTERVAL = 30_000;
const RECONNECT_BASE_DELAY = 1000;
const RECONNECT_MAX_DELAY = 30_000;
const RECONNECT_MULTIPLIER = 2;

export function useWebSocket() {
  const queryClient = useQueryClient();
  const [history, setHistory] = useState<{ time: string; discrepancies: number }[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttemptRef = useRef(0);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const updateHistory = useCallback((mismatchCount: number) => {
    const now = new Date();
    const timeStr = now.toLocaleTimeString('ru-RU', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    setHistory((prev) =>
      [...prev, { time: timeStr, discrepancies: mismatchCount }].slice(-60)
    );
  }, []);

  const handleEvent = useCallback(
    (event: WsEvent) => {
      console.log('[WS] Событие:', event.type, event);

      switch (event.type) {
        case 'transaction_received':
        case 'transaction_progress':
        case 'transaction_matched':
          queryClient.invalidateQueries({ queryKey: ['transactions'] });
          break;
        case 'incident_created':
        case 'incident_updated':
          console.log('[WS] Инцидент:', event.incident_id);
          break;
        case 'source_status_changed':
          console.log('[WS] Статус источника:', event.source, event.is_online);
          break;
      }

      // Обновляем график после любого события
      const transactions = queryClient.getQueryData<Transaction[]>(['transactions']);
      if (transactions) {
        const mismatchCount = transactions.filter(
          (t) =>
            t.sourceA.status === 'mismatch' ||
            t.sourceB.status === 'mismatch' ||
            t.sourceC.status === 'mismatch'
        ).length;
        updateHistory(mismatchCount);
      }
    },
    [queryClient, updateHistory]
  );

  useEffect(() => {
    let pingInterval: ReturnType<typeof setInterval> | null = null;

    const connect = () => {
      const ws = new WebSocket(WS_URL);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log('[WS] Соединение установлено');
        reconnectAttemptRef.current = 0;

        // Запускаем heartbeat
        pingInterval = setInterval(() => {
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'ping' }));
          }
        }, HEARTBEAT_INTERVAL);
      };

      ws.onmessage = (message) => {
        try {
          const event: WsEvent = JSON.parse(message.data);
          if (event.type === 'pong') return;
          handleEvent(event);
        } catch (err) {
          console.error('[WS] Ошибка парсинга сообщения', err);
        }
      };

      ws.onerror = (error) => {
        console.error('[WS] Ошибка соединения', error);
      };

      ws.onclose = () => {
        if (pingInterval) clearInterval(pingInterval);
        console.log('[WS] Соединение закрыто');

        // Exponential backoff reconnect
        const attempt = reconnectAttemptRef.current + 1;
        reconnectAttemptRef.current = attempt;
        const delay = Math.min(
          RECONNECT_BASE_DELAY * Math.pow(RECONNECT_MULTIPLIER, attempt - 1),
          RECONNECT_MAX_DELAY
        );
        console.log(`[WS] Переподключение через ${delay} мс (попытка ${attempt})`);
        reconnectTimerRef.current = setTimeout(() => {
          connect();
        }, delay);
      };
    };

    connect();

    return () => {
      wsRef.current?.close();
      if (reconnectTimerRef.current) clearTimeout(reconnectTimerRef.current);
      if (pingInterval) clearInterval(pingInterval);
    };
  }, [queryClient, handleEvent]);

  return { history };
}