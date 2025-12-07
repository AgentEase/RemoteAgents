// Session types
export interface Session {
  id: string;
  userId: string;
  name: string;
  command: string;
  workdir?: string;
  env?: Record<string, string>;
  status: 'running' | 'exited' | 'failed';
  exitCode?: number;
  pid?: number;
  logFilePath: string;
  createdAt: string;
  updatedAt: string;
}

// SmartEvent types
export interface SmartEvent {
  kind: 'question' | 'idle' | 'progress' | 'claude_confirm';
  options?: string[];
  prompt?: string;
}

// WebSocket message types
export type WSMessageType = 
  | 'stdin' 
  | 'command'
  | 'stdout' 
  | 'resize' 
  | 'ping' 
  | 'pong' 
  | 'smart_event' 
  | 'status' 
  | 'history'
  | 'conversation';

// Conversation message from driver parsing
export interface ConversationMessage {
  timestamp: string;
  type: 'user_input' | 'claude_response' | 'claude_action' | 'action_result' | 'command_output' | 'agent_interrupted' | 'session_resumed';
  content: string;
}

export interface WSMessage {
  type: WSMessageType;
  data?: string;
  rows?: number;
  cols?: number;
  payload?: SmartEvent | ConversationMessage;
  state?: string;
  code?: number;
}

// Client -> Server messages
export interface StdinMessage {
  type: 'stdin';
  data: string;
}

export interface CommandMessage {
  type: 'command';
  data: string;
}

export interface ResizeMessage {
  type: 'resize';
  rows: number;
  cols: number;
}

export interface PingMessage {
  type: 'ping';
}

// Server -> Client messages
export interface StdoutMessage {
  type: 'stdout';
  data: string;
}

export interface SmartEventMessage {
  type: 'smart_event';
  payload: SmartEvent;
}

export interface StatusMessage {
  type: 'status';
  state: string;
  code?: number;
}

export interface HistoryMessage {
  type: 'history';
  data: string;
}

export interface PongMessage {
  type: 'pong';
}

export interface ConversationWSMessage {
  type: 'conversation';
  payload: ConversationMessage;
}

// API request/response types
export interface CreateSessionRequest {
  command: string;
  name: string;
  workdir?: string;
  env?: Record<string, string>;
}

export interface ApiError {
  error: {
    code: string;
    message: string;
    details?: Record<string, unknown>;
  };
}
