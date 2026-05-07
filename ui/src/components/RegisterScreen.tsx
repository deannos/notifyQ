import { type FormEvent, useState } from 'react';
import { motion } from 'motion/react';
import { api } from '@/api';
import { MagneticButton } from './MagneticButton';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

interface Props { onLogin: () => void; }

export function RegisterScreen({ onLogin }: Props) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    try {
      await api.post('/auth/register', { username, password });
      onLogin();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  return (
    <div className="flex items-center justify-center min-h-screen p-5 bg-background">
      <motion.div
        initial={{ opacity: 0, scale: 0.95, y: 20 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.45, ease: [0.23, 1, 0.32, 1] }}
        className="w-full max-w-sm"
      >
        <Card className="border-border bg-card shadow-2xl">
          <CardHeader className="text-center pb-2">
            <motion.div
              className="text-4xl mb-2 inline-block"
              animate={{ y: [0, -10, -5, 0] }}
              transition={{ duration: 4, repeat: Infinity, ease: 'easeInOut', times: [0, 0.33, 0.66, 1] }}
            >
              🔔
            </motion.div>
            <h1 className="text-2xl font-bold text-primary">NotifyQ</h1>
            <p className="text-sm text-muted-foreground">Create your account</p>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <Label htmlFor="reg-username">Username</Label>
                <Input id="reg-username" placeholder="Choose a username" value={username} onChange={e => setUsername(e.target.value)} required />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="reg-password">Password</Label>
                <Input id="reg-password" type="password" placeholder="Min 6 characters" value={password} onChange={e => setPassword(e.target.value)} required />
              </div>
              {error && <p className="text-destructive text-xs">{error}</p>}
              <MagneticButton type="submit" className="w-full">Create Account</MagneticButton>
            </form>
            <p className="text-center mt-4 text-xs text-muted-foreground">
              Already have an account?{' '}
              <a href="#" onClick={e => { e.preventDefault(); onLogin(); }} className="text-primary hover:underline">Sign In</a>
            </p>
          </CardContent>
        </Card>
      </motion.div>
    </div>
  );
}
