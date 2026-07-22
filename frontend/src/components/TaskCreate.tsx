import React, { useState } from 'react';
import { Plus, Terminal } from 'lucide-react';
import { useToast } from '../context/ToastContext';

interface TaskCreateProps {
    onCreate: (title: string, description: string) => Promise<void>;
}

export const TaskCreate: React.FC<TaskCreateProps> = ({ onCreate }) => {
    const [title, setTitle] = useState('');
    const [description, setDescription] = useState('');
    const [isExpanded, setIsExpanded] = useState(false);
    const [loading, setLoading] = useState(false);
    const { showToast } = useToast();

    const handleSubmit = async (e: React.SyntheticEvent) => {
        e.preventDefault();

        if (!title.trim()) {
            showToast('Please enter a task title', 'error');
            return;
        }

        setLoading(true);
        try {
            await onCreate(title.trim(), description.trim());
            setTitle('');
            setDescription('');
            setIsExpanded(false);
        } finally {
            setLoading(false);
        }
    };

    return (
        <div
            className={`rounded-2xl border p-3 sm:p-4 transition-all ${
                isExpanded
                    ? 'border-zinc-700 bg-zinc-900'
                    : 'border-zinc-800 bg-zinc-900/60'
            }`}
        >
            <form onSubmit={handleSubmit} noValidate>
                <div className="flex items-center gap-3">
                    <Terminal className="h-4 w-4 text-emerald-400 shrink-0" />
                    <input
                        type="text"
                        value={title}
                        onChange={(e) => setTitle(e.target.value)}
                        onFocus={() => setIsExpanded(true)}
                        placeholder="Create a new task..."
                        className="w-full bg-transparent text-xs sm:text-sm font-medium text-zinc-100 outline-none"
                    />
                </div>

                {isExpanded && (
                    <div className="mt-3 space-y-3 border-t border-zinc-800/80 pt-3">
                        <textarea
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                            placeholder="Additional description or notes..."
                            rows={2}
                            className="w-full resize-none bg-transparent text-xs text-zinc-300 outline-none"
                        />

                        <div className="flex items-center justify-end gap-2">
                            <button
                                type="button"
                                onClick={() => setIsExpanded(false)}
                                className="rounded-xl px-3 py-1.5 text-xs font-semibold text-zinc-400 hover:text-zinc-200"
                            >
                                Cancel
                            </button>
                            <button
                                type="submit"
                                disabled={loading}
                                className="flex items-center gap-1.5 rounded-xl bg-emerald-500 px-4 py-1.5 text-xs font-bold text-zinc-950 shadow-md shadow-emerald-500/10 hover:bg-emerald-400 disabled:opacity-50 transition"
                            >
                                <Plus className="h-3.5 w-3.5 stroke-3" />
                                <span>{loading ? 'Saving...' : 'Add Task'}</span>
                            </button>
                        </div>
                    </div>
                )}
            </form>
        </div>
    );
};