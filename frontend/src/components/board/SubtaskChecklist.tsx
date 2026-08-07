import React, { useEffect, useRef, useState } from 'react';
import { Check, Loader2, Plus, Trash2 } from 'lucide-react';
import {
    createSubtask,
    deleteSubtask,
    listSubtasks,
    updateSubtask,
} from '../../api/endpoints';
import type { Subtask } from '../../types';
import { useToast } from '../../context/ToastContext';

interface SubtaskChecklistProps {
    taskId: string;
    /** Bubbles the done/total count up so the modal header can show it live. */
    onCountChange?: (done: number, total: number) => void;
}

export const SubtaskChecklist: React.FC<SubtaskChecklistProps> = ({ taskId, onCountChange }) => {
    const [items, setItems] = useState<Subtask[]>([]);
    const [loading, setLoading] = useState(true);
    const [draft, setDraft] = useState('');
    const [adding, setAdding] = useState(false);
    // Ids with an in-flight request, so a row can show a spinner and block
    // double-submits without freezing the rest of the list.
    const [busy, setBusy] = useState<Set<string>>(new Set());
    const { showToast } = useToast();
    const inputRef = useRef<HTMLInputElement>(null);

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        listSubtasks(taskId)
            .then((rows) => {
                if (!cancelled) setItems(rows);
            })
            .catch(() => {
                if (!cancelled) showToast('Failed to load the checklist', 'error');
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [taskId, showToast]);

    useEffect(() => {
        onCountChange?.(items.filter((i) => i.done).length, items.length);
    }, [items, onCountChange]);

    const markBusy = (id: string, on: boolean) =>
        setBusy((prev) => {
            const next = new Set(prev);
            if (on) next.add(id);
            else next.delete(id);
            return next;
        });

    const handleAdd = async () => {
        const title = draft.trim();
        if (!title || adding) return;

        setAdding(true);
        try {
            const created = await createSubtask(taskId, title);
            setItems((prev) => [...prev, created]);
            setDraft('');
            inputRef.current?.focus();
        } catch {
            showToast('Could not add the item', 'error');
        } finally {
            setAdding(false);
        }
    };

    const handleToggle = async (item: Subtask) => {
        // Optimistic flip; reverted on failure.
        const nextDone = !item.done;
        setItems((prev) => prev.map((i) => (i.id === item.id ? { ...i, done: nextDone } : i)));
        markBusy(item.id, true);
        try {
            await updateSubtask(item.id, { done: nextDone });
        } catch {
            setItems((prev) => prev.map((i) => (i.id === item.id ? { ...i, done: item.done } : i)));
            showToast('Could not update the item', 'error');
        } finally {
            markBusy(item.id, false);
        }
    };

    const handleDelete = async (item: Subtask) => {
        const snapshot = items;
        setItems((prev) => prev.filter((i) => i.id !== item.id));
        markBusy(item.id, true);
        try {
            await deleteSubtask(item.id);
        } catch {
            setItems(snapshot);
            showToast('Could not delete the item', 'error');
        } finally {
            markBusy(item.id, false);
        }
    };

    const done = items.filter((i) => i.done).length;
    const total = items.length;
    const pct = total === 0 ? 0 : Math.round((done / total) * 100);

    return (
        <div>
            <div className="mb-2 flex items-center justify-between">
                <span className="font-mono text-[10px] uppercase tracking-wider text-text">
                    Checklist
                </span>
                {total > 0 && (
                    <span className="font-mono text-[10px] text-text">
                        {done}/{total}
                    </span>
                )}
            </div>

            {total > 0 && (
                <div className="mb-3 h-1.5 w-full overflow-hidden rounded-full bg-border">
                    <div
                        className="h-full rounded-full bg-success transition-all"
                        style={{ width: `${pct}%` }}
                    />
                </div>
            )}

            {loading ? (
                <div className="flex justify-center py-3">
                    <Loader2 className="h-4 w-4 animate-spin text-text" />
                </div>
            ) : (
                <ul className="space-y-1.5">
                    {items.map((item) => {
                        const isBusy = busy.has(item.id);
                        return (
                            <li key={item.id} className="group flex items-center gap-2">
                                <button
                                    type="button"
                                    onClick={() => handleToggle(item)}
                                    disabled={isBusy}
                                    aria-pressed={item.done}
                                    aria-label={item.done ? 'Mark not done' : 'Mark done'}
                                    className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border transition ${
                                        item.done
                                            ? 'border-success bg-success text-bg'
                                            : 'border-border hover:border-success'
                                    }`}
                                >
                                    {item.done && <Check className="h-3 w-3" strokeWidth={3} />}
                                </button>

                                <span
                                    className={`flex-1 text-xs ${
                                        item.done ? 'text-text line-through' : 'text-text-h'
                                    }`}
                                >
                                    {item.title}
                                </span>

                                <button
                                    type="button"
                                    onClick={() => handleDelete(item)}
                                    disabled={isBusy}
                                    aria-label="Delete item"
                                    className="rounded p-1 text-text opacity-0 transition hover:text-danger group-hover:opacity-100"
                                >
                                    <Trash2 className="h-3 w-3" />
                                </button>
                            </li>
                        );
                    })}
                </ul>
            )}

            {/* A plain div, not a form: this component renders inside the task
                modal's <form>, and a nested form is invalid HTML. Enter still
                adds an item via the keydown handler. */}
            <div className="mt-2 flex items-center gap-2">
                <input
                    ref={inputRef}
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                            e.preventDefault();
                            void handleAdd();
                        }
                    }}
                    maxLength={500}
                    placeholder="Add an item…"
                    className="flex-1 rounded-lg border border-border bg-code-bg px-2.5 py-1.5 text-xs text-text-h outline-none transition focus:ring-1 focus:ring-success"
                />
                <button
                    type="button"
                    onClick={() => void handleAdd()}
                    disabled={adding || draft.trim() === ''}
                    aria-label="Add checklist item"
                    className="rounded-lg border border-border bg-surface p-1.5 text-text-h transition hover:bg-code-bg disabled:opacity-40"
                >
                    {adding ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
                </button>
            </div>
        </div>
    );
};
