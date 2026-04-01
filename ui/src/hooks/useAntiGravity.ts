import { useMotionValue, useSpring } from 'framer-motion';
import type { MouseEvent } from 'react';

export function useAntiGravity(strength = 8) {
  const x = useMotionValue(0);
  const y = useMotionValue(0);
  const springX = useSpring(x, { stiffness: 300, damping: 20 });
  const springY = useSpring(y, { stiffness: 300, damping: 20 });

  function onMouseMove(e: MouseEvent<HTMLElement>) {
    const r = e.currentTarget.getBoundingClientRect();
    const nx = (e.clientX - (r.left + r.width / 2)) / (r.width / 2);
    const ny = (e.clientY - (r.top + r.height / 2)) / (r.height / 2);
    x.set(-nx * strength);
    y.set(-ny * strength * 0.6);
  }

  function onMouseLeave() {
    x.set(0);
    y.set(0);
  }

  return { x: springX, y: springY, onMouseMove, onMouseLeave };
}
