import { useEffect, useRef, useState, useCallback } from 'react';
import type { Notification } from '../types';

export type WSStatus = 'connected' | 'disconnected';

export function useWebSocket(token: string, onNotification: (n: Notification) => void) {
  const [status, setStatus] = useState<WSStatus>('disconnected');
  const wsRef = useRef<WebSocket | null>(null);
  const retryRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const cbRef = useRef(onNotification);
  cbRef.current = onNotification;

  const connect = useCallback(() => {
    if (!token) return;
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const ws = new WebSocket(`${proto}://${location.host}/ws?token=${token}`);
    wsRef.current = ws;

    ws.onopen = () => setStatus('connected');
    ws.onclose = () => {
      setStatus('disconnected');
      if (token) retryRef.current = setTimeout(connect, 5000);
    };
    ws.onerror = () => ws.close();
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data as string) as { event: string; notification?: Notification };
        if (msg.event === 'notification' && msg.notification) {
          cbRef.current(msg.notification);
          if (typeof Notification !== 'undefined' && Notification.permission === 'granted') {
            new Notification(msg.notification.title, { body: msg.notification.message });
          }
        }
      } catch { /* ignore */ }
    };
  }, [token]);

  useEffect(() => {
    connect();
    return () => {
      clearTimeout(retryRef.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return status;
}
