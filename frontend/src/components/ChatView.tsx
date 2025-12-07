import { useState, useRef, useEffect, useCallback } from 'react';
import type { ConversationMessage, SmartEvent } from '../types';

export interface ChatViewProps {
  messages: ConversationMessage[];
  smartEvent: SmartEvent | null;
  onSendMessage: (message: string) => void;
  onSmartEventAction: (action: string) => void;
  onDismissSmartEvent: () => void;
  connected: boolean;
  onResume?: () => void;
}

// Group consecutive messages from the same sender
function groupMessages(messages: ConversationMessage[]): ConversationMessage[][] {
  const groups: ConversationMessage[][] = [];
  let currentGroup: ConversationMessage[] = [];
  let lastType: string | null = null;

  for (const msg of messages) {
    const isUserMessage = msg.type === 'user_input';
    const isAgentMessage = ['claude_response', 'claude_action', 'action_result', 'command_output'].includes(msg.type);
    
    const currentSender = isUserMessage ? 'user' : isAgentMessage ? 'agent' : 'system';
    const lastSender = lastType === 'user_input' ? 'user' : 
                      lastType && ['claude_response', 'claude_action', 'action_result', 'command_output'].includes(lastType) ? 'agent' : 
                      'system';

    if (currentSender === lastSender && currentSender !== 'system') {
      currentGroup.push(msg);
    } else {
      if (currentGroup.length > 0) {
        groups.push(currentGroup);
      }
      currentGroup = [msg];
    }
    
    lastType = msg.type;
  }

  if (currentGroup.length > 0) {
    groups.push(currentGroup);
  }

  return groups;
}

export function ChatView({
  messages,
  smartEvent,
  onSendMessage,
  onSmartEventAction,
  onDismissSmartEvent,
  connected,
  onResume,
}: ChatViewProps) {
  const [inputValue, setInputValue] = useState('');
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  // Auto-scroll to bottom when new messages arrive
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages]);

  // Focus input on mount
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();
    if (!inputValue.trim() || !connected) return;

    onSendMessage(inputValue + '\n');
    setInputValue('');
  }, [inputValue, connected, onSendMessage]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  }, [handleSubmit]);

  const messageGroups = groupMessages(messages);

  return (
    <div className="flex flex-col h-full bg-gray-900">
      {/* Messages area */}
      <div className="flex-1 overflow-y-auto px-4 py-6 space-y-6">
        {messages.length === 0 ? (
          <div className="flex items-center justify-center h-full text-gray-500">
            <div className="text-center">
              <div className="text-6xl mb-4">🤖</div>
              <h3 className="text-xl font-medium text-gray-300 mb-2">Ready to assist</h3>
              <p className="text-sm">Start a conversation with your AI coding assistant</p>
            </div>
          </div>
        ) : (
          messageGroups.map((group, groupIndex) => (
            <MessageGroup key={groupIndex} messages={group} />
          ))
        )}
        <div ref={messagesEndRef} />
      </div>

      {/* Smart Event Banner */}
      {smartEvent && (
        <SmartEventBanner
          event={smartEvent}
          onAction={onSmartEventAction}
          onDismiss={onDismissSmartEvent}
        />
      )}

      {/* Input area */}
      <div className="border-t border-gray-700 bg-gray-800 p-4">
        <form onSubmit={handleSubmit} className="flex gap-2">
          {onResume && (
            <button
              type="button"
              onClick={onResume}
              disabled={!connected}
              className="px-4 py-2 bg-purple-600 hover:bg-purple-500 disabled:bg-purple-600/50 disabled:cursor-not-allowed text-white rounded-lg transition-colors font-medium flex items-center gap-2"
              title="Resume previous session"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              Resume
            </button>
          )}
          <textarea
            ref={inputRef}
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={connected ? "Type a message..." : "Disconnected..."}
            disabled={!connected}
            rows={1}
            className="flex-1 px-4 py-2 bg-gray-700 border border-gray-600 rounded-lg text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500 resize-none disabled:opacity-50 disabled:cursor-not-allowed"
            style={{ minHeight: '42px', maxHeight: '120px' }}
          />
          <button
            type="submit"
            disabled={!inputValue.trim() || !connected}
            className="px-6 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-blue-600/50 disabled:cursor-not-allowed text-white rounded-lg transition-colors font-medium"
          >
            Send
          </button>
        </form>
        <div className="mt-2 text-xs text-gray-500 text-center">
          Press Enter to send, Shift+Enter for new line
        </div>
      </div>
    </div>
  );
}

interface MessageGroupProps {
  messages: ConversationMessage[];
}

function MessageGroup({ messages }: MessageGroupProps) {
  const [expandedItems, setExpandedItems] = useState<Set<number>>(new Set());
  
  if (messages.length === 0) return null;

  const firstMessage = messages[0];
  const isUserGroup = firstMessage.type === 'user_input';
  const isSystemMessage = ['agent_interrupted', 'session_resumed'].includes(firstMessage.type);

  // System messages (centered)
  if (isSystemMessage) {
    return (
      <div className="flex justify-center">
        {messages.map((msg, idx) => (
          <SystemMessage key={idx} message={msg} />
        ))}
      </div>
    );
  }

  // User messages (right-aligned)
  if (isUserGroup) {
    return (
      <div className="flex justify-end">
        <div className="max-w-[85%] flex flex-col gap-2">
          <div className="flex items-center gap-2 justify-end px-2">
            <span className="text-xs text-gray-500">
              {new Date(firstMessage.timestamp).toLocaleTimeString()}
            </span>
            <span className="text-xs font-medium text-gray-400">You</span>
            <div className="w-6 h-6 rounded-full bg-blue-600 flex items-center justify-center text-white text-xs font-medium">
              U
            </div>
          </div>
          {messages.map((msg, idx) => (
            <div key={idx} className="bg-blue-600 text-white px-4 py-3 rounded-2xl rounded-tr-sm">
              <div className="whitespace-pre-wrap break-words">{msg.content}</div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  // Agent messages (left-aligned)
  return (
    <div className="flex justify-start">
      <div className="max-w-[85%] flex flex-col gap-2">
        <div className="flex items-center gap-2 px-2">
          <div className="w-6 h-6 rounded-full bg-gradient-to-br from-purple-500 to-blue-500 flex items-center justify-center text-white text-xs font-medium">
            AI
          </div>
          <span className="text-xs font-medium text-gray-400">Claude</span>
          <span className="text-xs text-gray-500">
            {new Date(firstMessage.timestamp).toLocaleTimeString()}
          </span>
        </div>
        {messages.map((msg, idx) => (
          <AgentMessageItem
            key={idx}
            message={msg}
            isExpanded={expandedItems.has(idx)}
            onToggleExpand={() => {
              const newExpanded = new Set(expandedItems);
              if (newExpanded.has(idx)) {
                newExpanded.delete(idx);
              } else {
                newExpanded.add(idx);
              }
              setExpandedItems(newExpanded);
            }}
          />
        ))}
      </div>
    </div>
  );
}

interface AgentMessageItemProps {
  message: ConversationMessage;
  isExpanded: boolean;
  onToggleExpand: () => void;
}

function AgentMessageItem({ message, isExpanded, onToggleExpand }: AgentMessageItemProps) {
  const getMessageConfig = () => {
    switch (message.type) {
      case 'claude_response':
        return {
          icon: '💬',
          label: 'Response',
          bg: 'bg-gray-800',
          border: 'border-gray-700',
          collapsible: false,
        };
      case 'claude_action':
        return {
          icon: '⚡',
          label: 'Action',
          bg: 'bg-green-900/20',
          border: 'border-green-700/30',
          collapsible: true,
        };
      case 'action_result':
        return {
          icon: '✓',
          label: 'Result',
          bg: 'bg-gray-800/50',
          border: 'border-gray-700/50',
          collapsible: true,
        };
      case 'command_output':
        return {
          icon: '📟',
          label: 'Output',
          bg: 'bg-gray-800/50',
          border: 'border-gray-700/50',
          collapsible: true,
        };
      default:
        return {
          icon: '💬',
          label: 'Message',
          bg: 'bg-gray-800',
          border: 'border-gray-700',
          collapsible: false,
        };
    }
  };

  const config = getMessageConfig();

  return (
    <div className={`${config.bg} border ${config.border} rounded-lg overflow-hidden`}>
      {config.collapsible ? (
        <>
          <button
            onClick={onToggleExpand}
            className="w-full px-4 py-2 flex items-center justify-between hover:bg-gray-700/30 transition-colors"
          >
            <div className="flex items-center gap-2">
              <span className="text-sm">{config.icon}</span>
              <span className="text-sm font-medium text-gray-300">{config.label}</span>
            </div>
            <span className="text-gray-500 text-xs">
              {isExpanded ? '▼' : '▶'}
            </span>
          </button>
          {isExpanded && (
            <div className="px-4 py-3 border-t border-gray-700/50">
              <pre className="text-sm text-gray-300 whitespace-pre-wrap break-words font-mono">
                {message.content}
              </pre>
            </div>
          )}
        </>
      ) : (
        <div className="px-4 py-3">
          <div className="text-sm text-gray-200 whitespace-pre-wrap break-words">
            {message.content}
          </div>
        </div>
      )}
    </div>
  );
}

function SystemMessage({ message }: { message: ConversationMessage }) {
  const config = message.type === 'agent_interrupted'
    ? { icon: '⚠', label: 'Interrupted', color: 'text-red-400 bg-red-900/20 border-red-700/30' }
    : { icon: '🔄', label: 'Session Resumed', color: 'text-blue-400 bg-blue-900/20 border-blue-700/30' };

  return (
    <div className={`${config.color} border px-4 py-2 rounded-full text-sm flex items-center gap-2`}>
      <span>{config.icon}</span>
      <span className="font-medium">{config.label}</span>
    </div>
  );
}

interface SmartEventBannerProps {
  event: SmartEvent;
  onAction: (action: string) => void;
  onDismiss: () => void;
}

function SmartEventBanner({ event, onAction, onDismiss }: SmartEventBannerProps) {
  const handleOptionClick = (option: string) => {
    let input: string;
    
    if (event.kind === 'claude_confirm') {
      switch (option.toLowerCase()) {
        case '1':
        case 'yes':
          input = '1';
          break;
        case '2':
        case 'all':
          input = '2';
          break;
        case 'esc':
        case 'cancel':
          input = '\x1b';
          break;
        default:
          input = option;
      }
    } else if (event.kind === 'question') {
      if (option.toLowerCase() === 'yes' || option.toLowerCase() === 'y') {
        input = 'y\n';
      } else if (option.toLowerCase() === 'no' || option.toLowerCase() === 'n') {
        input = 'n\n';
      } else {
        input = option + '\n';
      }
    } else {
      input = option + '\n';
    }
    
    onAction(input);
  };

  const getOptionLabel = (option: string) => {
    if (event.kind === 'claude_confirm') {
      switch (option.toLowerCase()) {
        case '1':
          return 'Yes';
        case '2':
          return 'Yes, allow all';
        case 'esc':
          return 'Cancel';
        default:
          return option;
      }
    }
    return option;
  };

  const getOptionStyle = (option: string) => {
    if (event.kind === 'claude_confirm') {
      switch (option.toLowerCase()) {
        case '1':
          return 'bg-green-600 hover:bg-green-500';
        case '2':
          return 'bg-blue-600 hover:bg-blue-500';
        case 'esc':
          return 'bg-gray-600 hover:bg-gray-500';
      }
    }
    
    const lower = option.toLowerCase();
    if (lower === 'yes' || lower === 'y') {
      return 'bg-green-600 hover:bg-green-500';
    } else if (lower === 'no' || lower === 'n') {
      return 'bg-red-600 hover:bg-red-500';
    }
    
    return 'bg-blue-600 hover:bg-blue-500';
  };

  return (
    <div className="border-t border-yellow-500/50 bg-yellow-900/20 p-4">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <span className="text-yellow-400 text-xl">❓</span>
          <div className="flex-1 min-w-0">
            <div className="text-sm font-medium text-yellow-300 mb-1">
              {event.kind === 'claude_confirm' ? 'Claude needs confirmation' : 'Question'}
            </div>
            <div className="text-gray-200 text-sm">
              {event.prompt || 'Waiting for your response...'}
            </div>
          </div>
        </div>
        
        <div className="flex items-center gap-2 flex-shrink-0">
          {event.options?.map((option, index) => (
            <button
              key={index}
              onClick={() => handleOptionClick(option)}
              className={`px-4 py-2 rounded-lg text-white font-medium transition-colors ${getOptionStyle(option)}`}
            >
              {event.kind === 'claude_confirm' && (
                <span className="text-xs opacity-75 mr-1">{option}</span>
              )}
              {getOptionLabel(option)}
            </button>
          ))}
          
          <button
            onClick={onDismiss}
            className="px-2 py-2 text-gray-400 hover:text-gray-200 transition-colors"
            title="Dismiss"
          >
            ✕
          </button>
        </div>
      </div>
    </div>
  );
}

export default ChatView;
