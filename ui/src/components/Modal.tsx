import { type ReactNode } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
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
    <AnimatePresence>
      {open && (
        <motion.div
          className="modal-overlay"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.2 }}
          onClick={(e) => { if (e.target === e.currentTarget) onCancel(); }}
        >
          <motion.div
            className="modal"
            initial={{ opacity: 0, scale: 0.88, y: 24 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.92, y: 16 }}
            transition={{ duration: 0.35, ease: [0.23, 1, 0.32, 1] }}
          >
            <h3>{title}</h3>
            {children}
            <div className="modal-actions">
              <MagneticButton onClick={onCancel}>Cancel</MagneticButton>
              {onConfirm && (
                <MagneticButton variant="primary" onClick={onConfirm} disabled={confirmDisabled}>
                  {confirmLabel}
                </MagneticButton>
              )}
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}
