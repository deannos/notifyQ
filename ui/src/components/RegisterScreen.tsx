import { type FormEvent, useState } from 'react';
import { motion } from 'framer-motion';
import { api } from '../api';
import { MagneticButton } from './MagneticButton';

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
    <div className="screen">
      <motion.div
        className="auth-card"
        initial={{ opacity: 0, scale: 0.95, y: 20 }}
        animate={{ opacity: 1, scale: 1, y: 0 }}
        transition={{ duration: 0.45, ease: [0.23, 1, 0.32, 1] }}
      >
        <div className="logo">
          <motion.span
            className="logo-icon"
            animate={{ y: [0, -10, -5, 0] }}
            transition={{ duration: 4, repeat: Infinity, ease: 'easeInOut', times: [0, 0.33, 0.66, 1] }}
          >🔔</motion.span>
          <h1>NotifyQ</h1>
          <p>Create your account</p>
        </div>
        <form onSubmit={handleSubmit}>
          <div className="field">
            <label>Username</label>
            <input type="text" placeholder="Choose a username" value={username} onChange={e => setUsername(e.target.value)} required />
          </div>
          <div className="field">
            <label>Password</label>
            <input type="password" placeholder="Min 6 characters" value={password} onChange={e => setPassword(e.target.value)} required />
          </div>
          {error && <div className="error-msg">{error}</div>}
          <MagneticButton variant="primary" block type="submit">Create Account</MagneticButton>
        </form>
        <p className="auth-switch">
          Already have an account?{' '}
          <a href="#" onClick={e => { e.preventDefault(); onLogin(); }}>Sign In</a>
        </p>
      </motion.div>
    </div>
  );
}
