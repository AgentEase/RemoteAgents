import { useEffect, useRef, useCallback } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

export interface TerminalProps {
  sessionId: string;
  onData?: (data: string) => void;
  onResize?: (rows: number, cols: number) => void;
  onReady?: (terminal: XTerm) => void;
}

export function Terminal({ sessionId, onData, onResize, onReady }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const resizeObserverRef = useRef<ResizeObserver | null>(null);

  // Handle terminal resize
  const handleResize = useCallback(() => {
    if (fitAddonRef.current && terminalRef.current) {
      try {
        fitAddonRef.current.fit();
        const { rows, cols } = terminalRef.current;
        onResize?.(rows, cols);
      } catch {
        // Ignore resize errors during initialization
      }
    }
  }, [onResize]);

  // Initialize terminal
  useEffect(() => {
    if (!containerRef.current) return;

    const terminal = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#d4d4d4',
        cursorAccent: '#1e1e1e',
        selectionBackground: '#264f78',
      },
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);

    terminal.open(containerRef.current);
    fitAddon.fit();

    terminalRef.current = terminal;
    fitAddonRef.current = fitAddon;

    // Handle user input
    terminal.onData((data) => {
      onData?.(data);
    });

    // Notify parent that terminal is ready
    onReady?.(terminal);

    // Initial resize notification
    onResize?.(terminal.rows, terminal.cols);

    return () => {
      terminal.dispose();
      terminalRef.current = null;
      fitAddonRef.current = null;
    };
  }, [sessionId, onData, onResize, onReady]);

  // Setup resize observer for auto-fit
  useEffect(() => {
    if (!containerRef.current) return;

    const resizeObserver = new ResizeObserver(() => {
      handleResize();
    });

    resizeObserver.observe(containerRef.current);
    resizeObserverRef.current = resizeObserver;

    // Also listen to window resize
    window.addEventListener('resize', handleResize);

    return () => {
      resizeObserver.disconnect();
      window.removeEventListener('resize', handleResize);
      resizeObserverRef.current = null;
    };
  }, [handleResize]);

  // Method to write data to terminal (exposed via ref or callback)
  const write = useCallback((data: string) => {
    terminalRef.current?.write(data);
  }, []);

  // Method to clear terminal
  const clear = useCallback(() => {
    terminalRef.current?.clear();
  }, []);

  // Expose methods via a custom hook pattern
  useEffect(() => {
    // Store methods on the container element for external access
    if (containerRef.current) {
      (containerRef.current as HTMLDivElement & { terminalApi?: TerminalApi }).terminalApi = {
        write,
        clear,
        focus: () => terminalRef.current?.focus(),
        getTerminal: () => terminalRef.current,
      };
    }
  }, [write, clear]);

  return (
    <div
      ref={containerRef}
      className="w-full h-full min-h-[300px] bg-[#1e1e1e]"
      data-session-id={sessionId}
    />
  );
}

export interface TerminalApi {
  write: (data: string) => void;
  clear: () => void;
  focus: () => void;
  getTerminal: () => XTerm | null;
}

export default Terminal;
