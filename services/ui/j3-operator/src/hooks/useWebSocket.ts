import { useEffect, useState, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { Transaction } from '../types';
import type { WsEvent } from '../types/ws';

/**
 * Хук-заготовка для WebSocket.
 * Сейчас работает в режиме эмуляции: генерирует события WS через интервал.
 * Когда появится реальный сервер, достаточно раскомментировать строки
 * с new WebSocket и закомментировать эмуляцию.
 */
export function useWebSocket() {
  const queryClient = useQueryClient();
  const [history, setHistory] = useState<{ time: string; discrepancies: number }[]>([]);
  const wsRef = useRef<WebSocket | null>(null);

  // Функция обновления истории расхождений для графика
  const updateHistory = (mismatchCount: number) => {
    const now = new Date();
    const timeStr = now.toLocaleTimeString('ru-RU', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    setHistory((prev) =>
      [...prev, { time: timeStr, discrepancies: mismatchCount }].slice(-60)
    );
  };

  // Обработчик входящего события (одинаков для эмуляции и реального WS)
  const handleEvent = (event: WsEvent) => {
    console.log('[WS] Получено событие:', event.type, event);

    switch (event.type) {
      case 'transaction_received':
      case 'transaction_progress':
      case 'transaction_matched':
        // Инвалидируем список транзакций, чтобы подтянуть свежие данные
        queryClient.invalidateQueries({ queryKey: ['transactions'] });
        break;

      case 'incident_created':
      case 'incident_updated':
        // Когда будет страница инцидентов, здесь можно инвалидировать ['incidents']
        console.log('[WS] Инцидент обновлён:', event.incident_id);
        break;

      case 'source_status_changed':
        // Можно обновить статус источника в кеше
        break;
    }

    // После любого события пересчитываем количество расхождений для графика
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
  };

  useEffect(() => {
    // === ЭМУЛЯЦИЯ (убрать, когда появится реальный WebSocket) ===
    const fakeEvents: WsEvent[] = [
      { type: 'transaction_matched', ts_ms: Date.now(), transaction_id: 'tx-001' },
      { type: 'transaction_progress', ts_ms: Date.now(), transaction_id: 'tx-002', sources_seen: ['merchant', 'gateway'] },
      { type: 'incident_created', ts_ms: Date.now(), transaction_id: 'tx-003', incident_id: 'inc-1', incident_type: 'fee_mismatch', severity: 1, description: 'Комиссия не совпадает' },
    ];

    const interval = setInterval(() => {
      // Берём случайное событие из списка
      const event = fakeEvents[Math.floor(Math.random() * fakeEvents.length)];
      // Обновляем timestamp и id, если нужно
      const dynamicEvent: WsEvent = {
        ...event,
        ts_ms: Date.now(),
        transaction_id: event.transaction_id || `tx-${Math.floor(Math.random() * 1000)}`,
      };
      handleEvent(dynamicEvent);
    }, 5000);

    return () => clearInterval(interval);

    // === РЕАЛЬНЫЙ WEB SOCKET (раскомментировать, когда сервер будет готов) ===
    // const wsUrl = `ws://${window.location.host}/api/v1/ws`;
    // const ws = new WebSocket(wsUrl);
    // wsRef.current = ws;
    //
    // ws.onopen = () => {
    //   console.log('[WS] Соединение установлено');
    //   // Запускаем heartbeat
    //   const pingInterval = setInterval(() => {
    //     if (ws.readyState === WebSocket.OPEN) {
    //       ws.send(JSON.stringify({ type: 'ping' }));
    //     }
    //   }, 30000);
    //
    //   ws.onclose = () => {
    //     clearInterval(pingInterval);
    //     console.log('[WS] Соединение закрыто, пробуем переподключиться...');
    //     // Реализовать exponential backoff reconnect
    //   };
    // };
    //
    // ws.onmessage = (message) => {
    //   try {
    //     const event: WsEvent = JSON.parse(message.data);
    //     if (event.type === 'pong') return;
    //     handleEvent(event);
    //   } catch (err) {
    //     console.error('[WS] Ошибка парсинга сообщения', err);
    //   }
    // };
    //
    // ws.onerror = (error) => {
    //   console.error('[WS] Ошибка соединения', error);
    // };
    //
    // return () => {
    //   ws.close();
    //   clearInterval(pingInterval);
    // };

  }, [queryClient]);

  return { history };
}