import { useState, useCallback, useRef, useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Terminal, type TerminalApi } from '../components/Terminal';
import { SmartEventOverlay } from '../components/SmartEventOverlay';
import { ChatView } from '../components/ChatView';
import { useTerminalWebSocket } from '../hooks/useTerminalWebSocket';
import { getSession } from '../api/client';
import type { SmartEvent, Session, ConversationMessage } from '../types';

export interface TerminalPageProps {
  sessionId?: string;
}

// Connection status indicator
function ConnectionStatus({ 
  connected, 
  connecting, 
  reconnectAttempts,
  sessionStatus,
  isClaudeSession,
  onReconnect,
  onResumeReconnect,
}: { 
  connected: boolean; 
  connecting: boolean;
  reconnectAttempts: number;
  sessionStatus: string;
  isClaudeSession: boolean;
  onReconnect: () => void;
  onResumeReconnect?: () => void;
}) {
  if (connected) {
    return (
      <span className="inline-flex items-center gap-1.5 text-green-400 text-sm">
        <span className="w-2 h-2 bg-green-400 rounded-full animate-pulse" />
        Connected
      </span>
    );
  }

  if (connecting) {
    return (
      <span className="inline-flex items-center gap-1.5 text-yellow-400 text-sm">
        <span className="w-2 h-2 bg-yellow-400 rounded-full animate-pulse" />
        Connecting...
      </span>
    );
  }

  // Check if session has exited (not just WebSocket disconnected)
  const sessionExited = sessionStatus === 'exited' || sessionStatus === 'failed';
  const canResumeReconnect = sessionExited && isClaudeSession && onResumeReconnect;

  // Debug logging
  console.log('ConnectionStatus:', {
    sessionStatus,
    sessionExited,
    isClaudeSession,
    canResumeReconnect,
    reconnectAttempts
  });

  return (
    <span className="inline-flex items-center gap-2 text-red-400 text-sm">
      <span className="w-2 h-2 bg-red-400 rounded-full" />
      Disconnected
      {reconnectAttempts > 0 && !sessionExited && (
        <span className="text-gray-500">({reconnectAttempts} retries)</span>
      )}
      <button
        onClick={canResumeReconnect ? onResumeReconnect : onReconnect}
        className={`ml-2 px-2 py-0.5 rounded text-xs transition-colors ${
          canResumeReconnect 
            ? 'bg-purple-500/20 hover:bg-purple-500/30 text-purple-300' 
            : 'bg-red-500/20 hover:bg-red-500/30'
        }`}
        title={canResumeReconnect ? 'Create new session with resume' : 'Reconnect WebSocket'}
      >
        {canResumeReconnect ? 'Resume Session' : 'Reconnect'}
      </button>
    </span>
  );
}


// Session status badge
function SessionStatusBadge({ status, exitCode }: { status: string; exitCode?: number }) {
  const styles: Record<string, string> = {
    running: 'bg-green-500/20 text-green-400 border-green-500/30',
    exited: 'bg-gray-500/20 text-gray-400 border-gray-500/30',
    failed: 'bg-red-500/20 text-red-400 border-red-500/30',
  };

  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium border ${styles[status] || styles.running}`}>
      {status}
      {exitCode !== undefined && ` (${exitCode})`}
    </span>
  );
}

export function TerminalPage({ sessionId: propSessionId }: TerminalPageProps) {
  const { id: paramSessionId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const sessionId = propSessionId || paramSessionId || '';

  const [viewMode, setViewMode] = useState<'terminal' | 'chat'>('terminal');
  const [smartEvent, setSmartEvent] = useState<SmartEvent | null>(null);
  const [sessionStatus, setSessionStatus] = useState<string>('running');
  const [exitCode, setExitCode] = useState<number | undefined>();
  const [session, setSession] = useState<Session | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [conversationMessages, setConversationMessages] = useState<ConversationMessage[]>([]);
  
  const terminalContainerRef = useRef<HTMLDivElement>(null);
  const terminalApiRef = useRef<TerminalApi | null>(null);

  // Fetch session details
  const fetchSessionData = useCallback(async () => {
    if (!sessionId) return;

    try {
      const data = await getSession(sessionId);
      console.log('Fetched session data:', data);
      setSession(data);
      setSessionStatus(data.status);
      setExitCode(data.exitCode);
      console.log('Updated sessionStatus to:', data.status);
    } catch (err) {
      if (err instanceof Error && err.message.includes('404')) {
        setError('Session not found');
        return;
      }
      setError(err instanceof Error ? err.message : 'Failed to load session');
    }
  }, [sessionId]);

  useEffect(() => {
    fetchSessionData();
  }, [fetchSessionData]);

  // WebSocket callbacks
  const handleStdout = useCallback((data: string) => {
    terminalApiRef.current?.write(data);
  }, []);

  const handleHistory = useCallback((data: string) => {
    terminalApiRef.current?.write(data);
  }, []);

  const handleSmartEvent = useCallback((event: SmartEvent) => {
    setSmartEvent(event);
  }, []);

  const handleStatus = useCallback((state: string, code?: number) => {
    setSessionStatus(state);
    if (code !== undefined) {
      setExitCode(code);
    }
  }, []);

  const handleConversation = useCallback((message: ConversationMessage) => {
    console.log('Received conversation message:', message);
    setConversationMessages((prev) => [...prev, message]);
  }, []);

  const handleDisconnect = useCallback(() => {
    // When WebSocket disconnects, refresh session status to check if process exited
    console.log('WebSocket disconnected, fetching session status...');
    // Add a small delay to allow backend to update status
    setTimeout(() => {
      fetchSessionData();
    }, 500);
  }, [fetchSessionData]);

  const handleError = useCallback((error: string) => {
    console.log('WebSocket error:', error);
    // If we get connection errors, the process might have exited
    // Fetch session status to check
    setTimeout(() => {
      fetchSessionData();
    }, 500);
  }, [fetchSessionData]);

  // WebSocket connection
  const {
    connected,
    connecting,
    reconnectAttempts,
    sendStdin,
    sendCommand,
    sendResize,
    reconnect,
  } = useTerminalWebSocket(
    { sessionId },
    {
      onStdout: handleStdout,
      onHistory: handleHistory,
      onSmartEvent: handleSmartEvent,
      onStatus: handleStatus,
      onConversation: handleConversation,
      onDisconnect: handleDisconnect,
      onError: handleError,
    }
  );

  // Refresh session status when reconnect attempts reach max
  // This helps detect if the session has exited
  useEffect(() => {
    if (reconnectAttempts >= 10 && !connected) {
      fetchSessionData();
    }
  }, [reconnectAttempts, connected, fetchSessionData]);

  // Terminal callbacks
  const handleTerminalData = useCallback((data: string) => {
    sendStdin(data);
  }, [sendStdin]);

  const handleTerminalResize = useCallback((rows: number, cols: number) => {
    sendResize(rows, cols);
  }, [sendResize]);

  const handleTerminalReady = useCallback(() => {
    // Get terminal API from container
    if (terminalContainerRef.current) {
      const container = terminalContainerRef.current.querySelector('[data-session-id]') as HTMLDivElement & { terminalApi?: TerminalApi };
      if (container?.terminalApi) {
        terminalApiRef.current = container.terminalApi;
        container.terminalApi.focus();
      }
    }
  }, []);

  // SmartEvent action handler
  const handleSmartEventAction = useCallback((action: string) => {
    sendCommand(action);
    setSmartEvent(null);
  }, [sendCommand]);

  const handleDismissSmartEvent = useCallback(() => {
    setSmartEvent(null);
  }, []);

  // Resume handler - sends "/resume" command and switches to terminal view
  const handleResume = useCallback(() => {
    // Switch to terminal view first so user can interact with the menu
    setViewMode('terminal');
    // Send the resume command after a short delay to ensure view has switched
    setTimeout(() => {
      sendCommand('/resume\n');
      // Focus the terminal
      if (terminalApiRef.current) {
        terminalApiRef.current.focus();
      }
    }, 150);
  }, [sendCommand]);

  // Resume reconnect handler - restarts the session with resume
  const handleResumeReconnect = useCallback(async () => {
    if (!session) return;
    
    try {
      // Import restartSession dynamically
      const { restartSession } = await import('../api/client');
      
      // Restart the session (keeps same session ID)
      await restartSession(session.id);
      
      // Refresh session data to get updated status
      await fetchSessionData();
      
      // Reconnect WebSocket
      reconnect();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to restart session');
    }
  }, [session, fetchSessionData, reconnect]);

  // Navigate back to session list
  const handleBack = () => {
    navigate('/');
  };

  if (!sessionId) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center">
        <div className="text-center">
          <h2 className="text-xl font-medium text-gray-300 mb-4">No session selected</h2>
          <button
            onClick={handleBack}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
          >
            Go to Sessions
          </button>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center">
        <div className="text-center">
          <div className="text-red-400 text-6xl mb-4">⚠</div>
          <h2 className="text-xl font-medium text-gray-300 mb-2">Error</h2>
          <p className="text-gray-500 mb-6">{error}</p>
          <button
            onClick={handleBack}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
          >
            Back to Sessions
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-gray-900 flex flex-col">
      {/* Header */}
      <header className="flex-shrink-0 bg-gray-800 border-b border-gray-700 px-4 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <button
              onClick={handleBack}
              className="p-2 text-gray-400 hover:text-gray-200 hover:bg-gray-700 rounded-lg transition-colors"
              title="Back to sessions"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 19l-7-7m0 0l7-7m-7 7h18" />
              </svg>
            </button>
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-lg font-medium text-gray-100">
                  {session?.name || 'Terminal'}
                </h1>
                <SessionStatusBadge status={sessionStatus} exitCode={exitCode} />
              </div>
              {session?.command && (
                <p className="text-sm text-gray-500 font-mono">$ {session.command}</p>
              )}
            </div>
          </div>
          
          <div className="flex items-center gap-3">
            {/* View mode toggle */}
            <div className="flex items-center gap-1 bg-gray-700 rounded-lg p-1">
              <button
                onClick={() => setViewMode('terminal')}
                className={`px-3 py-1 rounded text-sm transition-colors ${
                  viewMode === 'terminal'
                    ? 'bg-gray-600 text-white'
                    : 'text-gray-400 hover:text-gray-200'
                }`}
                title="Terminal view"
              >
                💻 Terminal
              </button>
              <button
                onClick={() => setViewMode('chat')}
                className={`px-3 py-1 rounded text-sm transition-colors ${
                  viewMode === 'chat'
                    ? 'bg-gray-600 text-white'
                    : 'text-gray-400 hover:text-gray-200'
                }`}
                title="Chat view"
              >
                💬 Chat
              </button>
            </div>

            <ConnectionStatus
              connected={connected}
              connecting={connecting}
              reconnectAttempts={reconnectAttempts}
              sessionStatus={sessionStatus}
              isClaudeSession={session?.command.includes('claude') || false}
              onReconnect={reconnect}
              onResumeReconnect={handleResumeReconnect}
            />
          </div>
        </div>
      </header>

      {/* Content container */}
      <div ref={terminalContainerRef} className="flex-1 relative">
        {viewMode === 'terminal' ? (
          <>
            {/* SmartEvent overlay */}
            <SmartEventOverlay
              event={smartEvent}
              onAction={handleSmartEventAction}
              onDismiss={handleDismissSmartEvent}
            />
            
            {/* Terminal */}
            <div className="absolute inset-0">
              <Terminal
                sessionId={sessionId}
                onData={handleTerminalData}
                onResize={handleTerminalResize}
                onReady={handleTerminalReady}
              />
            </div>
          </>
        ) : (
          /* Chat view */
          <div className="absolute inset-0">
            <ChatView
              messages={conversationMessages}
              smartEvent={smartEvent}
              onSendMessage={sendCommand}
              onSmartEventAction={handleSmartEventAction}
              onDismissSmartEvent={handleDismissSmartEvent}
              connected={connected}
              onResume={session?.command.includes('claude') ? handleResume : undefined}
            />
          </div>
        )}
      </div>
    </div>
  );
}

export default TerminalPage;
