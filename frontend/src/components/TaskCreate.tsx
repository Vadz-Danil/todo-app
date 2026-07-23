import React, { useState } from 'react';
import { Plus, Terminal } from 'lucide-react';

interface TaskCreateProps {
    onCreate: (title: string, description: string) => Promise<void>;
}

export const TaskCreate: React.FC<TaskCreateProps> = ({ onCreate }) => {
    const [title, setTitle] = useState('');
    const [description, setDescription] = useState('');
    const [isExpanded, setIsExpanded] = useState(false);
    const [loading, setLoading] = useState(false);

    const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        if (!title.trim()) return;

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
                isExpanded ? 'border-border bg-surface' : 'border-border bg-surface/80'
            }`}
        >
            <form onSubmit={handleSubmit}>
                <div className="flex items-center gap-3">
                    <Terminal className="h-4 w-4 text-success shrink-0" />
                    <input
                        type="text"
                        value={title}
                        onChange={(e) => setTitle(e.target.value)}
                        onFocus={() => setIsExpanded(true)}
                        placeholder="Create a new task..."
                        className="w-full bg-transparent text-xs sm:text-sm font-medium text-text-h outline-none"
                    />
                </div>

                {isExpanded && (
                    <div className="mt-3 space-y-3 border-t border-border/60 pt-3">
                        <textarea
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                            placeholder="Additional description or notes..."
                            rows={2}
                            className="w-full resize-none bg-transparent text-xs text-text outline-none"
                        />

                        <div className="flex items-center justify-end gap-2">
                            <button
                                type="button"
                                onClick={() => setIsExpanded(false)}
                                className="rounded-xl px-3 py-1.5 text-xs font-semibold text-text hover:text-text-h"
                            >
                                Cancel
                            </button>
                            <button
                                type="submit"
                                disabled={loading || !title.trim()}
                                className="flex items-center gap-1.5 rounded-xl bg-success px-4 py-1.5 text-xs font-bold text-bg shadow-md shadow-success/10 hover:bg-success/90 disabled:opacity-50 transition"
                            >
                                <Plus className="h-3.5 w-3.5" strokeWidth={3} />
                                <span>{loading ? 'Saving...' : 'Add Task'}</span>
                            </button>
                        </div>
                    </div>
                )}
            </form>
        </div>
    );
};