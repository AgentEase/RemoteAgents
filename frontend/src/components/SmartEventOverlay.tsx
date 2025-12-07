import type { SmartEvent } from '../types';

export interface SmartEventOverlayProps {
  event: SmartEvent | null;
  onAction: (action: string) => void;
  onDismiss?: () => void;
}

export function SmartEventOverlay({ event, onAction, onDismiss }: SmartEventOverlayProps) {
  if (!event) {
    return null;
  }

  const handleOptionClick = (option: string) => {
    // Send the appropriate input based on the event kind and option
    let input: string;
    
    if (event.kind === 'claude_confirm') {
      // Claude Code confirmation menu: 1=Yes, 2=Yes allow all, esc=Cancel
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
          input = '\x1b'; // ESC key
          break;
        default:
          input = option;
      }
    } else if (event.kind === 'question') {
      // Standard y/n questions
      if (option.toLowerCase() === 'yes' || option.toLowerCase() === 'y') {
        input = 'y\n';
      } else if (option.toLowerCase() === 'no' || option.toLowerCase() === 'n') {
        input = 'n\n';
      } else {
        input = option + '\n';
      }
    } else {
      // Default: send option as-is with newline
      input = option + '\n';
    }
    
    onAction(input);
  };

  const renderContent = () => {
    switch (event.kind) {
      case 'question':
        return (
          <QuestionOverlay
            prompt={event.prompt}
            options={event.options}
            onOptionClick={handleOptionClick}
            onDismiss={onDismiss}
          />
        );
      case 'claude_confirm':
        return (
          <ClaudeConfirmOverlay
            prompt={event.prompt}
            options={event.options}
            onOptionClick={handleOptionClick}
            onDismiss={onDismiss}
          />
        );
      case 'idle':
        return (
          <IdleOverlay
            prompt={event.prompt}
            onDismiss={onDismiss}
          />
        );
      case 'progress':
        return (
          <ProgressOverlay
            prompt={event.prompt}
          />
        );
      default:
        return null;
    }
  };

  return (
    <div className="absolute top-0 left-0 right-0 z-10 pointer-events-none">
      <div className="pointer-events-auto">
        {renderContent()}
      </div>
    </div>
  );
}

interface ClaudeConfirmOverlayProps {
  prompt?: string;
  options?: string[];
  onOptionClick: (option: string) => void;
  onDismiss?: () => void;
}

function ClaudeConfirmOverlay({ prompt, options = [], onOptionClick, onDismiss }: ClaudeConfirmOverlayProps) {
  // Claude Code confirmation menu: 1=Yes, 2=Yes allow all, Esc=Cancel
  const getOptionLabel = (option: string) => {
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
  };

  const getOptionStyle = (option: string) => {
    switch (option.toLowerCase()) {
      case '1':
        return 'bg-green-600 hover:bg-green-500 text-white';
      case '2':
        return 'bg-blue-600 hover:bg-blue-500 text-white';
      case 'esc':
        return 'bg-gray-600 hover:bg-gray-500 text-white';
      default:
        return 'bg-blue-600 hover:bg-blue-500 text-white';
    }
  };

  return (
    <div className="bg-gradient-to-r from-purple-900/30 to-blue-900/30 border-b border-purple-500/50 p-3 shadow-lg backdrop-blur-sm">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <span className="text-purple-400 text-lg flex-shrink-0">🤖</span>
          <div className="flex-1 min-w-0">
            <div className="text-xs text-purple-300 font-medium mb-0.5">Claude Code</div>
            <div className="text-gray-200 text-sm truncate" title={prompt}>
              {prompt || 'Waiting for confirmation...'}
            </div>
          </div>
        </div>
        
        <div className="flex items-center gap-2 flex-shrink-0">
          {options.map((option, index) => (
            <button
              key={index}
              onClick={() => onOptionClick(option)}
              className={`px-4 py-1.5 rounded text-sm font-medium transition-colors ${getOptionStyle(option)}`}
              title={`Press ${option}`}
            >
              <span className="text-xs opacity-75 mr-1">{option}</span>
              {getOptionLabel(option)}
            </button>
          ))}
          
          {onDismiss && (
            <button
              onClick={onDismiss}
              className="px-2 py-1.5 text-gray-400 hover:text-gray-200 transition-colors"
              title="Dismiss"
            >
              ✕
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

interface QuestionOverlayProps {
  prompt?: string;
  options?: string[];
  onOptionClick: (option: string) => void;
  onDismiss?: () => void;
}

function QuestionOverlay({ prompt, options = [], onOptionClick, onDismiss }: QuestionOverlayProps) {
  return (
    <div className="bg-gray-800 border-b border-gray-600 p-3 shadow-lg">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <span className="text-yellow-400 text-lg flex-shrink-0">❓</span>
          <span className="text-gray-200 text-sm truncate" title={prompt}>
            {prompt || 'Waiting for your response...'}
          </span>
        </div>
        
        <div className="flex items-center gap-2 flex-shrink-0">
          {options.map((option, index) => (
            <button
              key={index}
              onClick={() => onOptionClick(option)}
              className={`px-4 py-1.5 rounded text-sm font-medium transition-colors ${
                option.toLowerCase() === 'yes' || option.toLowerCase() === 'y'
                  ? 'bg-green-600 hover:bg-green-500 text-white'
                  : option.toLowerCase() === 'no' || option.toLowerCase() === 'n'
                  ? 'bg-red-600 hover:bg-red-500 text-white'
                  : 'bg-blue-600 hover:bg-blue-500 text-white'
              }`}
            >
              {option}
            </button>
          ))}
          
          {onDismiss && (
            <button
              onClick={onDismiss}
              className="px-2 py-1.5 text-gray-400 hover:text-gray-200 transition-colors"
              title="Dismiss"
            >
              ✕
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

interface IdleOverlayProps {
  prompt?: string;
  onDismiss?: () => void;
}

function IdleOverlay({ prompt, onDismiss }: IdleOverlayProps) {
  return (
    <div className="bg-gray-800 border-b border-gray-600 p-3 shadow-lg">
      <div className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <span className="text-blue-400 text-lg flex-shrink-0">⏸</span>
          <span className="text-gray-200 text-sm truncate" title={prompt}>
            {prompt || 'Agent is waiting for input...'}
          </span>
        </div>
        
        {onDismiss && (
          <button
            onClick={onDismiss}
            className="px-2 py-1.5 text-gray-400 hover:text-gray-200 transition-colors flex-shrink-0"
            title="Dismiss"
          >
            ✕
          </button>
        )}
      </div>
    </div>
  );
}

interface ProgressOverlayProps {
  prompt?: string;
}

function ProgressOverlay({ prompt }: ProgressOverlayProps) {
  return (
    <div className="bg-gray-800 border-b border-gray-600 p-3 shadow-lg">
      <div className="flex items-center gap-3">
        <span className="text-green-400 text-lg animate-pulse">⚡</span>
        <span className="text-gray-200 text-sm">
          {prompt || 'Agent is working...'}
        </span>
        <div className="flex gap-1 ml-2">
          <span className="w-1.5 h-1.5 bg-green-400 rounded-full animate-bounce" style={{ animationDelay: '0ms' }} />
          <span className="w-1.5 h-1.5 bg-green-400 rounded-full animate-bounce" style={{ animationDelay: '150ms' }} />
          <span className="w-1.5 h-1.5 bg-green-400 rounded-full animate-bounce" style={{ animationDelay: '300ms' }} />
        </div>
      </div>
    </div>
  );
}

export default SmartEventOverlay;
