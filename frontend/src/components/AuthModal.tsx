import React, { useState } from 'react';
import { X, Lock, Mail, ArrowRight } from 'lucide-react';
import { api } from '../api/client';
import { isGoogleConfigured, startGoogleLogin } from '../api/googleAuth';
import { useToast } from '../context/ToastContext';

interface AuthModalProps {
    isOpen: boolean;
    onClose: () => void;
    onSuccess: () => void;
}

export const AuthModal: React.FC<AuthModalProps> = ({ isOpen, onClose, onSuccess }) => {
    const [isLogin, setIsLogin] = useState(true);
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [loading, setLoading] = useState(false);
    const { showToast } = useToast();

    const handleGoogleLogin = () => {
        try {
            // Leaves the page; the callback is picked up on the next load.
            startGoogleLogin();
        } catch {
            showToast('Google sign-in is not configured', 'error');
        }
    };

    if (!isOpen) return null;

    const handleSubmit = async (e: React.SyntheticEvent) => {
        e.preventDefault();

        if (!email.trim()) {
            showToast('Please enter your email', 'error');
            return;
        }

        if (password.length < 6) {
            showToast('Password must be at least 6 characters', 'error');
            return;
        }

        setLoading(true);
        const endpoint = isLogin ? '/auth/login' : '/auth/register';

        try {
            const { data } = await api.post(endpoint, { email, password });

            if (isLogin) {
                localStorage.setItem('access_token', data.access_token);
                localStorage.setItem('refresh_token', data.refresh_token);
                showToast('Welcome back! Login successful.', 'success');
                onSuccess();
                onClose();
            } else {
                setIsLogin(true);
                showToast('Account created! You can now log in.', 'success');
            }
        } catch (err: any) {
            const msg = err.response?.data?.error || 'Invalid email or password';
            showToast(msg, 'error');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-zinc-950/80 p-0 sm:p-4 backdrop-blur-md">
            <div className="w-full max-w-md rounded-t-3xl sm:rounded-2xl border border-zinc-800 bg-zinc-900 p-6 shadow-2xl">
                <div className="flex items-center justify-between border-b border-zinc-800 pb-4">
                    <h2 className="text-sm font-bold text-zinc-100 font-mono uppercase tracking-wider">
                        {isLogin ? 'Sign In' : 'Sign Up'}
                    </h2>
                    <button onClick={onClose} className="rounded-lg p-1 text-zinc-400 hover:bg-zinc-800">
                        <X className="h-4 w-4" />
                    </button>
                </div>

                {isGoogleConfigured() && (
                <button
                    type="button"
                    disabled={loading}
                    onClick={handleGoogleLogin}
                    className="mt-6 flex w-full items-center justify-center gap-3 rounded-xl border border-zinc-800 bg-zinc-950/60 py-2.5 text-xs font-semibold text-zinc-200 hover:bg-zinc-800 hover:border-zinc-700 transition disabled:opacity-50"
                >
                    <svg className="h-4 w-4" viewBox="0 0 24 24">
                        <path fill="#EA4335" d="M12 5c1.6 0 3 .6 4.1 1.6l3.1-3.1C17.3 1.7 14.8 1 12 1 7.5 1 3.7 3.6 1.9 7.3l3.7 2.9C6.5 7.3 9 5 12 5z" />
                        <path fill="#4285F4" d="M23.5 12.3c0-.8-.1-1.6-.2-2.3H12v4.5h6.5c-.3 1.5-1.1 2.8-2.4 3.7l3.7 2.9c2.2-2 3.7-5 3.7-8.8z" />
                        <path fill="#FBBC05" d="M5.6 14.8c-.2-.7-.4-1.5-.4-2.3s.2-1.6.4-2.3L1.9 7.3C.7 9.7 0 10.8 0 12s.7 2.3 1.9 4.7l3.7-2.9z" />
                        <path fill="#34A853" d="M12 23c3.2 0 6-1.1 8-3l-3.7-2.9c-1.1.7-2.5 1.2-4.3 1.2-3 0-5.5-2.3-6.4-5.2L1.9 16C3.7 19.7 7.5 23 12 23z" />
                    </svg>
                    <span>Continue with Google</span>
                </button>
                )}

                <div className="my-5 flex items-center gap-3 text-[10px] font-mono text-zinc-600 uppercase tracking-wider">
                    <div className="h-px flex-1 bg-zinc-800" />
                    <span>or</span>
                    <div className="h-px flex-1 bg-zinc-800" />
                </div>

                <form onSubmit={handleSubmit} noValidate className="space-y-4">
                    <div>
                        <label className="block text-[10px] font-mono text-zinc-400 uppercase tracking-wider mb-1">
                            Email
                        </label>
                        <div className="relative">
                            <Mail className="absolute left-3.5 top-3 h-4 w-4 text-zinc-500" />
                            <input
                                type="email"
                                value={email}
                                onChange={(e) => setEmail(e.target.value)}
                                placeholder="name@example.com"
                                className="w-full rounded-xl border border-zinc-800 bg-zinc-950 pl-10 pr-4 py-2.5 text-sm text-zinc-100 outline-none transition focus:ring-1 focus:ring-emerald-500"
                            />
                        </div>
                    </div>

                    <div>
                        <label className="block text-[10px] font-mono text-zinc-400 uppercase tracking-wider mb-1">
                            Password
                        </label>
                        <div className="relative">
                            <Lock className="absolute left-3.5 top-3 h-4 w-4 text-zinc-500" />
                            <input
                                type="password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                placeholder="••••••••"
                                className="w-full rounded-xl border border-zinc-800 bg-zinc-950 pl-10 pr-4 py-2.5 text-sm text-zinc-100 outline-none transition focus:ring-1 focus:ring-emerald-500"
                            />
                        </div>
                    </div>

                    <button
                        type="submit"
                        disabled={loading}
                        className="flex w-full items-center justify-center gap-2 rounded-xl bg-emerald-500 py-3 text-xs font-bold text-zinc-950 shadow-lg shadow-emerald-500/10 hover:bg-emerald-400 disabled:opacity-50 transition"
                    >
                        <span>{loading ? 'Loading...' : isLogin ? 'Sign In' : 'Create Account'}</span>
                        <ArrowRight className="h-4 w-4 stroke-[2.5]" />
                    </button>
                </form>

                <div className="mt-6 text-center text-xs text-zinc-500">
                    {isLogin ? "Don't have an account?" : 'Already have an account?'}{' '}
                    <button
                        onClick={() => setIsLogin(!isLogin)}
                        className="font-semibold text-emerald-400 hover:underline"
                    >
                        {isLogin ? 'Sign Up' : 'Sign In'}
                    </button>
                </div>
            </div>
        </div>
    );
};