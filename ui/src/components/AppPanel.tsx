import { useEffect, useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { api } from '@/api';
import type { App } from '@/types';
import { MagneticButton } from './MagneticButton';
import { Modal } from './Modal';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { KeyRoundIcon, Trash2Icon } from 'lucide-react';

const listItem = {
  hidden: { opacity: 0, y: 18 },
  show: { opacity: 1, y: 0, transition: { ease: [0.23, 1, 0.32, 1] as const, duration: 0.35 } },
  exit: { opacity: 0, x: -20, transition: { duration: 0.2 } },
};

export function AppPanel() {
  const [apps, setApps] = useState<App[]>([]);
  const [showModal, setShowModal] = useState(false);
  const [name, setName] = useState('');
  const [desc, setDesc] = useState('');
  const [newToken, setNewToken] = useState('');
  const [created, setCreated] = useState(false);
  const [error, setError] = useState('');

  const load = async () => {
    try {
      const data = await api.get<App[]>('/api/v1/application');
      setApps(data ?? []);
    } catch { /* ignore */ }
  };

  useEffect(() => { void load(); }, []);

  const openModal = () => {
    setName(''); setDesc(''); setNewToken(''); setCreated(false); setError('');
    setShowModal(true);
  };

  const createApp = async () => {
    if (!name.trim()) { setError('Name is required'); return; }
    try {
      const app = await api.post<App>('/api/v1/application', { name: name.trim(), description: desc });
      setNewToken(app.token ?? '');
      setCreated(true);
      void load();
    } catch (err) { setError((err as Error).message); }
  };

  const deleteApp = async (id: string) => {
    if (!confirm('Delete this application and all its notifications?')) return;
    await api.del(`/api/v1/application/${id}`);
    setApps(prev => prev.filter(a => a.id !== id));
  };

  const rotateToken = async (id: string) => {
    if (!confirm('Rotate the token? The old token will stop working immediately.')) return;
    const data = await api.post<{ token: string }>(`/api/v1/application/${id}/token`);
    alert(`New token (save this — it won't be shown again):\n\n${data.token}`);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold tracking-tight">Applications</h2>
        <MagneticButton size="sm" onClick={openModal}>+ New App</MagneticButton>
      </div>

      {apps.length === 0 && (
        <p className="text-center py-10 text-muted-foreground">No applications yet. Create one to start sending notifications.</p>
      )}

      <motion.div
        className="space-y-2.5"
        initial="hidden"
        animate="show"
        variants={{ hidden: {}, show: { transition: { staggerChildren: 0.06 } } }}
      >
        <AnimatePresence initial={false}>
          {apps.map(a => (
            <motion.div key={a.id} variants={listItem} exit="exit" layout>
              <Card className="border-0 bg-card card-glow hover:bg-accent/60 transition-all duration-200">
                <CardContent className="p-4 flex items-center gap-4">
                  <div className="w-9 h-9 rounded-lg bg-primary/15 border border-primary/20 flex items-center justify-center shrink-0">
                    <KeyRoundIcon className="w-4 h-4 text-primary" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="font-semibold text-sm">{a.name}</p>
                    {a.description && <p className="text-muted-foreground text-xs">{a.description}</p>}
                    <p className="text-muted-foreground text-xs font-mono mt-0.5">ID: {a.id}</p>
                  </div>
                  <div className="flex gap-2 shrink-0">
                    <MagneticButton variant="outline" size="sm" onClick={() => void rotateToken(a.id)}>Rotate Token</MagneticButton>
                    <MagneticButton variant="destructive" size="sm" onClick={() => void deleteApp(a.id)}>
                      <Trash2Icon className="w-3.5 h-3.5" />
                    </MagneticButton>
                  </div>
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </AnimatePresence>
      </motion.div>

      <Modal
        open={showModal}
        title="Create Application"
        onCancel={() => setShowModal(false)}
        onConfirm={created ? undefined : () => void createApp()}
        confirmDisabled={created}
      >
        {!created ? (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>Name</Label>
              <Input placeholder="My Service" value={name} onChange={e => setName(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>Description</Label>
              <Input placeholder="What sends notifications here?" value={desc} onChange={e => setDesc(e.target.value)} />
            </div>
            {error && <p className="text-destructive text-xs">{error}</p>}
          </div>
        ) : (
          <motion.div
            className="rounded-lg border border-primary/30 bg-secondary p-4 space-y-2"
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ ease: [0.23, 1, 0.32, 1], duration: 0.4 }}
          >
            <p className="text-xs text-orange-400 font-medium">Save your token — it won't be shown again:</p>
            <code className="block text-xs font-mono text-emerald-400 break-all">{newToken}</code>
            <MagneticButton variant="outline" size="sm" onClick={() => void navigator.clipboard.writeText(newToken)}>Copy</MagneticButton>
          </motion.div>
        )}
      </Modal>
    </div>
  );
}
