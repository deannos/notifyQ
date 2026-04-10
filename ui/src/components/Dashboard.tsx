import { useState, useCallback } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { useAuth } from '@/context/AuthContext';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useHealthCheck, type HealthStatus } from '@/hooks/useHealthCheck';
import type { Notification } from '@/types';
import { NotificationPanel } from './NotificationPanel';
import { AppPanel } from './AppPanel';
import { UserPanel } from './UserPanel';
import { MagneticButton } from './MagneticButton';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

type Panel = 'notifications' | 'apps' | 'users';

const panelVariants = {
  initial: { opacity: 0, y: 12 },
  animate: { opacity: 1, y: 0, transition: { duration: 0.28, ease: [0.23, 1, 0.32, 1] as const } },
  exit:    { opacity: 0, y: -8, transition: { duration: 0.16 } },
};

const healthMeta: Record<HealthStatus, { label: string; dot: string }> = {
  ok:       { label: 'Server healthy',            dot: 'bg-emerald-500' },
  degraded: { label: 'Degraded — DB unavailable', dot: 'bg-red-500'     },
  unknown:  { label: 'Checking…',                 dot: 'bg-zinc-500'    },
};

const navLabels: Record<Panel, string> = {
  notifications: 'Notifications',
  apps: 'Applications',
  users: 'Users',
};

export function Dashboard() {
  const { user, token, logout } = useAuth();
  const [panel, setPanel] = useState<Panel>('notifications');
  const [liveNotif, setLiveNotif] = useState<Notification | null>(null);

  const handleIncoming = useCallback((n: Notification) => setLiveNotif(n), []);
  const wsStatus = useWebSocket(token, handleIncoming);
  const health   = useHealthCheck();

  const navItems: Panel[] = [
    'notifications',
    'apps',
    ...(user?.is_admin ? ['users' as Panel] : []),
  ];

  return (
    <TooltipProvider>
      <div className="flex flex-col h-screen bg-background text-foreground antialiased">

        {/* ── Topbar ── */}
        <header className="flex items-center gap-6 px-6 h-[52px] bg-card/60 border-b border-border backdrop-blur-md sticky top-0 z-50 shrink-0">

          {/* Brand */}
          <div className="flex items-center gap-2 shrink-0">
            <motion.span
              className="text-lg"
              animate={{ rotate: [0, -12, 12, -8, 8, 0] }}
              transition={{ duration: 0.6, repeat: Infinity, repeatDelay: 5 }}
            >🔔</motion.span>
            <span className="text-sm font-semibold text-primary tracking-tight">NotifyQ</span>
          </div>

          {/* Tab navigation */}
          <nav className="flex items-center gap-0.5">
            {navItems.map(id => (
              <motion.button
                key={id}
                onClick={() => setPanel(id)}
                className={`relative px-3 py-1.5 text-sm rounded-md transition-colors duration-150 ${
                  panel === id
                    ? 'text-foreground font-medium'
                    : 'text-muted-foreground hover:text-foreground'
                }`}
                whileTap={{ scale: 0.97 }}
              >
                {panel === id && (
                  <motion.span
                    layoutId="tab-bg"
                    className="absolute inset-0 rounded-md bg-accent"
                    transition={{ type: 'spring', stiffness: 500, damping: 35 }}
                  />
                )}
                <span className="relative z-10">{navLabels[id]}</span>
              </motion.button>
            ))}
          </nav>

          {/* Right controls */}
          <div className="ml-auto flex items-center gap-4">

            {/* Health */}
            <Tooltip>
              <TooltipTrigger asChild>
                <motion.div
                  className="flex items-center gap-1.5 cursor-default"
                  animate={health === 'degraded' ? { opacity: [1, 0.4, 1] } : {}}
                  transition={{ duration: 1.2, repeat: Infinity }}
                >
                  <span className={`w-1.5 h-1.5 rounded-full ${healthMeta[health].dot} ${health === 'ok' ? 'shadow-[0_0_6px_theme(colors.emerald.500)]' : ''}`} />
                  <span className="text-xs text-muted-foreground hidden sm:inline">
                    {health === 'ok' ? 'Healthy' : health === 'degraded' ? 'Degraded' : '…'}
                  </span>
                </motion.div>
              </TooltipTrigger>
              <TooltipContent side="bottom">{healthMeta[health].label}</TooltipContent>
            </Tooltip>

            {/* WS */}
            <Tooltip>
              <TooltipTrigger asChild>
                <motion.span
                  className={`w-1.5 h-1.5 rounded-full cursor-default ${
                    wsStatus === 'connected'
                      ? 'bg-emerald-400 shadow-[0_0_6px_theme(colors.emerald.400)]'
                      : 'bg-zinc-600'
                  }`}
                  animate={wsStatus === 'connected' ? { scale: [1, 1.4, 1] } : {}}
                  transition={{ duration: 2.5, repeat: Infinity }}
                />
              </TooltipTrigger>
              <TooltipContent side="bottom">WebSocket {wsStatus}</TooltipContent>
            </Tooltip>

            {/* User */}
            <div className="flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-secondary border border-border text-xs text-muted-foreground">
              <span className="w-4 h-4 rounded-full bg-primary/20 flex items-center justify-center text-[10px] text-primary font-bold">
                {user?.username?.[0]?.toUpperCase()}
              </span>
              <span className="hidden sm:inline">{user?.username}</span>
            </div>

            <MagneticButton variant="ghost" size="sm" onClick={logout}
              className="text-muted-foreground hover:text-foreground text-xs px-2.5"
            >
              Logout
            </MagneticButton>
          </div>
        </header>

        {/* ── Main content ── */}
        <main className="flex-1 overflow-y-auto">
          <div className="max-w-5xl mx-auto px-6 py-8">
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
          </div>
        </main>

      </div>
    </TooltipProvider>
  );
}
