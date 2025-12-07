import { useState, useEffect, useRef, useCallback } from 'react';
import { getWebSocketUrl } from '../api/client';
import type { WSMessage, SmartEvent, ConversationMessage } from '../types';

export interface UseTerminalWebSocketOptions {
  sessionId: string;
  baseUrl?: string;
  reconnectInterval?: number;
  maxReconnectAttempts?: number;
  pingInterval?: number;
}

export interface UseTerminalWebSocketReturn {
  connected: boolean;
  connecting: boolean;
  error: string | null;
  reconnectAttempts: number;
  send: (msg: WSMessage) => void;
  sendStdin: (data: string) => void;
  sendCommand: (data: string) => void;
  sendResize: (rows: number, cols: number) => void;
  disconnect: () => void;
  reconnect: () => void;
}

export interface TerminalWebSocketCallbacks {
  onStdout?: (data: string) => void;
  onHistory?: (data: string) => void;
  onSmartEvent?: (event: SmartEvent) => void;
  onStatus?: (state: string, code?: number) => void;
  onConversation?: (message: ConversationMessage) => void;
  onConnect?: () => void;
  onDisconnect?: () => void;
  onError?: (error: string) => void;
}

export function useTerminalWebSocket(
  options: UseTerminalWebSocketOptions,
  callbacks: TerminalWebSocketCallbacks = {}
): UseTerminalWebSocketReturn {
  const {
    sessionId,
    baseUrl = `ws://${window.location.host}`,
    reconnectInterval = 3000,
    maxReconnectAttempts = 10,
    pingInterval = 30000,
  } = options;

  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [reconnectAttempts, setReconnectAttempts] = useState(0);

  const wsRef = useRef<WebSocket | null>(null);
  const pingIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const shouldReconnectRef = useRef(true);
  const callbacksRef = useRef(callbacks);

  // Update callbacks ref when callbacks change
  useEffect(() => {
    callbacksRef.current = callbacks;
  }, [callbacks]);

  // Clear ping interval
  const clearPingInterval = useCallback(() => {
    if (pingIntervalRef.current) {
      clearInterval(pingIntervalRef.current);
      pingIntervalRef.current = null;
    }
  }, []);

  // Clear reconnect timeout
  const clearReconnectTimeout = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
  }, []);

  // Start ping interval
  const startPingInterval = useCallback(() => {
    clearPingInterval();
    pingIntervalRef.current = setInterval(() => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(JSON.stringify({ type: 'ping' }));
      }
    }, pingInterval);
  }, [clearPingInterval, pingInterval]);

  // Handle incoming messages
  const handleMessage = useCallback((event: MessageEvent) => {
    try {
      const msg: WSMessage = JSON.parse(event.data);
      
      // Debug: log all messages
      if (msg.type === 'conversation') {
        console.log('WebSocket conversation message:', msg);
      }
      
      switch (msg.type) {
        case 'stdout':
          callbacksRef.current.onStdout?.(msg.data || '');
          break;
        case 'history':
          callbacksRef.current.onHistory?.(msg.data || '');
          break;
        case 'smart_event':
          if (msg.payload && 'kind' in msg.payload) {
            callbacksRef.current.onSmartEvent?.(msg.payload as SmartEvent);
          }
          break;
        case 'status':
          callbacksRef.current.onStatus?.(msg.state || '', msg.code);
          break;
        case 'conversation':
          if (msg.payload && 'timestamp' in msg.payload) {
            console.log('Calling onConversation with:', msg.payload);
            callbacksRef.current.onConversation?.(msg.payload as ConversationMessage);
          }
          break;
        case 'pong':
          // Heartbeat response, no action needed
          break;
        default:
          console.warn('Unknown message type:', msg.type);
      }
    } catch (err) {
      console.error('Failed to parse WebSocket message:', err);
    }
  }, []);

  // Connect to WebSocket
  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      return;
    }

    setConnecting(true);
    setError(null);

    // Use API client to get WebSocket URL with auth token
    const wsUrl = baseUrl 
      ? `${baseUrl}/api/sessions/${sessionId}/attach`
      : getWebSocketUrl(sessionId);
    const ws = new WebSocket(wsUrl);

    ws.onopen = () => {
      setConnected(true);
      setConnecting(false);
      setReconnectAttempts(0);
      setError(null);
      startPingInterval();
      callbacksRef.current.onConnect?.();
    };

    ws.onclose = () => {
      setConnected(false);
      setConnecting(false);
      clearPingInterval();
      callbacksRef.current.onDisconnect?.();

      // Auto-reconnect if enabled
      if (shouldReconnectRef.current && reconnectAttempts < maxReconnectAttempts) {
        reconnectTimeoutRef.current = setTimeout(() => {
          setReconnectAttempts((prev) => prev + 1);
          connect();
        }, reconnectInterval);
      }
    };

    ws.onerror = () => {
      const errorMsg = 'WebSocket connection error';
      setError(errorMsg);
      callbacksRef.current.onError?.(errorMsg);
    };

    ws.onmessage = handleMessage;

    wsRef.current = ws;
  }, [
    baseUrl,
    sessionId,
    reconnectAttempts,
    maxReconnectAttempts,
    reconnectInterval,
    startPingInterval,
    clearPingInterval,
    handleMessage,
  ]);

  // Disconnect from WebSocket
  const disconnect = useCallback(() => {
    shouldReconnectRef.current = false;
    clearReconnectTimeout();
    clearPingInterval();
    
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    
    setConnected(false);
    setConnecting(false);
  }, [clearReconnectTimeout, clearPingInterval]);

  // Manual reconnect
  const reconnect = useCallback(() => {
    disconnect();
    shouldReconnectRef.current = true;
    setReconnectAttempts(0);
    connect();
  }, [disconnect, connect]);

  // Send message
  const send = useCallback((msg: WSMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg));
    }
  }, []);

  // Send stdin data (for Terminal view - real-time input)
  const sendStdin = useCallback((data: string) => {
    send({ type: 'stdin', data });
  }, [send]);

  // Send command (for Chat view - complete commands with input clearing)
  const sendCommand = useCallback((data: string) => {
    send({ type: 'command', data });
  }, [send]);

  // Send resize event
  const sendResize = useCallback((rows: number, cols: number) => {
    send({ type: 'resize', rows, cols });
  }, [send]);

  // Connect on mount, disconnect on unmount
  useEffect(() => {
    shouldReconnectRef.current = true;
    connect();

    return () => {
      shouldReconnectRef.current = false;
      clearReconnectTimeout();
      clearPingInterval();
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [sessionId]); // eslint-disable-line react-hooks/exhaustive-deps

  return {
    connected,
    connecting,
    error,
    reconnectAttempts,
    send,
    sendStdin,
    sendCommand,
    sendResize,
    disconnect,
    reconnect,
  };
}

export default useTerminalWebSocket;
