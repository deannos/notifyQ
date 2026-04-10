import { type ReactNode } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { MagneticButton } from './MagneticButton';

interface Props {
  open: boolean;
  title: string;
  onCancel: () => void;
  onConfirm?: () => void;
  confirmLabel?: string;
  confirmDisabled?: boolean;
  children: ReactNode;
}

export function Modal({ open, title, onCancel, onConfirm, confirmLabel = 'Create', confirmDisabled, children }: Props) {
  return (
    <Dialog open={open} onOpenChange={o => { if (!o) onCancel(); }}>
      <DialogContent className="bg-card border-border text-foreground sm:max-w-[440px]">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">{children}</div>
        <DialogFooter>
          <MagneticButton variant="outline" onClick={onCancel}>Cancel</MagneticButton>
          {onConfirm && (
            <MagneticButton onClick={onConfirm} disabled={confirmDisabled}>{confirmLabel}</MagneticButton>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
