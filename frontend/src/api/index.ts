/**
 * API module exports
 */

export {
  ApiClient,
  ApiClientError,
  apiClient,
  createSession,
  listSessions,
  getSession,
  deleteSession,
  downloadSessionLogs,
  getWebSocketUrl,
  setAuthToken,
  clearAuthToken,
  isAuthenticated,
} from './client';

export type {
  ApiClientConfig,
  SessionListResponse,
} from './client';
