/**
 * HTTP API Client for Remote Agent Terminal
 * 
 * Provides centralized API access with authentication token management
 * and typed methods for session CRUD operations.
 * 
 * Requirements: 1.1, 2.1, 2.2, 2.3, 8.1
 */

import type { Session, CreateSessionRequest, ApiError } from '../types';

// Storage key for auth token
const AUTH_TOKEN_KEY = 'remote_terminal_auth_token';

/**
 * API response wrapper for session list
 */
export interface SessionListResponse {
  sessions: Session[];
}

/**
 * API client configuration options
 */
export interface ApiClientConfig {
  baseUrl?: string;
  onUnauthorized?: () => void;
}

/**
 * Custom error class for API errors
 */
export class ApiClientError extends Error {
  public readonly code: string;
  public readonly statusCode: number;
  public readonly details?: Record<string, unknown>;

  constructor(
    message: string,
    code: string,
    statusCode: number,
    details?: Record<string, unknown>
  ) {
    super(message);
    this.name = 'ApiClientError';
    this.code = code;
    this.statusCode = statusCode;
    this.details = details;
  }
}

/**
 * HTTP API Client class
 * 
 * Handles all HTTP communication with the backend API,
 * including authentication token management.
 */
export class ApiClient {
  private baseUrl: string;
  private onUnauthorized?: () => void;

  constructor(config: ApiClientConfig = {}) {
    this.baseUrl = config.baseUrl || '';
    this.onUnauthorized = config.onUnauthorized;
  }

  // ============================================
  // Token Management
  // ============================================

  /**
   * Get the current authentication token from storage
   */
  getToken(): string | null {
    try {
      return localStorage.getItem(AUTH_TOKEN_KEY);
    } catch {
      // localStorage may not be available in some contexts
      return null;
    }
  }

  /**
   * Set the authentication token in storage
   */
  setToken(token: string): void {
    try {
      localStorage.setItem(AUTH_TOKEN_KEY, token);
    } catch {
      // Silently fail if localStorage is not available
    }
  }

  /**
   * Remove the authentication token from storage
   */
  clearToken(): void {
    try {
      localStorage.removeItem(AUTH_TOKEN_KEY);
    } catch {
      // Silently fail if localStorage is not available
    }
  }

  /**
   * Check if user is authenticated (has a token)
   */
  isAuthenticated(): boolean {
    return this.getToken() !== null;
  }

  // ============================================
  // HTTP Request Helpers
  // ============================================

  /**
   * Build request headers with authentication
   */
  private buildHeaders(additionalHeaders?: Record<string, string>): Headers {
    const headers = new Headers({
      'Content-Type': 'application/json',
      ...additionalHeaders,
    });

    const token = this.getToken();
    if (token) {
      headers.set('Authorization', `Bearer ${token}`);
    }

    return headers;
  }

  /**
   * Parse error response from API
   */
  private async parseErrorResponse(response: Response): Promise<ApiClientError> {
    try {
      const data: ApiError = await response.json();
      return new ApiClientError(
        data.error?.message || response.statusText,
        data.error?.code || 'UNKNOWN_ERROR',
        response.status,
        data.error?.details
      );
    } catch {
      return new ApiClientError(
        response.statusText || 'Request failed',
        'UNKNOWN_ERROR',
        response.status
      );
    }
  }

  /**
   * Make an HTTP request to the API
   */
  private async request<T>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    const headers = this.buildHeaders();

    const options: RequestInit = {
      method,
      headers,
    };

    if (body !== undefined) {
      options.body = JSON.stringify(body);
    }

    const response = await fetch(url, options);

    // Handle unauthorized responses
    if (response.status === 401) {
      this.clearToken();
      this.onUnauthorized?.();
      throw await this.parseErrorResponse(response);
    }

    // Handle other error responses
    if (!response.ok) {
      throw await this.parseErrorResponse(response);
    }

    // Handle empty responses (e.g., 204 No Content)
    if (response.status === 204 || response.headers.get('content-length') === '0') {
      return undefined as T;
    }

    return response.json();
  }

  // ============================================
  // Session API Methods
  // ============================================

  /**
   * Create a new terminal session
   * 
   * Requirement 1.1: Create session with command, name, and optional env vars
   */
  async createSession(request: CreateSessionRequest): Promise<Session> {
    return this.request<Session>('POST', '/api/sessions', request);
  }

  /**
   * Get list of all sessions
   * 
   * Requirement 2.1: Return all sessions with ID, name, status, preview, duration
   */
  async listSessions(): Promise<Session[]> {
    const response = await this.request<SessionListResponse | Session[]>(
      'GET',
      '/api/sessions'
    );
    
    // Handle both { sessions: [...] } and [...] response formats
    if (Array.isArray(response)) {
      return response;
    }
    return response.sessions || [];
  }

  /**
   * Get details of a specific session
   * 
   * Requirement 2.2: Return complete session metadata including log file path
   */
  async getSession(id: string): Promise<Session> {
    return this.request<Session>('GET', `/api/sessions/${id}`);
  }

  /**
   * Delete a session
   * 
   * Requirement 2.3: Terminate PTY process and release all resources
   */
  async deleteSession(id: string): Promise<void> {
    return this.request<void>('DELETE', `/api/sessions/${id}`);
  }

  /**
   * Restart an exited session
   * 
   * Restarts the session with the same configuration, keeping the same session ID
   */
  async restartSession(id: string): Promise<Session> {
    return this.request<Session>('POST', `/api/sessions/${id}/restart`);
  }

  /**
   * Download session logs as Asciinema format
   * 
   * Returns the log file content as a Blob for download
   */
  async downloadSessionLogs(id: string): Promise<Blob> {
    const url = `${this.baseUrl}/api/sessions/${id}/logs`;
    const headers = this.buildHeaders();

    const response = await fetch(url, { headers });

    if (response.status === 401) {
      this.clearToken();
      this.onUnauthorized?.();
      throw await this.parseErrorResponse(response);
    }

    if (!response.ok) {
      throw await this.parseErrorResponse(response);
    }

    return response.blob();
  }

  /**
   * Get WebSocket URL for attaching to a session
   * 
   * Returns the full WebSocket URL including auth token if available
   */
  getWebSocketUrl(sessionId: string): string {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = this.baseUrl || window.location.host;
    const baseWsUrl = `${protocol}//${host}/api/sessions/${sessionId}/attach`;
    
    const token = this.getToken();
    if (token) {
      return `${baseWsUrl}?token=${encodeURIComponent(token)}`;
    }
    
    return baseWsUrl;
  }
}

// ============================================
// Default Client Instance
// ============================================

/**
 * Default API client instance for use throughout the application
 */
export const apiClient = new ApiClient();

// ============================================
// Convenience Functions
// ============================================

/**
 * Create a new terminal session
 */
export async function createSession(request: CreateSessionRequest): Promise<Session> {
  return apiClient.createSession(request);
}

/**
 * Get list of all sessions
 */
export async function listSessions(): Promise<Session[]> {
  return apiClient.listSessions();
}

/**
 * Get details of a specific session
 */
export async function getSession(id: string): Promise<Session> {
  return apiClient.getSession(id);
}

/**
 * Delete a session
 */
export async function deleteSession(id: string): Promise<void> {
  return apiClient.deleteSession(id);
}

/**
 * Restart an exited session
 */
export async function restartSession(id: string): Promise<Session> {
  return apiClient.restartSession(id);
}

/**
 * Download session logs
 */
export async function downloadSessionLogs(id: string): Promise<Blob> {
  return apiClient.downloadSessionLogs(id);
}

/**
 * Get WebSocket URL for a session
 */
export function getWebSocketUrl(sessionId: string): string {
  return apiClient.getWebSocketUrl(sessionId);
}

/**
 * Set authentication token
 */
export function setAuthToken(token: string): void {
  apiClient.setToken(token);
}

/**
 * Clear authentication token
 */
export function clearAuthToken(): void {
  apiClient.clearToken();
}

/**
 * Check if user is authenticated
 */
export function isAuthenticated(): boolean {
  return apiClient.isAuthenticated();
}

export default apiClient;
