import React, { useEffect, useId, useRef, useState } from 'react';
import { AlertTriangle, Loader2, Save, Trash2, X } from 'lucide-react';
import type { Sprint, Task, TaskInput, TaskPriority, TaskStatus } from '../../types';
import {
    BOARD_STATUSES,
    PRIORITIES,
    PRIORITY_LABELS,
    STATUS_LABELS,
} from '../../types';

export interface TaskModalProps {
    open: boolean;
    /** Absent / null puts the modal in create mode. */
    task?: Task | null;
    defaultStatus?: TaskStatus;
    /** Focus the reviewer field on open — used by the IN_REVIEW gate. */
    focusReviewer?: boolean;
    sprints?: Sprint[];
    onClose: () => void;
    onSave: (input: TaskInput) => Promise<void>;
    onDelete?: (id: string) => Promise<void>;
}

interface FormState {
    title: string;
    description: string;
    status: TaskStatus;
    priority: TaskPriority;
    reviewer: string;
    estimate: string;
    buffer: string;
    spent: string;
    due: string;
    blockers: string;
    sprintId: string;
}

const DATE_ONLY = /^\d{4}-\d{2}-\d{2}$/;

const toDateInput = (value?: string | null): string => {
    if (!value) return '';
    const head = value.slice(0, 10);
    return DATE_ONLY.test(head) ? head : '';
};

const toNumberInput = (value?: number | null): string =>
    value === null || value === undefined ? '' : String(value);

const emptyForm = (task: Task | null | undefined, defaultStatus?: TaskStatus): FormState => ({
    title: task?.title ?? '',
    description: task?.description ?? '',
    status: task?.status ?? defaultStatus ?? 'TODO',
    priority: task?.priority ?? 'MEDIUM',
    reviewer: task?.reviewer ?? '',
    estimate: toNumberInput(task?.estimate_hours),
    buffer: toNumberInput(task?.buffer_hours),
    spent: toNumberInput(task?.spent_hours),
    due: toDateInput(task?.due_date),
    blockers: task?.blockers ?? '',
    sprintId: task?.sprint_id ?? '',
});

const FOCUSABLE =
    'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

const fieldClass =
    'w-full rounded-xl border border-border bg-bg px-3 py-2 text-sm text-text-h outline-none transition focus:border-accent';

const labelClass = 'block text-[10px] font-mono uppercase tracking-wider text-text';

/** Parses an hours field; returns `undefined` when the value is not usable. */
const parseHours = (raw: string): number | null | undefined => {
    if (raw.trim() === '') return null;
    const n = Number(raw);
    if (Number.isNaN(n) || n < 0 || n > 1000) return undefined;
    return n;
};

export const TaskModal: React.FC<TaskModalProps> = ({
    open,
    task,
    defaultStatus,
    focusReviewer = false,
    sprints,
    onClose,
    onSave,
    onDelete,
}) => {
    const uid = useId();
    const dialogRef = useRef<HTMLDivElement | null>(null);
    const titleRef = useRef<HTMLInputElement | null>(null);
    const reviewerRef = useRef<HTMLInputElement | null>(null);
    const openerRef = useRef<HTMLElement | null>(null);

    const [form, setForm] = useState<FormState>(() => emptyForm(task, defaultStatus));
    const [saving, setSaving] = useState(false);
    const [deleting, setDeleting] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const isEdit = Boolean(task?.id);
    const sprintList = Array.isArray(sprints) ? sprints : [];

    // Re-seed the form whenever a different task (or mode) is opened.
    useEffect(() => {
        if (!open) return;
        setForm(emptyForm(task, defaultStatus));
        setConfirmDelete(false);
        setError(null);
        setSaving(false);
        setDeleting(false);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, task?.id, defaultStatus]);

    // Focus management: park focus inside, lock the page, restore on close.
    useEffect(() => {
        if (!open) return;
        openerRef.current =
            document.activeElement instanceof HTMLElement ? document.activeElement : null;

        const timer = window.setTimeout(() => {
            if (focusReviewer && reviewerRef.current) reviewerRef.current.focus();
            else titleRef.current?.focus();
        }, 0);

        const previousOverflow = document.body.style.overflow;
        document.body.style.overflow = 'hidden';

        return () => {
            window.clearTimeout(timer);
            document.body.style.overflow = previousOverflow;
            openerRef.current?.focus();
        };
    }, [open, focusReviewer]);

    if (!open) return null;

    const reviewerMissing = form.status === 'IN_REVIEW' && form.reviewer.trim() === '';
    const canSubmit = form.title.trim().length > 0 && !reviewerMissing && !saving && !deleting;

    const set = <K extends keyof FormState>(key: K, value: FormState[K]) =>
        setForm((prev) => ({ ...prev, [key]: value }));

    const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key === 'Escape') {
            e.stopPropagation();
            onClose();
            return;
        }
        if (e.key !== 'Tab') return;

        const root = dialogRef.current;
        if (!root) return;
        const nodes = Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
            (n) => n.offsetParent !== null || n === document.activeElement
        );
        if (nodes.length === 0) return;

        const first = nodes[0];
        const last = nodes[nodes.length - 1];
        if (e.shiftKey && document.activeElement === first) {
            e.preventDefault();
            last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
            e.preventDefault();
            first.focus();
        }
    };

    const handleSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        if (!canSubmit) return;

        const estimate = parseHours(form.estimate);
        const buffer = parseHours(form.buffer);
        const spent = parseHours(form.spent);
        if (estimate === undefined || buffer === undefined || spent === undefined) {
            setError('Hours must be a number between 0 and 1000.');
            return;
        }

        const input: TaskInput = {
            title: form.title.trim(),
            description: form.description.trim(),
            status: form.status,
            priority: form.priority,
            reviewer: form.reviewer.trim() || null,
            estimate_hours: estimate,
            buffer_hours: buffer,
            blockers: form.blockers.trim() || null,
            sprint_id: form.sprintId || null,
            due_date: form.due ? `${form.due}T00:00:00Z` : null,
        };
        // spent_hours is patch-only per the API contract.
        if (isEdit) input.spent_hours = spent;

        setSaving(true);
        setError(null);
        try {
            await onSave(input);
        } catch {
            setError('Saving failed. Nothing was lost — try again.');
        } finally {
            setSaving(false);
        }
    };

    const handleDelete = async () => {
        if (!task?.id || !onDelete) return;
        setDeleting(true);
        setError(null);
        try {
            await onDelete(task.id);
        } catch {
            setError('Delete failed. The task is still here.');
            setConfirmDelete(false);
        } finally {
            setDeleting(false);
        }
    };

    return (
        <div
            onMouseDown={(e) => {
                if (e.target === e.currentTarget) onClose();
            }}
            onKeyDown={handleKeyDown}
            style={{ backgroundColor: 'rgba(0, 0, 0, 0.55)' }}
            className="fixed inset-0 z-50 flex items-end justify-center overflow-y-auto p-0 backdrop-blur-md sm:items-center sm:p-4"
        >
            <div
                ref={dialogRef}
                role="dialog"
                aria-modal="true"
                aria-labelledby={`${uid}-heading`}
                className="max-h-[92svh] w-full max-w-2xl overflow-y-auto rounded-t-3xl border border-border bg-surface shadow-(--shadow) sm:rounded-2xl"
            >
                <div className="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border bg-surface px-5 py-4">
                    <h2
                        id={`${uid}-heading`}
                        className="truncate text-sm font-bold uppercase tracking-wider text-text-h font-mono"
                    >
                        {isEdit ? 'Edit task' : 'New task'}
                    </h2>
                    <button
                        type="button"
                        onClick={onClose}
                        aria-label="Close"
                        className="shrink-0 rounded-lg border border-border p-1.5 text-text transition hover:bg-hover hover:text-text-h active:scale-95"
                    >
                        <X className="h-4 w-4" />
                    </button>
                </div>

                <form onSubmit={handleSubmit} className="space-y-4 px-5 py-4">
                    <div>
                        <label htmlFor={`${uid}-title`} className={labelClass}>
                            Title <span className="text-danger">*</span>
                        </label>
                        <input
                            id={`${uid}-title`}
                            ref={titleRef}
                            type="text"
                            value={form.title}
                            onChange={(e) => set('title', e.target.value)}
                            required
                            placeholder="What needs to happen?"
                            className={`${fieldClass} mt-1`}
                        />
                    </div>

                    <div>
                        <label htmlFor={`${uid}-description`} className={labelClass}>
                            Description
                        </label>
                        <textarea
                            id={`${uid}-description`}
                            value={form.description}
                            onChange={(e) => set('description', e.target.value)}
                            rows={3}
                            placeholder="Context, links, acceptance criteria..."
                            className={`${fieldClass} mt-1 resize-y`}
                        />
                    </div>

                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                        <div>
                            <label htmlFor={`${uid}-status`} className={labelClass}>
                                Status
                            </label>
                            <select
                                id={`${uid}-status`}
                                value={form.status}
                                onChange={(e) => set('status', e.target.value as TaskStatus)}
                                className={`${fieldClass} mt-1 font-mono`}
                            >
                                {BOARD_STATUSES.map((s) => (
                                    <option key={s} value={s}>
                                        {STATUS_LABELS[s]}
                                    </option>
                                ))}
                            </select>
                        </div>

                        <div>
                            <label htmlFor={`${uid}-priority`} className={labelClass}>
                                Priority
                            </label>
                            <select
                                id={`${uid}-priority`}
                                value={form.priority}
                                onChange={(e) => set('priority', e.target.value as TaskPriority)}
                                className={`${fieldClass} mt-1 font-mono`}
                            >
                                {PRIORITIES.map((p) => (
                                    <option key={p} value={p}>
                                        {PRIORITY_LABELS[p]}
                                    </option>
                                ))}
                            </select>
                        </div>
                    </div>

                    <div>
                        <label htmlFor={`${uid}-reviewer`} className={labelClass}>
                            Reviewer <span className="text-danger">*</span>
                        </label>
                        <input
                            id={`${uid}-reviewer`}
                            ref={reviewerRef}
                            type="text"
                            value={form.reviewer}
                            onChange={(e) => set('reviewer', e.target.value)}
                            required={form.status === 'IN_REVIEW'}
                            aria-invalid={reviewerMissing}
                            aria-describedby={reviewerMissing ? `${uid}-reviewer-hint` : undefined}
                            placeholder="Who signs this off?"
                            className={`${fieldClass} mt-1 ${
                                reviewerMissing ? 'border-danger focus:border-danger' : ''
                            }`}
                        />
                        {reviewerMissing && (
                            <p
                                id={`${uid}-reviewer-hint`}
                                className="mt-1 flex items-center gap-1.5 text-[11px] text-danger"
                            >
                                <AlertTriangle aria-hidden="true" className="h-3.5 w-3.5" />
                                In Review requires a reviewer before this can be saved.
                            </p>
                        )}
                    </div>

                    <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
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
                                value={form.estimate}
                                onChange={(e) => set('estimate', e.target.value)}
                                className={`${fieldClass} mt-1 font-mono`}
                            />
                        </div>
                        <div>
                            <label htmlFor={`${uid}-buffer`} className={labelClass}>
                                Buffer (h)
                            </label>
                            <input
                                id={`${uid}-buffer`}
                                type="number"
                                min={0}
                                max={1000}
                                step={0.5}
                                inputMode="decimal"
                                value={form.buffer}
                                onChange={(e) => set('buffer', e.target.value)}
                                className={`${fieldClass} mt-1 font-mono`}
                            />
                        </div>
                        <div>
                            <label htmlFor={`${uid}-spent`} className={labelClass}>
                                Spent (h)
                            </label>
                            <input
                                id={`${uid}-spent`}
                                type="number"
                                min={0}
                                max={1000}
                                step={0.5}
                                inputMode="decimal"
                                value={form.spent}
                                disabled={!isEdit}
                                title={isEdit ? undefined : 'Available once the task exists'}
                                onChange={(e) => set('spent', e.target.value)}
                                className={`${fieldClass} mt-1 font-mono disabled:opacity-50`}
                            />
                        </div>
                        <div>
                            <label htmlFor={`${uid}-due`} className={labelClass}>
                                Due date
                            </label>
                            <input
                                id={`${uid}-due`}
                                type="date"
                                value={form.due}
                                onChange={(e) => set('due', e.target.value)}
                                className={`${fieldClass} mt-1 font-mono`}
                            />
                        </div>
                    </div>

                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                        <div>
                            <label htmlFor={`${uid}-blockers`} className={labelClass}>
                                Blockers
                            </label>
                            <input
                                id={`${uid}-blockers`}
                                type="text"
                                value={form.blockers}
                                onChange={(e) => set('blockers', e.target.value)}
                                placeholder="What is in the way?"
                                className={`${fieldClass} mt-1`}
                            />
                        </div>
                        <div>
                            <label htmlFor={`${uid}-sprint`} className={labelClass}>
                                Sprint
                            </label>
                            <select
                                id={`${uid}-sprint`}
                                value={form.sprintId}
                                onChange={(e) => set('sprintId', e.target.value)}
                                className={`${fieldClass} mt-1`}
                            >
                                <option value="">No sprint (backlog)</option>
                                {sprintList.map((s) => (
                                    <option key={s.id} value={s.id}>
                                        {s.name}
                                    </option>
                                ))}
                            </select>
                        </div>
                    </div>

                    {error && (
                        <p
                            role="alert"
                            className="rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger"
                        >
                            {error}
                        </p>
                    )}

                    <div className="flex flex-col-reverse gap-2 border-t border-border pt-4 sm:flex-row sm:items-center sm:justify-between">
                        <div className="flex items-center gap-2">
                            {isEdit && onDelete && !confirmDelete && (
                                <button
                                    type="button"
                                    onClick={() => setConfirmDelete(true)}
                                    className="flex items-center gap-1.5 rounded-xl border border-border px-3 py-2 text-xs font-semibold text-text transition hover:border-danger/40 hover:bg-danger/10 hover:text-danger active:scale-95"
                                >
                                    <Trash2 className="h-3.5 w-3.5" />
                                    Delete
                                </button>
                            )}

                            {isEdit && onDelete && confirmDelete && (
                                <div className="flex items-center gap-2">
                                    <span className="text-xs font-semibold text-danger">
                                        Delete for good?
                                    </span>
                                    <button
                                        type="button"
                                        onClick={handleDelete}
                                        disabled={deleting}
                                        className="rounded-xl bg-danger px-3 py-2 text-xs font-bold text-bg transition hover:opacity-90 active:scale-95 disabled:opacity-50"
                                    >
                                        {deleting ? 'Deleting...' : 'Yes, delete'}
                                    </button>
                                    <button
                                        type="button"
                                        onClick={() => setConfirmDelete(false)}
                                        className="rounded-xl px-2 py-2 text-xs font-semibold text-text transition hover:text-text-h"
                                    >
                                        Keep
                                    </button>
                                </div>
                            )}
                        </div>

                        <div className="flex items-center justify-end gap-2">
                            <button
                                type="button"
                                onClick={onClose}
                                className="rounded-xl px-3 py-2 text-xs font-semibold text-text transition hover:text-text-h active:scale-95"
                            >
                                Cancel
                            </button>
                            <button
                                type="submit"
                                disabled={!canSubmit}
                                className="flex items-center gap-1.5 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                            >
                                {saving ? (
                                    <Loader2 aria-hidden="true" className="h-3.5 w-3.5 animate-spin" />
                                ) : (
                                    <Save aria-hidden="true" className="h-3.5 w-3.5" />
                                )}
                                <span>{saving ? 'Saving...' : isEdit ? 'Save changes' : 'Create task'}</span>
                            </button>
                        </div>
                    </div>
                </form>
            </div>
        </div>
    );
};
