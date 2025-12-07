import { useState, useEffect, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import { listSessions, deleteSession } from '../api/client';
import type { Session } from '../types';

export interface SessionListProps {
  sessions?: Session[];
  onSelect?: (id: string) => void;
  onDelete?: (id: string) => void;
  onReconnect?: (session: Session) => void;
  onRefresh?: () => void;
  onCreateNew?: () => void;
}

// Calculate duration from createdAt to now or updatedAt
function formatDuration(createdAt: string, status: string): string {
  const start = new Date(createdAt).getTime();
  const end = status === 'running' ? Date.now() : Date.now();
  const diff = Math.floor((end - start) / 1000);
  
  if (diff < 60) return `${diff}s`;
  if (diff < 3600) return `${Math.floor(diff / 60)}m ${diff % 60}s`;
  const hours = Math.floor(diff / 3600);
  const mins = Math.floor((diff % 3600) / 60);
  return `${hours}h ${mins}m`;
}

// Status badge component
function StatusBadge({ status }: { status: Session['status'] }) {
  const styles = {
    running: 'bg-green-500/20 text-green-400 border-green-500/30',
    exited: 'bg-gray-500/20 text-gray-400 border-gray-500/30',
    failed: 'bg-red-500/20 text-red-400 border-red-500/30',
  };

  const icons = {
    running: '●',
    exited: '○',
    failed: '✕',
  };

  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium border ${styles[status]}`}>
      <span className={status === 'running' ? 'animate-pulse' : ''}>{icons[status]}</span>
      {status}
    </span>
  );
}


// Session card component
function SessionCard({ 
  session, 
  onSelect, 
  onDelete,
  onReconnect,
}: { 
  session: Session; 
  onSelect: () => void; 
  onDelete: () => void;
  onReconnect?: () => void;
}) {
  const [isDeleting, setIsDeleting] = useState(false);

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (isDeleting) return;
    
    if (window.confirm(`Delete session "${session.name}"?`)) {
      setIsDeleting(true);
      onDelete();
    }
  };

  const handleReconnect = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (onReconnect) {
      onReconnect();
    }
  };

  // Check if this is a Claude session that can be reconnected
  const canReconnect = session.status === 'exited' && 
                       session.command.includes('claude') && 
                       !session.command.includes('--resume');

  return (
    <div
      onClick={onSelect}
      className="bg-gray-800 border border-gray-700 rounded-lg p-4 hover:border-gray-600 hover:bg-gray-750 transition-all cursor-pointer group"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-3 mb-2">
            <h3 className="text-lg font-medium text-gray-100 truncate">
              {session.name}
            </h3>
            <StatusBadge status={session.status} />
          </div>
          
          <div className="text-sm text-gray-400 font-mono truncate mb-2">
            $ {session.command}
          </div>
          
          <div className="flex items-center gap-4 text-xs text-gray-500">
            <span title="Duration">
              ⏱ {formatDuration(session.createdAt, session.status)}
            </span>
            {session.exitCode !== undefined && (
              <span title="Exit code">
                Exit: {session.exitCode}
              </span>
            )}
            {session.pid && (
              <span title="Process ID">
                PID: {session.pid}
              </span>
            )}
          </div>
        </div>
        
        <div className="flex items-center gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
          {canReconnect && onReconnect && (
            <button
              onClick={handleReconnect}
              className="p-2 text-gray-400 hover:text-purple-400 hover:bg-purple-500/10 rounded transition-colors"
              title="Reconnect with resume"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>
          )}
          <button
            onClick={onSelect}
            className="p-2 text-gray-400 hover:text-blue-400 hover:bg-blue-500/10 rounded transition-colors"
            title="Open terminal"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
            </svg>
          </button>
          <button
            onClick={handleDelete}
            disabled={isDeleting}
            className="p-2 text-gray-400 hover:text-red-400 hover:bg-red-500/10 rounded transition-colors disabled:opacity-50"
            title="Delete session"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  );
}


// Empty state component
function EmptyState({ onCreateNew }: { onCreateNew?: () => void }) {
  return (
    <div className="text-center py-12">
      <div className="text-gray-500 mb-4">
        <svg className="w-16 h-16 mx-auto" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
        </svg>
      </div>
      <h3 className="text-lg font-medium text-gray-300 mb-2">No sessions yet</h3>
      <p className="text-gray-500 mb-6">Create a new terminal session to get started</p>
      {onCreateNew && (
        <button
          onClick={onCreateNew}
          className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New Session
        </button>
      )}
    </div>
  );
}

// Loading skeleton
function LoadingSkeleton() {
  return (
    <div className="space-y-4">
      {[1, 2, 3].map((i) => (
        <div key={i} className="bg-gray-800 border border-gray-700 rounded-lg p-4 animate-pulse">
          <div className="flex items-start justify-between gap-4">
            <div className="flex-1">
              <div className="flex items-center gap-3 mb-2">
                <div className="h-6 w-32 bg-gray-700 rounded" />
                <div className="h-5 w-16 bg-gray-700 rounded-full" />
              </div>
              <div className="h-4 w-48 bg-gray-700 rounded mb-2" />
              <div className="h-3 w-24 bg-gray-700 rounded" />
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// Main SessionList component
export function SessionList({ 
  sessions: propSessions, 
  onSelect: propOnSelect, 
  onDelete: propOnDelete,
  onReconnect: propOnReconnect,
  onRefresh,
  onCreateNew,
}: SessionListProps) {
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<Session[]>(propSessions || []);
  const [loading, setLoading] = useState(!propSessions);
  const [error, setError] = useState<string | null>(null);

  // Fetch sessions from API if not provided via props
  const fetchSessions = useCallback(async () => {
    if (propSessions) {
      setSessions(propSessions);
      return;
    }

    setLoading(true);
    setError(null);
    
    try {
      const data = await listSessions();
      setSessions(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sessions');
    } finally {
      setLoading(false);
    }
  }, [propSessions]);

  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

  // Update sessions when props change
  useEffect(() => {
    if (propSessions) {
      setSessions(propSessions);
    }
  }, [propSessions]);

  const handleSelect = (id: string) => {
    if (propOnSelect) {
      propOnSelect(id);
    } else {
      navigate(`/sessions/${id}`);
    }
  };

  const handleDelete = async (id: string) => {
    if (propOnDelete) {
      propOnDelete(id);
      return;
    }

    try {
      await deleteSession(id);
      setSessions((prev) => prev.filter((s) => s.id !== id));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete session');
    }
  };

  const handleRefresh = () => {
    if (onRefresh) {
      onRefresh();
    } else {
      fetchSessions();
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 text-gray-100">
      <div className="max-w-4xl mx-auto px-4 py-8">
        {/* Header */}
        <div className="flex items-center justify-between mb-8">
          <div>
            <h1 className="text-2xl font-bold text-gray-100">Terminal Sessions</h1>
            <p className="text-gray-400 mt-1">Manage your remote CLI agent sessions</p>
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={handleRefresh}
              className="p-2 text-gray-400 hover:text-gray-200 hover:bg-gray-800 rounded-lg transition-colors"
              title="Refresh"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>
            <button
              onClick={onCreateNew}
              className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded-lg transition-colors"
            >
              <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
              </svg>
              New Session
            </button>
          </div>
        </div>

        {/* Error message */}
        {error && (
          <div className="mb-6 p-4 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400">
            {error}
          </div>
        )}

        {/* Content */}
        {loading ? (
          <LoadingSkeleton />
        ) : sessions.length === 0 ? (
          <EmptyState onCreateNew={onCreateNew} />
        ) : (
          <div className="space-y-4">
            {sessions.map((session) => (
              <SessionCard
                key={session.id}
                session={session}
                onSelect={() => handleSelect(session.id)}
                onDelete={() => handleDelete(session.id)}
                onReconnect={propOnReconnect ? () => propOnReconnect(session) : undefined}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

export default SessionList;
