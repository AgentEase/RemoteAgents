import { useState, useCallback, useEffect, useRef } from 'react';
import type { CreateSessionRequest } from '../types';

export interface CreateSessionDialogProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (request: CreateSessionRequest) => Promise<void>;
}

// Environment variable row component
function EnvVarRow({
  envKey,
  envValue,
  onChange,
  onRemove,
}: {
  envKey: string;
  envValue: string;
  onChange: (key: string, value: string) => void;
  onRemove: () => void;
}) {
  return (
    <div className="flex items-center gap-2">
      <input
        type="text"
        value={envKey}
        onChange={(e) => onChange(e.target.value, envValue)}
        placeholder="KEY"
        className="flex-1 px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500 font-mono text-sm"
      />
      <span className="text-gray-500">=</span>
      <input
        type="text"
        value={envValue}
        onChange={(e) => onChange(envKey, e.target.value)}
        placeholder="value"
        className="flex-1 px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500 font-mono text-sm"
      />
      <button
        type="button"
        onClick={onRemove}
        className="p-2 text-gray-400 hover:text-red-400 hover:bg-red-500/10 rounded transition-colors"
        title="Remove"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  );
}


// Preset session types
const SESSION_PRESETS = [
  {
    id: 'claude',
    name: 'Claude Code',
    icon: '🤖',
    command: 'claude',
    description: 'AI coding assistant with smart UI',
    color: 'from-purple-600 to-blue-600',
    supportsResume: true,
  },
  {
    id: 'bash',
    name: 'Bash',
    icon: '💻',
    command: 'bash',
    description: 'Standard Unix shell',
    color: 'from-gray-600 to-gray-700',
    supportsResume: false,
  },
  {
    id: 'python',
    name: 'Python',
    icon: '🐍',
    command: 'python3',
    description: 'Python interactive shell',
    color: 'from-blue-600 to-cyan-600',
    supportsResume: false,
  },
  {
    id: 'custom',
    name: 'Custom',
    icon: '⚙️',
    command: '',
    description: 'Enter your own command',
    color: 'from-green-600 to-teal-600',
    supportsResume: false,
  },
];

export function CreateSessionDialog({ isOpen, onClose, onSubmit }: CreateSessionDialogProps) {
  const [selectedPreset, setSelectedPreset] = useState<string | null>(null);
  const [command, setCommand] = useState('');
  const [name, setName] = useState('');
  const [workdir, setWorkdir] = useState('');
  const [envVars, setEnvVars] = useState<Array<{ key: string; value: string }>>([]);
  const [useResume, setUseResume] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const commandInputRef = useRef<HTMLInputElement>(null);

  // Focus command input when dialog opens
  useEffect(() => {
    if (isOpen) {
      setTimeout(() => commandInputRef.current?.focus(), 100);
    }
  }, [isOpen]);

  // Reset form when dialog closes
  useEffect(() => {
    if (!isOpen) {
      setSelectedPreset(null);
      setCommand('');
      setName('');
      setWorkdir('');
      setEnvVars([]);
      setUseResume(false);
      setError(null);
    }
  }, [isOpen]);

  // Handle preset selection
  const handlePresetSelect = (presetId: string) => {
    const preset = SESSION_PRESETS.find(p => p.id === presetId);
    if (!preset) return;

    setSelectedPreset(presetId);
    setCommand(preset.command);
    setUseResume(false); // Reset resume option when changing preset
    
    // Auto-fill name based on preset
    if (presetId !== 'custom') {
      setName(preset.name);
    }
  };

  const handleAddEnvVar = useCallback(() => {
    setEnvVars((prev) => [...prev, { key: '', value: '' }]);
  }, []);

  const handleEnvVarChange = useCallback((index: number, key: string, value: string) => {
    setEnvVars((prev) => {
      const updated = [...prev];
      updated[index] = { key, value };
      return updated;
    });
  }, []);

  const handleRemoveEnvVar = useCallback((index: number) => {
    setEnvVars((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    
    if (!command.trim()) {
      setError('Command is required');
      return;
    }

    setIsSubmitting(true);
    setError(null);

    try {
      // Build environment variables object
      const env: Record<string, string> = {};
      for (const { key, value } of envVars) {
        if (key.trim()) {
          env[key.trim()] = value;
        }
      }

      // Build the final command - add --resume flag if selected
      let finalCommand = command.trim();
      if (useResume && selectedPreset === 'claude') {
        finalCommand = 'claude --resume';
      }

      const request: CreateSessionRequest = {
        command: finalCommand,
        name: name.trim() || command.trim().split(' ')[0],
        ...(workdir.trim() && { workdir: workdir.trim() }),
        ...(Object.keys(env).length > 0 && { env }),
      };

      await onSubmit(request);
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create session');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose();
    }
  };

  if (!isOpen) {
    return null;
  }

  return (
    <div 
      className="fixed inset-0 z-50 flex items-center justify-center"
      onKeyDown={handleKeyDown}
    >
      {/* Backdrop */}
      <div 
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={onClose}
      />
      
      {/* Dialog */}
      <div className="relative bg-gray-800 border border-gray-700 rounded-xl shadow-2xl w-full max-w-lg mx-4">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-700">
          <h2 className="text-lg font-semibold text-gray-100">Create New Session</h2>
          <button
            onClick={onClose}
            className="p-1 text-gray-400 hover:text-gray-200 hover:bg-gray-700 rounded transition-colors"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit}>
          <div className="px-6 py-4 space-y-4">
            {/* Error message */}
            {error && (
              <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-lg text-red-400 text-sm">
                {error}
              </div>
            )}

            {/* Preset selection */}
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-2">
                Session Type
              </label>
              <div className="grid grid-cols-2 gap-2">
                {SESSION_PRESETS.map((preset) => (
                  <button
                    key={preset.id}
                    type="button"
                    onClick={() => handlePresetSelect(preset.id)}
                    disabled={isSubmitting}
                    className={`relative p-3 rounded-lg border-2 transition-all text-left ${
                      selectedPreset === preset.id
                        ? 'border-blue-500 bg-blue-500/10'
                        : 'border-gray-600 bg-gray-700/50 hover:border-gray-500'
                    }`}
                  >
                    <div className="flex items-start gap-2">
                      <span className="text-2xl">{preset.icon}</span>
                      <div className="flex-1 min-w-0">
                        <div className="font-medium text-gray-100 text-sm">
                          {preset.name}
                        </div>
                        <div className="text-xs text-gray-400 mt-0.5">
                          {preset.description}
                        </div>
                      </div>
                    </div>
                    {selectedPreset === preset.id && (
                      <div className="absolute top-2 right-2">
                        <span className="text-blue-400 text-sm">✓</span>
                      </div>
                    )}
                  </button>
                ))}
              </div>
            </div>

            {/* Command input - show when custom is selected or a preset is chosen */}
            {selectedPreset && (
              <div>
                <label htmlFor="command" className="block text-sm font-medium text-gray-300 mb-1.5">
                  Command <span className="text-red-400">*</span>
                </label>
                <input
                  ref={commandInputRef}
                  id="command"
                  type="text"
                  value={useResume && selectedPreset === 'claude' ? 'claude --resume' : command}
                  onChange={(e) => setCommand(e.target.value)}
                  placeholder="e.g., claude, python3, bash"
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500 font-mono"
                  disabled={isSubmitting || (selectedPreset !== 'custom')}
                  readOnly={selectedPreset !== 'custom'}
                />
                <p className="mt-1 text-xs text-gray-500">
                  {selectedPreset === 'custom' 
                    ? 'Enter the CLI command to execute'
                    : 'Command is set by the selected preset'}
                </p>
              </div>
            )}

            {/* Resume option - only show for Claude preset */}
            {selectedPreset === 'claude' && (
              <div className="flex items-center gap-2 p-3 bg-purple-900/20 border border-purple-700/30 rounded-lg">
                <input
                  type="checkbox"
                  id="useResume"
                  checked={useResume}
                  onChange={(e) => setUseResume(e.target.checked)}
                  disabled={isSubmitting}
                  className="w-4 h-4 bg-gray-700 border-gray-600 rounded text-purple-600 focus:ring-purple-500 focus:ring-2"
                />
                <label htmlFor="useResume" className="flex-1 cursor-pointer">
                  <div className="text-sm font-medium text-purple-300">
                    🔄 Resume previous session
                  </div>
                  <div className="text-xs text-gray-400 mt-0.5">
                    Start with session selection menu (claude --resume)
                  </div>
                </label>
              </div>
            )}

            {/* Name input - show when a preset is selected */}
            {selectedPreset && (
              <div>
                <label htmlFor="name" className="block text-sm font-medium text-gray-300 mb-1.5">
                  Session Name
                </label>
                <input
                  id="name"
                  type="text"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Optional - defaults to preset name"
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500"
                  disabled={isSubmitting}
                />
              </div>
            )}

            {/* Working directory input - show when a preset is selected */}
            {selectedPreset && (
              <div>
                <label htmlFor="workdir" className="block text-sm font-medium text-gray-300 mb-1.5">
                  Working Directory
                </label>
                <input
                  id="workdir"
                  type="text"
                  value={workdir}
                  onChange={(e) => setWorkdir(e.target.value)}
                  placeholder="e.g., /home/user/project or ~/project"
                  className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded-lg text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500 font-mono text-sm"
                  disabled={isSubmitting}
                />
                <p className="mt-1 text-xs text-gray-500">
                  Optional - defaults to current directory if not specified
                </p>
              </div>
            )}

            {/* Environment variables - show when a preset is selected */}
            {selectedPreset && (
              <div>
                <div className="flex items-center justify-between mb-1.5">
                  <label className="block text-sm font-medium text-gray-300">
                    Environment Variables
                  </label>
                  <button
                    type="button"
                    onClick={handleAddEnvVar}
                    className="text-xs text-blue-400 hover:text-blue-300 transition-colors"
                    disabled={isSubmitting}
                  >
                    + Add Variable
                  </button>
                </div>
                
                {envVars.length === 0 ? (
                  <p className="text-xs text-gray-500">
                    No environment variables configured
                  </p>
                ) : (
                  <div className="space-y-2">
                    {envVars.map((env, index) => (
                      <EnvVarRow
                        key={index}
                        envKey={env.key}
                        envValue={env.value}
                        onChange={(key, value) => handleEnvVarChange(index, key, value)}
                        onRemove={() => handleRemoveEnvVar(index)}
                      />
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>

          {/* Footer */}
          <div className="flex items-center justify-end gap-3 px-6 py-4 border-t border-gray-700">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-gray-300 hover:text-gray-100 hover:bg-gray-700 rounded-lg transition-colors"
              disabled={isSubmitting}
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={isSubmitting || !command.trim() || !selectedPreset}
              className="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-blue-600/50 disabled:cursor-not-allowed text-white rounded-lg transition-colors flex items-center gap-2"
            >
              {isSubmitting ? (
                <>
                  <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                  </svg>
                  Creating...
                </>
              ) : (
                'Create Session'
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default CreateSessionDialog;
