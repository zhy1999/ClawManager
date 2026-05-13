import { useEffect, useRef, useState, useCallback } from 'react';

interface WebSocketMessage {
  type: string;
  user_id?: number;
  instance_id?: number;
  data: any;
  timestamp: string;
}

interface InstanceStatusUpdate {
  instance_id: number;
  status: string;
  pod_name?: string;
  pod_ip?: string;
  updated_at: string;
}

type MessageHandler = (message: WebSocketMessage) => void;

export function useWebSocket() {
  const [isConnected, setIsConnected] = useState(false);
  const [lastMessage, setLastMessage] = useState<WebSocketMessage | null>(null);
  const ws = useRef<WebSocket | null>(null);
  const reconnectTimeout = useRef<ReturnType<typeof setTimeout> | null>(null);
  const messageHandlers = useRef<Set<MessageHandler>>(new Set());
  const token = localStorage.getItem('access_token');

  const connect = useCallback(() => {
    if (!token || ws.current?.readyState === WebSocket.OPEN) {
      return;
    }

    // Build WebSocket URL with token
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsHost = window.location.host;
    const wsUrl = `${wsProtocol}//${wsHost}/api/v1/ws?token=${token}`;

    try {
      ws.current = new WebSocket(wsUrl);

      ws.current.onopen = () => {
        console.log('WebSocket connected');
        setIsConnected(true);
        
        // Clear any reconnect timeout
        if (reconnectTimeout.current) {
          clearTimeout(reconnectTimeout.current);
          reconnectTimeout.current = null;
        }
      };

      ws.current.onmessage = (event) => {
        try {
          const message: WebSocketMessage = JSON.parse(event.data);
          setLastMessage(message);
          
          // Notify all registered handlers
          messageHandlers.current.forEach(handler => {
            try {
              handler(message);
            } catch (err) {
              console.error('Message handler error:', err);
            }
          });
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err);
        }
      };

      ws.current.onclose = () => {
        console.log('WebSocket disconnected');
        setIsConnected(false);
        ws.current = null;
        
        // Attempt to reconnect after 3 seconds
        if (!reconnectTimeout.current) {
          reconnectTimeout.current = setTimeout(() => {
            reconnectTimeout.current = null;
            connect();
          }, 3000);
        }
      };

      ws.current.onerror = (error) => {
        console.error('WebSocket error:', error);
      };
    } catch (err) {
      console.error('Failed to create WebSocket connection:', err);
    }
  }, [token]);

  const disconnect = useCallback(() => {
    if (reconnectTimeout.current) {
      clearTimeout(reconnectTimeout.current);
      reconnectTimeout.current = null;
    }
    
    if (ws.current) {
      ws.current.close();
      ws.current = null;
    }
    setIsConnected(false);
  }, []);

  const sendMessage = useCallback((data: any) => {
    if (ws.current?.readyState === WebSocket.OPEN) {
      ws.current.send(JSON.stringify(data));
    } else {
      console.warn('WebSocket is not connected');
    }
  }, []);

  const addMessageHandler = useCallback((handler: MessageHandler) => {
    messageHandlers.current.add(handler);
    
    // Return cleanup function
    return () => {
      messageHandlers.current.delete(handler);
    };
  }, []);

  const removeMessageHandler = useCallback((handler: MessageHandler) => {
    messageHandlers.current.delete(handler);
  }, []);

  // Connect when token is available
  useEffect(() => {
    if (token) {
      connect();
    } else {
      disconnect();
    }

    return () => {
      disconnect();
    };
  }, [token, connect, disconnect]);

  // Ping to keep connection alive
  useEffect(() => {
    if (!isConnected) return;

    const pingInterval = setInterval(() => {
      sendMessage({ type: 'ping' });
    }, 30000);

    return () => {
      clearInterval(pingInterval);
    };
  }, [isConnected, sendMessage]);

  return {
    isConnected,
    lastMessage,
    sendMessage,
    addMessageHandler,
    removeMessageHandler,
    connect,
    disconnect,
  };
}

// Hook specifically for instance status updates
export function useInstanceStatusWebSocket(
  onStatusUpdate?: (update: InstanceStatusUpdate) => void
) {
  const { addMessageHandler, removeMessageHandler, isConnected } = useWebSocket();

  useEffect(() => {
    if (!onStatusUpdate) return;

    const handler = (message: WebSocketMessage) => {
      if (message.type === 'instance_status' && message.data) {
        onStatusUpdate(message.data as InstanceStatusUpdate);
      }
    };

    const cleanup = addMessageHandler(handler);
    
    return () => {
      cleanup();
    };
  }, [onStatusUpdate, addMessageHandler, removeMessageHandler]);

  return { isConnected };
}
