import { useEffect, useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { toast } from 'sonner';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api';
import type { App, Notification } from '@/types';
import { MagneticButton } from './MagneticButton';
import { ConfirmDialog } from './ConfirmDialog';
import { Card, CardContent } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Input } from '@/components/ui/input';
import { CheckIcon, Trash2Icon, SearchIcon, XIcon } from 'lucide-react';
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select';

const LIMIT = 20;

const listItem = {
  hidden: { opacity: 0, y: 18 },
  show: { opacity: 1, y: 0, transition: { ease: [0.23, 1, 0.32, 1] as const, duration: 0.35 } },
  exit: { opacity: 0, x: -20, transition: { duration: 0.2 } },
};

interface Props {
  liveNotif: Notification | null;
  onLiveConsumed: () => void;
}

interface NotifPage {
  notifications: Notification[];
  total: number;
}

export function NotificationPanel({ liveNotif, onLiveConsumed }: Props) {
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const [search, setSearch] = useState('');
  const [filterApp, setFilterApp] = useState('');
  const [filterRead, setFilterRead] = useState('');
  const [filterPriority, setFilterPriority] = useState('');
  const [confirmDeleteAll, setConfirmDeleteAll] = useState(false);

  const notifKey = ['notifications', offset, search, filterApp, filterRead, filterPriority] as const;

  const buildUrl = (off: number) => {
    const p = new URLSearchParams({ limit: String(LIMIT), offset: String(off) });
    if (search)         p.set('q', search);
    if (filterApp)      p.set('app_id', filterApp);
    if (filterRead)     p.set('read', filterRead);
    if (filterPriority) p.set('priority', filterPriority);
    return `/api/v1/notification?${p.toString()}`;
  };

  // Reset to page 1 whenever filters change
  useEffect(() => { setOffset(0); }, [search, filterApp, filterRead, filterPriority]);

  const { data: appsData } = useQuery({
    queryKey: ['apps'],
    queryFn: () => api.get<App[]>('/api/v1/application'),
    staleTime: 60_000,
  });
  const apps = appsData ?? [];

  const { data: notifData, isLoading } = useQuery({
    queryKey: notifKey,
    queryFn: () => api.get<NotifPage>(buildUrl(offset)),
  });
  const notifs = notifData?.notifications ?? [];
  const total = notifData?.total ?? 0;

  // Inject live WebSocket notification into the current page cache
  useEffect(() => {
    if (!liveNotif) return;
    qc.setQueryData<NotifPage>(notifKey, prev =>
      prev
        ? { notifications: [liveNotif, ...prev.notifications], total: prev.total + 1 }
        : { notifications: [liveNotif], total: 1 }
    );
    onLiveConsumed();
  // notifKey is derived from state values captured at render; explicit deps below keep it fresh
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [liveNotif, qc, onLiveConsumed, offset, search, filterApp, filterRead, filterPriority]);

  const markReadMut = useMutation({
    mutationFn: (id: string) => api.put(`/api/v1/notification/${id}/read`),
    onMutate: (id) => {
      qc.setQueryData<NotifPage>(notifKey, prev =>
        prev ? { ...prev, notifications: prev.notifications.map(n => n.id === id ? { ...n, read: true } : n) } : prev
      );
    },
    onError: () => toast.error('Failed to mark as read'),
  });

  const deleteNotifMut = useMutation({
    mutationFn: (id: string) => api.del(`/api/v1/notification/${id}`),
    onMutate: (id) => {
      qc.setQueryData<NotifPage>(notifKey, prev =>
        prev ? { notifications: prev.notifications.filter(n => n.id !== id), total: Math.max(0, prev.total - 1) } : prev
      );
    },
    onSuccess: () => toast.success('Notification deleted'),
    onError: () => { toast.error('Failed to delete notification'); void qc.invalidateQueries({ queryKey: notifKey }); },
  });

  const markAllReadMut = useMutation({
    mutationFn: (ids: string[]) => Promise.all(ids.map(id => api.put(`/api/v1/notification/${id}/read`))),
    onMutate: (ids) => {
      const set = new Set(ids);
      qc.setQueryData<NotifPage>(notifKey, prev =>
        prev ? { ...prev, notifications: prev.notifications.map(n => set.has(n.id) ? { ...n, read: true } : n) } : prev
      );
    },
    onSuccess: (_, ids) => toast.success(`Marked ${ids.length} notification${ids.length > 1 ? 's' : ''} as read`),
    onError: () => toast.error('Failed to mark all as read'),
  });

  const deleteAllMut = useMutation({
    mutationFn: () => api.del('/api/v1/notification'),
    onSuccess: () => {
      qc.setQueryData<NotifPage>(notifKey, { notifications: [], total: 0 });
      setOffset(0);
      toast.success('All notifications deleted');
    },
    onError: () => toast.error('Failed to delete notifications'),
  });

  const unreadIds = notifs.filter(n => !n.read).map(n => n.id);
  const clearFilters = () => { setSearch(''); setFilterApp(''); setFilterRead(''); setFilterPriority(''); };
  const hasFilters = search || filterApp || filterRead || filterPriority;
  const pages = Math.ceil(total / LIMIT);
  const currentPage = Math.floor(offset / LIMIT) + 1;
  const unreadCount = notifs.filter(n => !n.read).length;

  const priorityBadge = (p: number) => {
    if (p >= 8) return <Badge variant="destructive">{p}</Badge>;
    if (p >= 4) return <Badge className="bg-orange-500/20 text-orange-400 border-orange-500/30">{p}</Badge>;
    return <Badge variant="secondary" className="text-emerald-400">{p}</Badge>;
  };

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">Notifications</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            <motion.span key={`t${total}`} initial={{ opacity: 0 }} animate={{ opacity: 1 }}>{total}</motion.span> total ·{' '}
            <motion.span key={`u${unreadCount}`} initial={{ opacity: 0 }} animate={{ opacity: 1 }} className="text-primary">{unreadCount} unread</motion.span>
          </p>
        </div>
        <div className="flex gap-2">
          <MagneticButton
            variant="ghost" size="sm"
            className="text-xs text-muted-foreground hover:text-foreground"
            disabled={!unreadIds.length || markAllReadMut.isPending}
            onClick={() => markAllReadMut.mutate(unreadIds)}
          >
            Mark all read
          </MagneticButton>
          <MagneticButton
            variant="ghost" size="sm"
            className="text-xs text-red-400 hover:text-red-300 hover:bg-red-500/10"
            onClick={() => setConfirmDeleteAll(true)}
          >
            Delete all
          </MagneticButton>
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 items-center">
        <div className="relative flex-1 min-w-[160px]">
          <SearchIcon className="absolute left-2.5 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground pointer-events-none" />
          <Input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search…"
            className="pl-8 h-8 text-sm bg-secondary border-0"
          />
        </div>

        <Select value={filterApp || '__all__'} onValueChange={v => setFilterApp(v === '__all__' ? '' : v)}>
          <SelectTrigger className="w-[120px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All apps</SelectItem>
            {apps.map(a => <SelectItem key={a.id} value={a.id}>{a.name}</SelectItem>)}
          </SelectContent>
        </Select>

        <Select value={filterRead || '__all__'} onValueChange={v => setFilterRead(v === '__all__' ? '' : v)}>
          <SelectTrigger className="w-[90px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">All</SelectItem>
            <SelectItem value="false">Unread</SelectItem>
            <SelectItem value="true">Read</SelectItem>
          </SelectContent>
        </Select>

        <Select value={filterPriority || '__all__'} onValueChange={v => setFilterPriority(v === '__all__' ? '' : v)}>
          <SelectTrigger className="w-[110px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="__all__">Any priority</SelectItem>
            {[...Array(11).keys()].map(p => <SelectItem key={p} value={String(p)}>{p}</SelectItem>)}
          </SelectContent>
        </Select>

        {hasFilters && (
          <motion.button
            onClick={clearFilters}
            className="h-8 px-2 text-xs rounded-md text-muted-foreground hover:text-foreground flex items-center gap-1 transition-colors"
            initial={{ opacity: 0, scale: 0.9 }} animate={{ opacity: 1, scale: 1 }}
          >
            <XIcon className="w-3 h-3" /> Clear
          </motion.button>
        )}
      </div>

      {/* List */}
      <ScrollArea className="h-[calc(100vh-280px)]">
        {isLoading && (
          <div className="space-y-2 pr-3">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="h-20 rounded-lg bg-card animate-pulse" />
            ))}
          </div>
        )}
        {!isLoading && notifs.length === 0 && (
          <motion.p className="text-center py-16 text-muted-foreground text-sm" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
            {hasFilters ? 'No notifications match your filters.' : 'No notifications yet.'}
          </motion.p>
        )}
        <motion.div
          className="space-y-2 pr-3"
          key={`${offset}-${search}-${filterApp}-${filterRead}-${filterPriority}`}
          initial="hidden"
          animate="show"
          variants={{ hidden: {}, show: { transition: { staggerChildren: 0.04 } } }}
        >
          <AnimatePresence initial={false}>
            {notifs.map(n => (
              <motion.div key={n.id} variants={listItem} exit="exit" layout transition={{ layout: { duration: 0.2 } }}>
                <Card className={`border-0 bg-card transition-all duration-200 hover:bg-accent/60 ${!n.read ? 'card-glow-amber' : 'card-glow'}`}>
                  <CardContent className="p-4 flex gap-3 items-start">
                    <div className="mt-0.5 shrink-0">{priorityBadge(n.priority)}</div>
                    <div className="flex-1 min-w-0">
                      <p className={`text-sm font-medium ${!n.read ? 'text-foreground' : 'text-muted-foreground'}`}>{n.title}</p>
                      <p className="text-muted-foreground text-xs mt-0.5 leading-relaxed">{n.message}</p>
                      <div className="flex gap-2 mt-2 items-center flex-wrap">
                        <span className="bg-primary/10 text-primary text-[10px] font-medium px-2 py-0.5 rounded-full">{n.app?.name ?? String(n.app_id)}</span>
                        <span className="text-[11px] text-muted-foreground">{new Date(n.created_at).toLocaleString()}</span>
                        {n.read && <span className="text-[11px] text-muted-foreground/60">✓</span>}
                      </div>
                    </div>
                    <div className="flex gap-1 shrink-0">
                      {!n.read && (
                        <motion.button
                          onClick={() => markReadMut.mutate(n.id)}
                          className="p-1.5 rounded-md text-muted-foreground hover:text-primary hover:bg-primary/10 transition-colors"
                          whileHover={{ scale: 1.1 }}
                          title="Mark read"
                        >
                          <CheckIcon className="w-3.5 h-3.5" />
                        </motion.button>
                      )}
                      <motion.button
                        onClick={() => deleteNotifMut.mutate(n.id)}
                        className="p-1 text-muted-foreground hover:text-destructive transition-colors"
                        whileHover={{ scale: 1.2 }}
                        title="Delete"
                      >
                        <Trash2Icon className="w-4 h-4" />
                      </motion.button>
                    </div>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </AnimatePresence>
        </motion.div>
      </ScrollArea>

      {/* Pagination */}
      {pages > 1 && (
        <div className="flex items-center justify-center gap-3">
          <MagneticButton variant="outline" size="sm" disabled={currentPage <= 1} onClick={() => setOffset(o => Math.max(0, o - LIMIT))}>← Prev</MagneticButton>
          <span className="text-sm text-muted-foreground">Page {currentPage} of {pages}</span>
          <MagneticButton variant="outline" size="sm" disabled={currentPage >= pages} onClick={() => setOffset(o => o + LIMIT)}>Next →</MagneticButton>
        </div>
      )}

      <ConfirmDialog
        open={confirmDeleteAll}
        title="Delete all notifications?"
        description="This will permanently delete every notification. This cannot be undone."
        confirmLabel="Delete all"
        onConfirm={() => deleteAllMut.mutate()}
        onCancel={() => setConfirmDeleteAll(false)}
      />
    </div>
  );
}
