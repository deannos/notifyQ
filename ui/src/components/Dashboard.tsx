import { useState, useCallback } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { useAuth } from '../context/AuthContext';
import { useWebSocket } from '../hooks/useWebSocket';
import type { Notification } from '../types';
import { NotificationPanel } from './NotificationPanel';
import { AppPanel } from './AppPanel';
import { UserPanel } from './UserPanel';
import { MagneticButton } from './MagneticButton';

type Panel = 'notifications' | 'apps' | 'users';

const panelVariants = {
  initial: { opacity: 0, y: 14 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.3, ease: [0.23, 1, 0.32, 1] as const } },
  exit: { opacity: 0, y: -10, transition: { duration: 0.18 } },
};

export function Dashboard() {
  const { user, token, logout } = useAuth();
  const [panel, setPanel] = useState<Panel>('notifications');
  const [liveNotif, setLiveNotif] = useState<Notification | null>(null);

  const handleIncoming = useCallback((n: Notification) => setLiveNotif(n), []);
  const wsStatus = useWebSocket(token, handleIncoming);

  const navItems: { id: Panel; label: string }[] = [
    { id: 'notifications', label: '🔔 Notifications' },
    { id: 'apps', label: '🔑 Applications' },
    ...(user?.is_admin ? [{ id: 'users' as Panel, label: '👤 Users' }] : []),
  ];

  return (
    <>
      <header className="topbar">
        <div className="topbar-brand">
          <motion.span
            animate={{ rotate: [0, -15, 15, -10, 10, 0] }}
            transition={{ duration: 0.5, repeat: Infinity, repeatDelay: 4 }}
          >🔔</motion.span>
          <span>NotifyQ</span>
        </div>
        <div className="topbar-right">
          <span className="user-badge">👤 {user?.username}</span>
          <motion.span
            className={`ws-dot ${wsStatus === 'connected' ? 'connected' : 'disconnected'}`}
            title={`WebSocket ${wsStatus}`}
            animate={wsStatus === 'connected' ? { scale: [1, 1.3, 1], opacity: [1, 0.6, 1] } : {}}
            transition={{ duration: 2, repeat: Infinity }}
          />
          <MagneticButton size="sm" onClick={logout}>Logout</MagneticButton>
        </div>
      </header>

      <div className="layout">
        <aside className="sidebar">
          <nav>
            {navItems.map(item => (
              <motion.button
                key={item.id}
                className={`nav-item${panel === item.id ? ' active' : ''}`}
                onClick={() => setPanel(item.id)}
                whileHover={{ x: 4 }}
                transition={{ type: 'spring', stiffness: 400, damping: 25 }}
              >
                {item.label}
              </motion.button>
            ))}
          </nav>
        </aside>

        <main className="main">
          <AnimatePresence mode="wait">
            {panel === 'notifications' && (
              <motion.div key="notifications" {...panelVariants}>
                <NotificationPanel liveNotif={liveNotif} onLiveConsumed={() => setLiveNotif(null)} />
              </motion.div>
            )}
            {panel === 'apps' && (
              <motion.div key="apps" {...panelVariants}>
                <AppPanel />
              </motion.div>
            )}
            {panel === 'users' && (
              <motion.div key="users" {...panelVariants}>
                <UserPanel />
              </motion.div>
            )}
          </AnimatePresence>
        </main>
      </div>
    </>
  );
}
