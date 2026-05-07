import { motion } from 'motion/react';
import { Button, type ButtonProps } from '@/components/ui/button';
import { useAntiGravity } from '@/hooks/useAntiGravity';
import type { MouseEvent } from 'react';

interface Props extends ButtonProps {
  strength?: number;
}

export function MagneticButton({ strength = 8, children, onMouseMove, onMouseLeave, ...rest }: Props) {
  const { x, y, onMouseMove: agOnMouseMove, onMouseLeave: agOnMouseLeave } = useAntiGravity(strength);

  return (
    <motion.div style={{ x, y, display: 'inline-flex' }} whileTap={{ scale: 0.96 }}>
      <Button
        onMouseMove={(e: MouseEvent<HTMLButtonElement>) => {
          agOnMouseMove(e as MouseEvent<HTMLElement>);
          onMouseMove?.(e);
        }}
        onMouseLeave={(e: MouseEvent<HTMLButtonElement>) => {
          agOnMouseLeave();
          onMouseLeave?.(e);
        }}
        {...rest}
      >
        {children}
      </Button>
    </motion.div>
  );
}
