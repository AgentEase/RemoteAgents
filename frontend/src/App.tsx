import { useState, useCallback } from 'react';
import { BrowserRouter, Routes, Route, useNavigate } from 'react-router-dom';
import { SessionList } from './pages/SessionList';
import { TerminalPage } from './pages/TerminalPage';
import { CreateSessionDialog } from './components/CreateSessionDialog';
import { createSession } from './api/client';
import type { CreateSessionRequest, Session } from './types';
import './App.css';

function AppContent() {
  const navigate = useNavigate();
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);

  const handleCreateNew = useCallback(() => {
    setIsCreateDialogOpen(true);
  }, []);

  const handleCloseDialog = useCallback(() => {
    setIsCreateDialogOpen(false);
  }, []);

  const handleCreateSession = useCallback(async (request: CreateSessionRequest) => {
    const session = await createSession(request);
    navigate(`/sessions/${session.id}`);
  }, [navigate]);

  const handleReconnect = useCallback(async (oldSession: Session) => {
    // Import restartSession dynamically
    const { restartSession } = await import('./api/client');
    
    // Restart the session (keeps same session ID)
    await restartSession(oldSession.id);
    
    // Navigate to the restarted session
    navigate(`/sessions/${oldSession.id}`);
  }, [navigate]);

  return (
    <>
      <Routes>
        <Route 
          path="/" 
          element={<SessionList onCreateNew={handleCreateNew} onReconnect={handleReconnect} />} 
        />
        <Route 
          path="/sessions/:id" 
          element={<TerminalPage />} 
        />
      </Routes>
      
      <CreateSessionDialog
        isOpen={isCreateDialogOpen}
        onClose={handleCloseDialog}
        onSubmit={handleCreateSession}
      />
    </>
  );
}

function App() {
  return (
    <BrowserRouter>
      <AppContent />
    </BrowserRouter>
  );
}

export default App;
