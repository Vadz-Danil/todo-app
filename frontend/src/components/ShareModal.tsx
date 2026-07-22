import React, { useState } from 'react';
import { Mail, Send, X } from 'lucide-react';
import { api } from '../api/client';
import { useToast } from '../context/ToastContext';

interface ShareModalProps {
    isOpen: boolean;
    onClose: () => void;
}

export const ShareModal: React.FC<ShareModalProps> = ({ isOpen, onClose }) => {
    const [email, setEmail] = useState('');
    const [loading, setLoading] = useState(false);
    const { showToast } = useToast();

    if (!isOpen) return null;

    const handleShare = async (e: React.SyntheticEvent) => {
        e.preventDefault();

        if (!email.trim()) {
            showToast('Please enter an email address', 'error');
            return;
        }

        const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
        if (!emailRegex.test(email)) {
            showToast('Please enter a valid email address', 'error');
            return;
        }

        setLoading(true);

        try {
            await api.post('/api/tasks/share', { recipient_email: email });
            showToast(`Task list sent to ${email}`, 'success');
            setEmail('');
            onClose();
        } catch (err: any) {
            const errorMsg = err.response?.data?.error || 'Failed to send task list';
            showToast(errorMsg, 'error');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center bg-zinc-950/80 p-0 sm:p-4 backdrop-blur-md">
            <div className="w-full max-w-md rounded-t-3xl sm:rounded-2xl border border-zinc-800 bg-zinc-900 p-6 shadow-2xl">
                <div className="flex items-center justify-between border-b border-zinc-800 pb-4">
                    <div className="flex items-center gap-2.5">
                        <div className="rounded-xl border border-zinc-700 bg-zinc-800 p-2 text-emerald-400">
                            <Mail className="h-4 w-4" />
                        </div>
                        <h2 className="text-sm font-bold text-zinc-100 font-mono uppercase">
                            Share Dashboard
                        </h2>
                    </div>
                    <button onClick={onClose} className="rounded-lg p-1 text-zinc-400 hover:bg-zinc-800">
                        <X className="h-4 w-4" />
                    </button>
                </div>

                <form onSubmit={handleShare} noValidate className="mt-4 space-y-4">
                    <p className="text-xs text-zinc-400 leading-relaxed">
                        Enter your colleague's email. We'll send your task list directly to their inbox.
                    </p>

                    <div>
                        <input
                            type="email"
                            value={email}
                            onChange={(e) => setEmail(e.target.value)}
                            placeholder="colleague@domain.com"
                            className="w-full rounded-xl border border-zinc-800 bg-zinc-950 px-4 py-2.5 text-sm text-zinc-100 outline-none transition focus:ring-1 focus:ring-emerald-500"
                        />
                    </div>

                    <button
                        type="submit"
                        disabled={loading}
                        className="flex w-full items-center justify-center gap-2 rounded-xl bg-emerald-500 py-2.5 text-xs font-bold text-zinc-950 shadow-lg shadow-emerald-500/10 hover:bg-emerald-400 disabled:opacity-50 transition"
                    >
                        <Send className="h-3.5 w-3.5" />
                        <span>{loading ? 'Sending...' : 'Send List'}</span>
                    </button>
                </form>
            </div>
        </div>
    );
};