import React, { useId, useState } from 'react';
import { Plus, Terminal } from 'lucide-react';
import type { TaskInput, TaskPriority, TaskStatus } from '../types';
import { PRIORITIES, PRIORITY_LABELS, STATUS_LABELS } from '../types';

export interface TaskCreateProps {
    onCreate: (input: TaskInput) => Promise<void>;
    /** Preselects the column a header "+ Add" was pressed in. */
    defaultStatus?: TaskStatus;
    autoFocus?: boolean;
    onCancel?: () => void;
}

const fieldClass =
    'w-full rounded-xl border border-border bg-bg px-2.5 py-1.5 text-xs text-text-h outline-none transition focus:border-accent';

const labelClass = 'block text-[10px] font-mono uppercase tracking-wider text-text';

export const TaskCreate: React.FC<TaskCreateProps> = ({
    onCreate,
    defaultStatus,
    autoFocus = false,
    onCancel,
}) => {
    const uid = useId();
    const [title, setTitle] = useState('');
    const [description, setDescription] = useState('');
    const [priority, setPriority] = useState<TaskPriority>('MEDIUM');
    const [estimate, setEstimate] = useState('');
    const [dueDate, setDueDate] = useState('');
    const [reviewer, setReviewer] = useState('');
    const [isExpanded, setIsExpanded] = useState(autoFocus);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const needsReviewer = defaultStatus === 'IN_REVIEW';
    const canSubmit =
        title.trim().length > 0 && (!needsReviewer || reviewer.trim().length > 0) && !loading;

    const reset = () => {
        setTitle('');
        setDescription('');
        setPriority('MEDIUM');
        setEstimate('');
        setDueDate('');
        setReviewer('');
        setError(null);
    };

    const collapse = () => {
        reset();
        setIsExpanded(false);
        onCancel?.();
    };

    const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        if (!canSubmit) return;

        const parsedEstimate = estimate.trim() === '' ? null : Number(estimate);
        if (parsedEstimate !== null && (Number.isNaN(parsedEstimate) || parsedEstimate < 0)) {
            setError('Estimate must be a positive number of hours.');
            return;
        }

        const input: TaskInput = {
            title: title.trim(),
            description: description.trim(),
            priority,
            estimate_hours: parsedEstimate,
            due_date: dueDate ? `${dueDate}T00:00:00Z` : null,
            reviewer: reviewer.trim() || null,
        };
        if (defaultStatus) input.status = defaultStatus;

        setLoading(true);
        setError(null);
        try {
            await onCreate(input);
            reset();
            // Stay open when opened from a column header so several cards can be
            // typed in a row; collapse again in the standalone quick-add.
            setIsExpanded(autoFocus);
        } catch {
            // The caller surfaces the toast; keep the draft so nothing is lost.
            setError('Could not create the task. Your draft is still here.');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div
            className={`rounded-2xl border border-border p-3 transition-colors ${
                isExpanded ? 'bg-surface shadow-(--shadow)' : 'bg-surface/80'
            }`}
        >
            <form onSubmit={handleSubmit}>
                <div className="flex items-center gap-2.5">
                    <Terminal aria-hidden="true" className="h-4 w-4 shrink-0 text-success" />
                    <input
                        type="text"
                        value={title}
                        autoFocus={autoFocus}
                        onChange={(e) => setTitle(e.target.value)}
                        onFocus={() => setIsExpanded(true)}
                        placeholder={
                            defaultStatus
                                ? `New task in ${STATUS_LABELS[defaultStatus]}...`
                                : 'Create a new task...'
                        }
                        aria-label="Task title"
                        className="w-full min-w-0 bg-transparent text-xs font-medium text-text-h outline-none placeholder:text-text/70 sm:text-sm"
                    />
                </div>

                {isExpanded && (
                    <div className="mt-3 space-y-3 border-t border-border/60 pt-3">
                        <textarea
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                            placeholder="Additional description or notes..."
                            rows={2}
                            aria-label="Description"
                            className="w-full resize-none bg-transparent text-xs text-text outline-none placeholder:text-text/70"
                        />

                        <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-3">
                            <div>
                                <label htmlFor={`${uid}-priority`} className={labelClass}>
                                    Priority
                                </label>
                                <select
                                    id={`${uid}-priority`}
                                    value={priority}
                                    onChange={(e) => setPriority(e.target.value as TaskPriority)}
                                    className={`${fieldClass} mt-1 font-mono`}
                                >
                                    {PRIORITIES.map((p) => (
                                        <option key={p} value={p}>
                                            {PRIORITY_LABELS[p]}
                                        </option>
                                    ))}
                                </select>
                            </div>

                            <div>
                                <label htmlFor={`${uid}-estimate`} className={labelClass}>
                                    Estimate (h)
                                </label>
                                <input
                                    id={`${uid}-estimate`}
                                    type="number"
                                    min={0}
                                    max={1000}
                                    step={0.5}
                                    inputMode="decimal"
                                    value={estimate}
                                    onChange={(e) => setEstimate(e.target.value)}
                                    placeholder="4"
                                    className={`${fieldClass} mt-1 font-mono`}
                                />
                            </div>

                            <div>
                                <label htmlFor={`${uid}-due`} className={labelClass}>
                                    Due date
                                </label>
                                <input
                                    id={`${uid}-due`}
                                    type="date"
                                    value={dueDate}
                                    onChange={(e) => setDueDate(e.target.value)}
                                    className={`${fieldClass} mt-1 font-mono`}
                                />
                            </div>
                        </div>

                        {needsReviewer && (
                            <div>
                                <label htmlFor={`${uid}-reviewer`} className={labelClass}>
                                    Reviewer <span className="text-danger">*</span>
                                </label>
                                <input
                                    id={`${uid}-reviewer`}
                                    type="text"
                                    value={reviewer}
                                    onChange={(e) => setReviewer(e.target.value)}
                                    placeholder="Who will review this?"
                                    required
                                    className={`${fieldClass} mt-1`}
                                />
                                {reviewer.trim() === '' && (
                                    <p className="mt-1 text-[10px] text-warning">
                                        In Review needs a reviewer before it can be saved.
                                    </p>
                                )}
                            </div>
                        )}

                        {error && (
                            <p role="alert" className="text-[11px] text-danger">
                                {error}
                            </p>
                        )}

                        <div className="flex items-center justify-end gap-2">
                            <button
                                type="button"
                                onClick={collapse}
                                className="rounded-xl px-3 py-1.5 text-xs font-semibold text-text transition hover:text-text-h active:scale-95"
                            >
                                Cancel
                            </button>
                            <button
                                type="submit"
                                disabled={!canSubmit}
                                className="flex items-center gap-1.5 rounded-xl bg-success px-4 py-1.5 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                            >
                                <Plus aria-hidden="true" className="h-3.5 w-3.5" strokeWidth={3} />
                                <span>{loading ? 'Saving...' : 'Add Task'}</span>
                            </button>
                        </div>
                    </div>
                )}
            </form>
        </div>
    );
};
