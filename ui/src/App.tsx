import { AnimatePresence, motion } from 'framer-motion';
import { useState, useEffect } from 'react';
import { Toaster } from 'sonner';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider, useAuth } from './context/AuthContext';
import { ThemeProvider, useTheme } from './context/ThemeContext';

const queryClient = new QueryClient();
import { LoginScreen } from './components/LoginScreen';
import { RegisterScreen } from './components/RegisterScreen';
import { Dashboard } from './components/Dashboard';

type Screen = 'login' | 'register' | 'dashboard';

const pageVariants = {
  initial: { opacity: 0, y: 28 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.4, ease: [0.23, 1, 0.32, 1] as const } },
  exit: { opacity: 0, y: -20, transition: { duration: 0.22 } },
};

function AppInner() {
  const { token, user } = useAuth();
  const [screen, setScreen] = useState<Screen>(() => (token && user ? 'dashboard' : 'login'));

  useEffect(() => {
    setScreen(token && user ? 'dashboard' : 'login');
  }, [token, user]);

  useEffect(() => {
    if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
      void Notification.requestPermission();
    }
  }, []);

  return (
    <AnimatePresence mode="wait">
      {screen === 'login' && (
        <motion.div key="login" {...pageVariants}>
          <LoginScreen onRegister={() => setScreen('register')} />
        </motion.div>
      )}
      {screen === 'register' && (
        <motion.div key="register" {...pageVariants}>
          <RegisterScreen onLogin={() => setScreen('login')} />
        </motion.div>
      )}
      {screen === 'dashboard' && (
        <motion.div key="dashboard" {...pageVariants} style={{ minHeight: '100vh' }}>
          <Dashboard />
        </motion.div>
      )}
    </AnimatePresence>
  );
}

function ThemedToaster() {
  const { theme } = useTheme();
  return <Toaster theme={theme} richColors position="bottom-right" closeButton />;
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthProvider>
          <AppInner />
        </AuthProvider>
        <ThemedToaster />
      </ThemeProvider>
    </QueryClientProvider>
  );
}
