import { useState } from 'react';
import { motion, AnimatePresence } from 'motion/react';
import { toast } from 'sonner';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@/api';
import type { App } from '@/types';
import { MagneticButton } from './MagneticButton';
import { Modal } from './Modal';
import { ConfirmDialog } from './ConfirmDialog';
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
  const qc = useQueryClient();
  const [showModal, setShowModal] = useState(false);
  const [name, setName] = useState('');
  const [desc, setDesc] = useState('');
  const [newToken, setNewToken] = useState('');
  const [created, setCreated] = useState(false);
  const [formError, setFormError] = useState('');
  const [rotatedToken, setRotatedToken] = useState('');
  const [deleteAppId, setDeleteAppId] = useState<string | null>(null);
  const [rotateTokenId, setRotateTokenId] = useState<string | null>(null);

  const { data: apps = [], isLoading } = useQuery({
    queryKey: ['apps'],
    queryFn: () => api.get<App[]>('/api/v1/application').then(d => d ?? []),
    staleTime: 30_000,
  });

  const createMut = useMutation({
    mutationFn: () => api.post<App>('/api/v1/application', { name: name.trim(), description: desc }),
    onSuccess: (app) => {
      setNewToken(app.token ?? '');
      setCreated(true);
      void qc.invalidateQueries({ queryKey: ['apps'] });
    },
    onError: (err) => setFormError((err as Error).message),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.del(`/api/v1/application/${id}`),
    onMutate: (id) => {
      qc.setQueryData<App[]>(['apps'], prev => prev?.filter(a => a.id !== id));
    },
    onSuccess: () => toast.success('Application deleted'),
    onError: () => { toast.error('Failed to delete application'); void qc.invalidateQueries({ queryKey: ['apps'] }); },
  });

  const rotateMut = useMutation({
    mutationFn: (id: string) => api.post<{ token: string }>(`/api/v1/application/${id}/token`),
    onSuccess: (data) => setRotatedToken(data.token),
    onError: () => toast.error('Failed to rotate token'),
  });

  const openModal = () => {
    setName(''); setDesc(''); setNewToken(''); setCreated(false); setFormError('');
    setShowModal(true);
  };

  const handleCreate = () => {
    if (!name.trim()) { setFormError('Name is required'); return; }
    createMut.mutate();
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold tracking-tight">Applications</h2>
        <MagneticButton size="sm" onClick={openModal}>+ New App</MagneticButton>
      </div>

      {/* Loading skeletons */}
      {isLoading && (
        <div className="space-y-2.5">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-[72px] rounded-lg bg-card animate-pulse" />
          ))}
        </div>
      )}

      {/* Empty state */}
      {!isLoading && apps.length === 0 && (
        <motion.div
          className="flex flex-col items-center py-20 gap-3 text-muted-foreground"
          initial={{ opacity: 0 }} animate={{ opacity: 1 }}
        >
          <KeyRoundIcon className="w-10 h-10 opacity-20" />
          <div className="text-center">
            <p className="text-sm font-medium">No applications yet</p>
            <p className="text-xs mt-1 text-muted-foreground/70">Create one to start sending notifications.</p>
          </div>
          <MagneticButton size="sm" onClick={openModal}>+ New App</MagneticButton>
        </motion.div>
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
                    <MagneticButton variant="outline" size="sm" onClick={() => setRotateTokenId(a.id)}>Rotate Token</MagneticButton>
                    <MagneticButton variant="destructive" size="sm" onClick={() => setDeleteAppId(a.id)}>
                      <Trash2Icon className="w-3.5 h-3.5" />
                    </MagneticButton>
                  </div>
                </CardContent>
              </Card>
            </motion.div>
          ))}
        </AnimatePresence>
      </motion.div>

      <ConfirmDialog
        open={!!deleteAppId}
        title="Delete application?"
        description="This will permanently delete the app and all its notifications."
        confirmLabel="Delete"
        onConfirm={() => { deleteMut.mutate(deleteAppId!); }}
        onCancel={() => setDeleteAppId(null)}
      />

      <ConfirmDialog
        open={!!rotateTokenId}
        title="Rotate token?"
        description="The current token will stop working immediately."
        confirmLabel="Rotate"
        variant="default"
        onConfirm={() => { rotateMut.mutate(rotateTokenId!); }}
        onCancel={() => setRotateTokenId(null)}
      />

      <Modal
        open={!!rotatedToken}
        title="Token Rotated"
        onCancel={() => setRotatedToken('')}
      >
        <motion.div
          className="rounded-lg border border-primary/30 bg-secondary p-4 space-y-2"
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ ease: [0.23, 1, 0.32, 1], duration: 0.4 }}
        >
          <p className="text-xs text-orange-400 font-medium">Save your new token — it won't be shown again:</p>
          <code className="block text-xs font-mono text-emerald-400 break-all">{rotatedToken}</code>
          <MagneticButton variant="outline" size="sm" onClick={() => {
            void navigator.clipboard.writeText(rotatedToken);
            toast.success('Token copied to clipboard');
          }}>Copy</MagneticButton>
        </motion.div>
      </Modal>

      <Modal
        open={showModal}
        title="Create Application"
        onCancel={() => setShowModal(false)}
        onConfirm={created ? undefined : handleCreate}
        confirmDisabled={created || createMut.isPending}
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
            {formError && <p className="text-destructive text-xs">{formError}</p>}
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
